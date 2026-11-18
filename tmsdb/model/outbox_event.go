package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
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

// JSONRaw is a raw JSON value stored in a Postgres jsonb column. It is a
// drop-in for encoding/json.RawMessage that also survives GORM writes under
// pgx's simple protocol (PreferSimpleProtocol): Value returns a string, so
// Postgres parses it as JSON text instead of rejecting a []byte with 22P02.
//
// Because a defined type does NOT inherit json.RawMessage's marshaler methods,
// JSONRaw implements MarshalJSON/UnmarshalJSON itself — otherwise json.Marshal
// would base64-encode the underlying []byte in API responses. With them, JSONRaw
// round-trips as raw JSON both to the DB and over the wire.
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

// MarshalJSON emits the stored bytes verbatim (raw JSON), mirroring
// json.RawMessage. An empty/nil value serializes to JSON null.
func (j JSONRaw) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON captures the raw JSON bytes verbatim, mirroring json.RawMessage.
func (j *JSONRaw) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("model.JSONRaw: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[:0], data...)
	return nil
}
