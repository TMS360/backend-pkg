package rules

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EventRule represents a dynamic business logic rule.
// Example: "When USER_CREATED, if role='admin', then NOTIFY_ADMINS"
type EventRule struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`

	// EventType acts as the filter (e.g., "USER_CREATED")
	EventType string `gorm:"size:255;index;not null"`

	// Conditions is a JSON object describing logic (e.g. {"priority": "high"})
	// You can use a library like 'nikunjy/rules' to parse this.
	Conditions json.RawMessage `gorm:"type:jsonb"`

	// ActionType maps to a Go function (e.g., "SEND_EMAIL")
	ActionType string `gorm:"size:255;not null"`

	// ActionConfig is the params for that function (e.g., {"template": "welcome"})
	ActionConfig json.RawMessage `gorm:"type:jsonb"`

	IsActive  bool      `gorm:"default:true"`
	CreatedAt time.Time `gorm:"default:now()"`
}
