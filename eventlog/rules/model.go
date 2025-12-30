package rules

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EventRule represents a dynamic business logic rule.
// Example: "When USER_CREATED, if role='admin', then NOTIFY_ADMINS"
type EventRule struct {
	ID           uuid.UUID       `gorm:"type:uuid;primaryKey"`
	Topic        string          `gorm:"size:255;index;not null"` // e.g., "users", "orders"
	EventType    string          `gorm:"size:255;index;not null"` // e.g., "USER_CREATED"
	Conditions   json.RawMessage `gorm:"type:jsonb"`              // e.g., {"role": "admin"}
	ActionType   string          `gorm:"size:255;not null"`       // e.g., "NOTIFY_ADMINS", "SEND_WELCOME_EMAIL"
	ActionConfig json.RawMessage `gorm:"type:jsonb"`              // e.g., {"emailTemplate": "welcome_admin"}
	IsActive     bool            `gorm:"default:true"`
	CreatedAt    time.Time       `gorm:"default:now()"`
}
