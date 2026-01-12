package model

import (
	"time"

	"github.com/google/uuid"
)

// OutboxEvent maps to the 'outbox_events' table
type OutboxEvent struct {
	ID            uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	AggregateID   uuid.UUID `gorm:"type:uuid;not null"`
	AggregateType string    `gorm:"type:varchar(50);not null"`
	EventType     string    `gorm:"type:varchar(50);not null"`
	Payload       []byte    `gorm:"type:jsonb;not null"` // Postgres JSONB
	Status        string    `gorm:"type:varchar(20);default:'PENDING';not null;index"`
	CreatedAt     time.Time `gorm:"not null;autoCreateTime"`
	ProcessedAt   *time.Time
}
