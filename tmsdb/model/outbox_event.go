package model

import (
	"time"

	"github.com/google/uuid"
)

// OutboxEvent maps to the 'outbox_events' table
type OutboxEvent struct {
	ID         uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	EntityID   uuid.UUID `gorm:"type:uuid;not null"`
	EntityType string    `gorm:"type:varchar(50);not null"`
	EventType  string    `gorm:"type:varchar(50);not null"`
	Payload    []byte    `gorm:"type:jsonb;not null"` // Postgres JSONB
	Status     string    `gorm:"type:varchar(20);default:'PENDING';not null;index"`
	// Topic optionally overrides the Kafka topic the relay publishes to.
	// Empty string ("") means "use EntityType as the topic" — the default
	// behaviour preserved for every existing emitter. Producers opt in via
	// EventBuilder.WithTopic when a child entity should route onto its
	// parent's topic (e.g. customer_comments → "customers").
	Topic       string    `gorm:"type:varchar(50);not null;default:''"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
	ProcessedAt *time.Time
}
