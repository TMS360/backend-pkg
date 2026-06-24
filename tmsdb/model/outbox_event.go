package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OutboxEvent maps to the 'outbox_events' table
type OutboxEvent struct {
	ID         uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	EntityID   uuid.UUID `gorm:"type:uuid;not null"`
	EntityType string    `gorm:"type:varchar(50);not null"`
	EventType  string    `gorm:"type:varchar(50);not null"`
	Payload    JSONRaw   `gorm:"type:jsonb;not null"` // Postgres JSONB
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

type JSONRaw json.RawMessage

func (j JSONRaw) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}
func (j *JSONRaw) Scan(src any) error {
	switch v := src.(type) {
	case []byte:
		*j = append((*j)[:0], v...)
	case string:
		*j = JSONRaw(v)
	case nil:
		*j = nil
	default:
		return fmt.Errorf("unsupported scan type %T for JSONRaw", src)
	}
	return nil
}
