package eventlog

import (
	"testing"

	"github.com/segmentio/kafka-go"
)

// DEV-4482: the outbox relay keys messages by entity id; the writer must route
// one key to one partition, or one entity's events are consumed out of order.
func TestNewWriter_KeepsOneKeyOnOnePartition(t *testing.T) {
	w := NewWriter([]string{"localhost:9092"})
	partitions := []int{0, 1, 2}

	for _, key := range []string{"3f1c0a2e-0000-4000-8000-000000000001", "9b7d6e5f-0000-4000-8000-000000000002"} {
		msg := kafka.Message{Key: []byte(key), Value: make([]byte, 64)}
		first := w.Balancer.Balance(msg, partitions...)
		for i := 0; i < 20; i++ {
			msg.Value = make([]byte, 64*(i+2))
			if got := w.Balancer.Balance(msg, partitions...); got != first {
				t.Fatalf("key %s went to partition %d, then %d", key, first, got)
			}
		}
	}
}
