package eventlog

import (
	"reflect"
	"time"

	"github.com/google/uuid"
)

// EventPayload defines the standard message format for all Kafka events
type EventPayload struct {
	EventID       uuid.UUID   `json:"event_id"`
	RequestID     string      `json:"request_id"`
	ActorID       uuid.UUID   `json:"actor_id"`
	ActorRole     string      `json:"actor_role"`
	EntityType    string      `json:"entity_type"` // TEAM, LOAD, TRIP
	EntityID      uuid.UUID   `json:"entity_id"`
	Action        string      `json:"action"` // CREATE, UPDATE, DELETE
	SourceService string      `json:"source_service"`
	Timestamp     time.Time   `json:"timestamp"`
	Data          interface{} `json:"data,omitempty"`
	Changes       []Change    `json:"changes,omitempty"`
}

type Change struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}

// CalculateChanges compares two structs and returns a list of changes.
// Both oldVal and newVal must be pointers to structs of the same type.
func CalculateChanges(oldVal, newVal interface{}) []Change {
	var changes []Change

	vOld := reflect.ValueOf(oldVal).Elem()
	vNew := reflect.ValueOf(newVal).Elem()
	typeOf := vOld.Type()

	for i := 0; i < vOld.NumField(); i++ {
		field := typeOf.Field(i)

		// Skip unexported fields or fields tagged to be ignored
		if field.PkgPath != "" {
			continue
		}

		valOld := vOld.Field(i).Interface()
		valNew := vNew.Field(i).Interface()

		if !reflect.DeepEqual(valOld, valNew) {
			changes = append(changes, Change{
				Field:    field.Name, // Or use field.Tag.Get("json")
				OldValue: valOld,
				NewValue: valNew,
			})
		}
	}

	return changes
}
