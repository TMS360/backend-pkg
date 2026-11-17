package eventlog

import (
	"time"

	"github.com/segmentio/kafka-go"
)

// NewWriter NewSafeWriter creates a Kafka writer pre-configured for High Consistency.
// It forces 'RequireAll' acks to prevent data loss. The relay keys every
// message by entity id, so it balances by Hash: one entity stays on one
// partition and its events are consumed in write order (a nil key goes round robin).
func NewWriter(brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    "", // Empty to allow dynamic topic selection per message
		Balancer: &kafka.Hash{},

		// Critical Safety Settings
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  5,

		// Performance Settings
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		Compression:  kafka.Snappy,
	}
}
