package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/TMS360/backend-pkg/observability"
	"github.com/TMS360/backend-pkg/tmsdb"
	kafkaGo "github.com/segmentio/kafka-go"
)

// Outbox row statuses. The column is plain varchar, so a new status needs no
// table change in any service.
const (
	StatusPending = "PENDING"
	// StatusParkedTooLarge marks an event Kafka refused as too large (DEV-3198).
	// It is put aside instead of retried: retrying can never succeed, and
	// while it stayed PENDING it blocked its whole topic and, once enough of
	// them piled up, the whole fetch window. The status itself is the reason;
	// the size is octet_length(payload::text). An admin re-queues or deletes
	// it through AdminService once the producer is fixed.
	StatusParkedTooLarge = "PARKED_TOO_LARGE"
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
	// alert reports a parked event. Swappable in tests; Sentry in production.
	alert func(ctx context.Context, err error)
}

func NewRelay(tm tmsdb.TransactionManager, kafkaWriter Publisher) *Relay {
	return NewRelayWithRepository(tm, NewOutboxEventRepository(tm), kafkaWriter, nil)
}

// NewRelayWithRepository builds a relay on an explicit repository and alert
// sink (nil alert = Sentry). Used by tests to run ProcessBatch without a DB.
func NewRelayWithRepository(tm tmsdb.TransactionManager, repository Repository, kafkaWriter Publisher, alert func(ctx context.Context, err error)) *Relay {
	if alert == nil {
		alert = observability.CaptureWithCtx
	}
	return &Relay{
		tm:          tm,
		repository:  repository,
		kafkaWriter: kafkaWriter,
		alert:       alert,
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
		// 1. Fetch Pending Events with SKIP LOCKED. Only PENDING rows are read,
		// so parked rows never take a place in the window.
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
		result := PublishByTopic(ctx, r.kafkaWriter, kafkaMessages)

		// 4. Park what Kafka refused as too large, delete what reached Kafka.
		// Anything else on a failed topic stays PENDING and is retried on the
		// next tick — a Kafka outage or a missing topic never parks an event.
		parked := make(map[int]struct{}, len(result.TooLarge))
		parkedIDs := make([]string, 0, len(result.TooLarge))
		for _, i := range result.TooLarge {
			parked[i] = struct{}{}
			parkedIDs = append(parkedIDs, idByIndex[i])
		}

		idsToDelete := make([]string, 0, len(idByIndex))
		for i, msg := range kafkaMessages {
			if _, isParked := parked[i]; isParked {
				continue
			}
			if _, failed := result.Failed[msg.Topic]; failed {
				continue
			}
			idsToDelete = append(idsToDelete, idByIndex[i])
		}

		for topic, err := range result.Failed {
			// Not returned: returning here would roll back the deletes of the
			// topics that did publish, so a single unpublishable topic would
			// republish everything else forever. Loud in logs and Sentry, and
			// the events themselves are still in the table.
			slog.ErrorContext(ctx, "outbox topic not published", "topic", topic, "error", err)
			observability.CaptureWithCtx(ctx, err)
		}

		if len(parkedIDs) > 0 {
			if err := r.repository.MarkParked(ctx, parkedIDs, StatusParkedTooLarge); err != nil {
				return err
			}
			// One alert per event, here at the moment of parking: the row is no
			// longer PENDING, so later ticks never see it again.
			for _, i := range result.TooLarge {
				msg := kafkaMessages[i]
				err := fmt.Errorf("outbox event parked: too large for Kafka: topic=%s event_id=%s size_bytes=%d",
					msg.Topic, idByIndex[i], len(msg.Value))
				slog.ErrorContext(ctx, "outbox event parked: too large for Kafka",
					"topic", msg.Topic, "event_id", idByIndex[i], "size_bytes", len(msg.Value), "status", StatusParkedTooLarge)
				r.alert(ctx, err)
			}
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

// PublishResult is what PublishByTopic could not deliver.
type PublishResult struct {
	// Failed holds the topics whose batch failed for a passing reason (broker
	// down, unknown topic, ...), keyed by topic. Retry them.
	Failed map[string]error
	// TooLarge holds the indexes (into the input slice) of the messages Kafka
	// refused as too large. Retrying them can never succeed.
	TooLarge []int
}

// PublishByTopic writes each topic's messages as its own batch.
//
// One batch per topic, not one batch for everything (DEV-1867 QA decline): a
// single WriteMessages spanning several topics fails as a whole, so one topic
// the cluster does not have — UNKNOWN_TOPIC_OR_PARTITION — took down the entire
// outbox of that service. Loads, trips and every other healthy topic stopped
// draining behind it, and the events looked lost rather than blocked. Isolating
// per topic confines a bad topic to itself: it keeps retrying, everything else
// keeps flowing.
//
// A message over the size limit (DEV-3198) is taken out of its topic's batch
// and the rest of that batch is sent again in the same call, so one too-large
// event no longer blocks its topic. The limit is whatever the writer enforces
// (kafka.Writer.BatchBytes, 1 MB by default) or the broker returns; it is not
// repeated here.
//
// Ordering per entity: an entity's events all carry the same topic and the
// same key, and their relative order inside that topic's batch is preserved.
// A parked event is the one exception — newer events of the same entity still
// go out past it. Consumers must not rely on delivery order across a parked
// event (accounting orders week-lock events by when the manager acted).
func PublishByTopic(ctx context.Context, w Publisher, msgs []kafkaGo.Message) PublishResult {
	var result PublishResult
	if len(msgs) == 0 {
		return result
	}

	order := make([]string, 0, 4)
	byTopic := make(map[string][]int, 4)
	for i, m := range msgs {
		if _, seen := byTopic[m.Topic]; !seen {
			order = append(order, m.Topic)
		}
		byTopic[m.Topic] = append(byTopic[m.Topic], i)
	}

	for _, topic := range order {
		tooLarge, err := publishTopic(ctx, w, msgs, byTopic[topic])
		result.TooLarge = append(result.TooLarge, tooLarge...)
		if err != nil {
			if result.Failed == nil {
				result.Failed = make(map[string]error, 1)
			}
			result.Failed[topic] = err
		}
	}
	return result
}

// publishTopic sends the messages at idx, dropping each one Kafka refuses as
// too large and resending the rest. It returns the dropped indexes and the
// error that failed the topic, if any.
func publishTopic(ctx context.Context, w Publisher, all []kafkaGo.Message, idx []int) ([]int, error) {
	var tooLarge []int
	idx = append([]int(nil), idx...)
	for len(idx) > 0 {
		batch := make([]kafkaGo.Message, len(idx))
		for i, j := range idx {
			batch[i] = all[j]
		}

		err := w.WriteMessages(ctx, batch...)
		if err == nil {
			return tooLarge, nil
		}

		// Writer-side check: nothing was sent, the error names the message.
		var tl kafkaGo.MessageTooLargeError
		if errors.As(err, &tl) {
			k := indexOfMessage(batch, tl.Message)
			if k < 0 {
				return tooLarge, err
			}
			tooLarge = append(tooLarge, idx[k])
			idx = append(idx[:k], idx[k+1:]...)
			continue
		}

		// Broker-side refusal: one error per message, nil = written.
		var we kafkaGo.WriteErrors
		if errors.As(err, &we) && len(we) == len(batch) {
			var other error
			for k, e := range we {
				switch {
				case e == nil:
				case errors.Is(e, kafkaGo.MessageSizeTooLarge):
					tooLarge = append(tooLarge, idx[k])
				default:
					other = err
				}
			}
			return tooLarge, other
		}

		return tooLarge, err
	}
	return tooLarge, nil
}

// indexOfMessage finds m in batch by identity of its payload bytes: the
// writer's error carries a copy of the Message struct, whose Value still points
// at our slice.
func indexOfMessage(batch []kafkaGo.Message, m kafkaGo.Message) int {
	for i := range batch {
		if sameBytes(batch[i].Value, m.Value) && sameBytes(batch[i].Key, m.Key) {
			return i
		}
	}
	return -1
}

func sameBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}
