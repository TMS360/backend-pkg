package producer

import (
	"time"

	"github.com/segmentio/kafka-go"
)

// NewWriter NewSafeWriter creates a Kafka writer pre-configured for High Consistency.
// It forces 'RequireAll' acks to prevent data loss.
func NewWriter(brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    "", // Empty to allow dynamic topic selection per message
		Balancer: &kafka.LeastBytes{},

		// Critical Safety Settings
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  5,

		// Performance Settings
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		Compression:  kafka.Snappy,
	}
}
