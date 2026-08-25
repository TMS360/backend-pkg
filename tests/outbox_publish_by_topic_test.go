package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/TMS360/backend-pkg/eventlog/outbox"
	kafkaGo "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1867 (QA decline). The outbox relay used to hand every pending event to
// one WriteMessages call, whatever mix of topics it contained. Kafka fails that
// call as a whole, so the day a service published to a topic the cluster did
// not have (UNKNOWN_TOPIC_OR_PARTITION), the batch never succeeded, nothing was
// deleted, and the SAME batch was retried every 500ms forever — the service's
// entire audit stream stopped, not just the new topic's. These tests pin the
// per-topic isolation that replaced it.

// writerSpy records what was written and fails the topics it is told to fail.
type writerSpy struct {
	batches  [][]kafkaGo.Message
	failWith map[string]error
}

func (w *writerSpy) WriteMessages(_ context.Context, msgs ...kafkaGo.Message) error {
	w.batches = append(w.batches, msgs)
	if len(msgs) == 0 {
		return nil
	}
	return w.failWith[msgs[0].Topic]
}

func msg(topic, key string) kafkaGo.Message {
	return kafkaGo.Message{Topic: topic, Key: []byte(key)}
}

func TestPublishByTopic_OneBatchPerTopic(t *testing.T) {
	w := &writerSpy{}

	failed := outbox.PublishByTopic(context.Background(), w, []kafkaGo.Message{
		msg("trips", "a"), msg("security_events", "b"), msg("trips", "c"),
	})

	assert.Empty(t, failed)
	require.Len(t, w.batches, 2, "one write per distinct topic, not one per message")
	assert.Equal(t, "trips", w.batches[0][0].Topic)
	assert.Len(t, w.batches[0], 2, "a topic's messages travel together")
	assert.Equal(t, "security_events", w.batches[1][0].Topic)
}

// The decline itself: an unknown topic must not stop the healthy ones.
func TestPublishByTopic_AnUnknownTopicDoesNotBlockTheOthers(t *testing.T) {
	unknown := errors.New("[3] Unknown Topic Or Partition")
	w := &writerSpy{failWith: map[string]error{"accounting_settings": unknown}}

	failed := outbox.PublishByTopic(context.Background(), w, []kafkaGo.Message{
		msg("accounting_settings", "a"), msg("driver_crews", "b"), msg("invoices", "c"),
	})

	require.Len(t, failed, 1)
	assert.ErrorIs(t, failed["accounting_settings"], unknown)
	assert.Len(t, w.batches, 3, "the healthy topics were still attempted")
}

// Ordering within a topic is what keeps an entity's history readable.
func TestPublishByTopic_KeepsOrderWithinATopic(t *testing.T) {
	w := &writerSpy{}

	outbox.PublishByTopic(context.Background(), w, []kafkaGo.Message{
		msg("roles", "created"), msg("teams", "x"), msg("roles", "deleted"),
	})

	require.Len(t, w.batches, 2)
	assert.Equal(t, []byte("created"), w.batches[0][0].Key)
	assert.Equal(t, []byte("deleted"), w.batches[0][1].Key)
}

func TestPublishByTopic_NothingToPublish(t *testing.T) {
	w := &writerSpy{}

	assert.Empty(t, outbox.PublishByTopic(context.Background(), w, nil))
	assert.Empty(t, w.batches, "an empty batch must not reach the broker at all")
}
