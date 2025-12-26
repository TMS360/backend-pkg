package consumer

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/TMS360/backend-pkg/eventlog/events"
	"github.com/segmentio/kafka-go"
)

// ActionFunc defines the signature for your business logic functions
type ActionFunc func(ctx context.Context, config json.RawMessage) error

type Consumer struct {
	reader  *kafka.Reader
	engine  *Engine
	actions map[string]ActionFunc // Registry of executable functions
}

func NewConsumer(
	brokers []string,
	groupID string,
	topics []string,
	engine *Engine,
	actions map[string]ActionFunc,
) *Consumer {

	// Configure Reader to listen to multiple topics
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		GroupTopics:    topics, // <--- Listen to "users", "teams", etc.
		GroupID:        groupID,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		CommitInterval: 1 * time.Second,
	})

	return &Consumer{
		reader:  reader,
		engine:  engine,
		actions: actions,
	}
}

func (c *Consumer) Start(ctx context.Context) {
	defer c.reader.Close()
	log.Printf("Dynamic Dispatcher started for topics: %v", c.reader.Config().GroupTopics)

	for {
		m, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			} // Context cancelled
			log.Printf("Consumer fetch error: %v", err)
			continue
		}

		// 1. Parse Envelope
		var payload events.EventPayload
		if err := json.Unmarshal(m.Value, &payload); err != nil {
			log.Printf("Skipping malformed event: %v", err)
			_ = c.reader.CommitMessages(ctx, m)
			continue
		}

		// 2. Dispatch Logic
		if err := c.dispatch(ctx, payload); err != nil {
			log.Printf("Error dispatching event %s: %v", payload.EventID, err)
			// Decide here: Commit anyway? Or retry?
			// Usually safe to commit if it's just a rule failure.
		}

		// 3. Commit Offset
		if err := c.reader.CommitMessages(ctx, m); err != nil {
			log.Printf("Failed to commit offset: %v", err)
		}
	}
}

func (c *Consumer) dispatch(ctx context.Context, event events.EventPayload) error {
	// A. Find Rules
	rules, err := c.engine.GetMatchingRules(ctx, event.Action, event.Data)
	if err != nil {
		return err
	}

	// B. Execute Actions
	for _, rule := range rules {
		handler, exists := c.actions[rule.ActionType]
		if !exists {
			log.Printf("Warning: No handler found for ActionType: %s", rule.ActionType)
			continue
		}

		log.Printf("Executing Rule %s -> Action %s", rule.ID, rule.ActionType)

		// Execute the injected function
		if err := handler(ctx, rule.ActionConfig); err != nil {
			log.Printf("Action execution failed: %v", err)
		}
	}
	return nil
}
