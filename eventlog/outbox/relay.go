package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/TMS360/backend-pkg/observability"
	"github.com/TMS360/backend-pkg/tmsdb"
	kafkaGo "github.com/segmentio/kafka-go"
)

// Publisher is the subset of *kafka.Writer the relay uses. It exists so the
// per-topic publish loop below can be exercised without a broker; *kafka.Writer
// satisfies it, so every existing call site is unchanged.
type Publisher interface {
	WriteMessages(ctx context.Context, msgs ...kafkaGo.Message) error
}

type Relay struct {
	tm          tmsdb.TransactionManager
	repository  Repository
	kafkaWriter Publisher
}

func NewRelay(tm tmsdb.TransactionManager, kafkaWriter Publisher) *Relay {
	repository := NewOutboxEventRepository(tm)
	return &Relay{
		tm:          tm,
		repository:  repository,
		kafkaWriter: kafkaWriter,
	}
}

// Start polls the DB and publishes to Kafka
func (r *Relay) Start(ctx context.Context) {
	defer observability.RecoverGoroutine(ctx)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	batchSize := 50

	for {
		select {
		case <-ticker.C:
			batchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := r.ProcessBatch(batchCtx, batchSize); err != nil {
				slog.Error("outbox batch failed", "error", err)
				observability.CaptureWithCtx(batchCtx, err)
			}
			cancel() // Always clean up context
		case <-ctx.Done():
			return // Exit cleanly
		}
	}
}

// ProcessBatch processes a batch of outbox events
func (r *Relay) ProcessBatch(ctx context.Context, limit int) error {
	return r.tm.WithTransaction(ctx, func(ctx context.Context) error {
		// 1. Fetch Pending Events with SKIP LOCKED
		eventsList, err := r.repository.FetchPendingBatch(ctx, limit)
		if err != nil {
			return err
		}

		if len(eventsList) == 0 {
			return nil
		}

		// 2. Prepare Kafka Messages
		kafkaMessages := make([]kafkaGo.Message, 0, len(eventsList))
		idByIndex := make([]string, 0, len(eventsList))

		for _, event := range eventsList {
			// Producers can route a child entity onto a parent's topic via
			// EventBuilder.WithTopic. Empty Topic means "no override" — the
			// relay falls back to EntityType (the legacy behaviour).
			topic := event.Topic
			if topic == "" {
				topic = event.EntityType
			}
			kafkaMessages = append(kafkaMessages, kafkaGo.Message{
				Topic: topic,
				Key:   []byte(event.EntityID.String()), // Order by EntityID
				Value: event.Payload,
				Time:  event.CreatedAt,
			})
			idByIndex = append(idByIndex, event.ID.String())
		}

		// 3. Publish to Kafka, one batch per topic
		failedTopics := PublishByTopic(ctx, r.kafkaWriter, kafkaMessages)

		// 4. Delete what actually reached Kafka. Anything on a failed topic
		// stays PENDING and is retried on the next tick.
		idsToDelete := make([]string, 0, len(idByIndex))
		for i, msg := range kafkaMessages {
			if _, failed := failedTopics[msg.Topic]; failed {
				continue
			}
			idsToDelete = append(idsToDelete, idByIndex[i])
		}

		for topic, err := range failedTopics {
			// Not returned: returning here would roll back the deletes of the
			// topics that did publish, so a single unpublishable topic would
			// republish everything else forever. Loud in logs and Sentry, and
			// the events themselves are still in the table.
			slog.ErrorContext(ctx, "outbox topic not published", "topic", topic, "error", err)
			observability.CaptureWithCtx(ctx, err)
		}

		if len(idsToDelete) == 0 {
			return nil
		}
		slog.Debug("outbox events published", "count", len(idsToDelete))

		if err := r.repository.DeleteBatch(ctx, idsToDelete); err != nil {
			return err
		}
		return nil
	})
}

// PublishByTopic writes each topic's messages as its own batch and returns the
// topics whose batch failed, keyed by topic name.
//
// One batch per topic, not one batch for everything (DEV-1867 QA decline): a
// single WriteMessages spanning several topics fails as a whole, so one topic
// the cluster does not have — UNKNOWN_TOPIC_OR_PARTITION — took down the entire
// outbox of that service. Loads, trips and every other healthy topic stopped
// draining behind it, and the events looked lost rather than blocked. Isolating
// per topic confines a bad topic to itself: it keeps retrying, everything else
// keeps flowing.
//
// Ordering per entity is unaffected: an entity's events all carry the same
// topic and the same key, and their relative order inside that topic's batch is
// preserved.
func PublishByTopic(ctx context.Context, w Publisher, msgs []kafkaGo.Message) map[string]error {
	if len(msgs) == 0 {
		return nil
	}

	order := make([]string, 0, 4)
	byTopic := make(map[string][]kafkaGo.Message, 4)
	for _, m := range msgs {
		if _, seen := byTopic[m.Topic]; !seen {
			order = append(order, m.Topic)
		}
		byTopic[m.Topic] = append(byTopic[m.Topic], m)
	}

	var failed map[string]error
	for _, topic := range order {
		if err := w.WriteMessages(ctx, byTopic[topic]...); err != nil {
			if failed == nil {
				failed = make(map[string]error, 1)
			}
			failed[topic] = err
		}
	}
	return failed
}
