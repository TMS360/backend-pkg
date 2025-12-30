package rules

import (
	"context"
	"encoding/json"
	"log"

	pkgRule "github.com/nikunjy/rules"
	"gorm.io/gorm"
)

type Engine struct {
	db *gorm.DB
}

// NewEngine creates a new Rule Engine instance
func NewEngine(db *gorm.DB) *Engine {
	return &Engine{db: db}
}

// GetMatchingRules fetches active rules for the event type and filters them.
// Note: For high performance, you might want to cache these rules in Redis/Memory later.
func (e *Engine) GetMatchingRules(ctx context.Context, topic, eventType string, eventData interface{}) ([]EventRule, error) {
	var rules []EventRule

	// 1. Fetch potential rules from DB
	err := e.db.WithContext(ctx).
		Where("topic = ?", topic).
		Where("event_type = ?", eventType).
		Where("is_active = ?", true).
		Find(&rules).Error
	if err != nil {
		return nil, err
	}

	var matchedRules []EventRule

	// 2. Filter in-memory based on Conditions
	for _, rule := range rules {
		if e.matchesCondition(rule.Conditions, eventData) {
			matchedRules = append(matchedRules, rule)
		}
	}

	return matchedRules, nil
}

// matchesCondition is a placeholder for your JSON logic evaluator.
func (e *Engine) matchesCondition(conditions json.RawMessage, data interface{}) bool {
	// A. Empty conditions always match (Default behavior)
	if len(conditions) == 0 || string(conditions) == "{}" || string(conditions) == "null" {
		return true
	}

	// B. Prepare Data
	// The rules engine requires map[string]interface{}
	// We handle cases where 'data' might be a Struct or json.RawMessage
	inputMap := make(map[string]interface{})

	// If data is already bytes (json.RawMessage), unmarshal it
	if bytesData, ok := data.(json.RawMessage); ok {
		if err := json.Unmarshal(bytesData, &inputMap); err != nil {
			log.Printf("rules engine: failed to unmarshal event data: %v", err)
			return false // Fail safe: if we can't read data, condition fails
		}
	} else {
		// If data is a struct, round-trip it to JSON to get a map (robustness)
		tmp, _ := json.Marshal(data)
		if err := json.Unmarshal(tmp, &inputMap); err != nil {
			return false
		}
	}

	// C. Evaluate Logic
	// library signature: Evaluate(jsonRule string, data map[string]interface{})
	match, err := pkgRule.Evaluate(string(conditions), inputMap)
	if err != nil {
		log.Printf("rules engine: invalid rule syntax: %v", err)
		return false
	}

	return match
}
