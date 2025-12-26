package consumer

import (
	"context"
	"encoding/json"

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
func (e *Engine) GetMatchingRules(ctx context.Context, eventType string, eventData interface{}) ([]EventRule, error) {
	var rules []EventRule

	// 1. Fetch potential rules from DB
	err := e.db.WithContext(ctx).
		Where("event_type = ? AND is_active = ?", eventType, true).
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
// For now, it returns true if no conditions exist.
func (e *Engine) matchesCondition(conditions json.RawMessage, data interface{}) bool {
	if len(conditions) == 0 || string(conditions) == "{}" || string(conditions) == "null" {
		return true // No conditions = Always match
	}

	// TODO: Integrate a library like "github.com/nikunjy/rules" here
	// return ruleParser.Evaluate(conditions, data)
	return true
}
