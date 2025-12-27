package outbox

import (
	"context"
	"log"
	"time"

	"github.com/TMS360/backend-pkg/tmsdb"
	kafkaGo "github.com/segmentio/kafka-go"
)

type Relay struct {
	tm          tmsdb.TransactionManager
	repository  Repository
	kafkaWriter *kafkaGo.Writer
}

func NewRelay(tm tmsdb.TransactionManager, kafkaWriter *kafkaGo.Writer) *Relay {
	repository := NewOutboxEventRepository(tm)
	return &Relay{
		tm:          tm,
		repository:  repository,
		kafkaWriter: kafkaWriter,
	}
}

// Start polls the DB and publishes to Kafka
func (r *Relay) Start(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	batchSize := 50

	for {
		select {
		case <-ticker.C:
			batchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := r.ProcessBatch(batchCtx, batchSize); err != nil {
				log.Printf("⚠️ Batch failed: %v", err)
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
		var kafkaMessages []kafkaGo.Message
		var idsToDelete []string

		for _, event := range eventsList {
			kafkaMessages = append(kafkaMessages, kafkaGo.Message{
				Topic: event.Topic,
				Key:   []byte(event.AggregateID.String()), // Order by EntityID
				Value: event.Payload,
				Time:  event.CreatedAt,
			})
			idsToDelete = append(idsToDelete, event.ID.String())
		}

		// 3. Publish to Kafka (Batch Write)
		if err := r.kafkaWriter.WriteMessages(ctx, kafkaMessages...); err != nil {
			return err
		}
		log.Printf("Sent %d events to Kafka", len(kafkaMessages))

		// 4. Delete processed events
		if err := r.repository.DeleteBatch(ctx, idsToDelete); err != nil {
			return err
		}
		return nil
	})
}
