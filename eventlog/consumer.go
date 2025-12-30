package eventlog

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/TMS360/backend-pkg/eventlog/events"
	"github.com/TMS360/backend-pkg/eventlog/rules"
	"github.com/segmentio/kafka-go"
)

// SystemHandlerFunc is simpler than ActionFunc because it doesn't need DB config
type SystemHandlerFunc func(ctx context.Context, eventData json.RawMessage) error

// ActionFunc defines the signature for your business logic functions
type ActionFunc func(ctx context.Context, eventData json.RawMessage, config json.RawMessage) error

type Consumer struct {
	reader         *kafka.Reader
	engine         *rules.Engine
	systemHandlers map[string][]SystemHandlerFunc // Registry of system handlers
	actions        map[string]ActionFunc          // Registry of executable functions
}

func NewConsumer(
	brokers []string,
	groupID string,
	topics []string,
	engine *rules.Engine,
	systemHandlers map[string][]SystemHandlerFunc, // <--- Add this
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
		reader:         reader,
		engine:         engine,
		systemHandlers: systemHandlers,
		actions:        actions,
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
	if handlers, exists := c.systemHandlers[event.EntityType]; exists {
		for _, handler := range handlers {
			log.Printf("Executing System Handler for EntityType %s", event.EntityType)
			if err := handler(ctx, event.Data); err != nil {
				log.Printf("System handler execution failed: %v", err)
			}
		}
	}

	// A. Find Rules
	matchingRules, err := c.engine.GetMatchingRules(ctx, event.EntityType, event.Action, event.Data)
	if err != nil {
		log.Printf("Error getting matching rules: %v", err)
		return nil // Fail-safe: don't block event processing
	}

	for _, rule := range matchingRules {
		handler, exists := c.actions[rule.ActionType]
		if !exists {
			continue
		}

		log.Printf("Executing Rule %s -> Action %s", rule.ID, rule.ActionType)
		if err := handler(ctx, event.Data, rule.ActionConfig); err != nil {
			log.Printf("Action execution failed: %v", err)
		}
	}
	return nil
}
