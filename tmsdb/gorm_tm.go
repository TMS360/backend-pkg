package tmsdb

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/eventlog/events"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"github.com/TMS360/backend-pkg/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ctxTransactionKey struct{}

// GormTransactionManager TransactionManager implementation for GORM
type GormTransactionManager struct {
	db            *gorm.DB
	sourceService string
}

func NewGormTransactionManager(db *gorm.DB, sourceService string) TransactionManager {
	return &GormTransactionManager{db, sourceService}
}

// WithTransaction implement interface service.TransactionManager
func (m *GormTransactionManager) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, ctxTransactionKey{}, tx)
		return fn(txCtx)
	})
}

// GetDB извлекает транзакцию или возвращает fallback DB.
func (m *GormTransactionManager) GetDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(ctxTransactionKey{}).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return m.db.WithContext(ctx)
}

// Publish is the legacy shorthand. The published event's root_entity is itself
// (root_entity_type = aggType, root_entity_id = aggID). For nested leaves that
// roll up to a parent aggregate (e.g. trip_events → shipments), use Event(...).
func (m *GormTransactionManager) Publish(ctx context.Context, aggType, evtType string, aggID uuid.UUID, data interface{}, oldData ...interface{}) error {
	b := m.Event(aggType, evtType, aggID).WithData(data)
	if len(oldData) > 0 && oldData[0] != nil {
		b = b.WithOldData(oldData[0])
	}
	return b.Publish(ctx)
}

// Event opens a fluent builder for emitting an event.
func (m *GormTransactionManager) Event(aggType, evtType string, aggID uuid.UUID) *EventBuilder {
	return &EventBuilder{tm: m, aggType: aggType, evtType: evtType, aggID: aggID}
}

func (m *GormTransactionManager) writeEvent(ctx context.Context, b *EventBuilder) error {
	actor, _ := middleware.GetActor(ctx)

	var actorID, companyID *uuid.UUID
	if actor != nil {
		actorID = utils.Pointer(actor.ID)
		if actor.Claims.CompanyID != nil {
			companyID = utils.Pointer(*actor.Claims.CompanyID)
		}
	}

	dataBytes, err := json.Marshal(b.data)
	if err != nil {
		return err
	}

	var changes []events.Change
	if b.oldData != nil {
		changes = CalculateChanges(b.oldData, b.data)
	}

	rootType := b.rootType
	rootID := b.rootID
	if rootType == "" {
		// Default: an event roots to itself. Aggregate-root queries will still find
		// "self-rooted" leaves (shipments, users, …) via this default; nested leaves
		// that omit WithRoot remain invisible to aggregate queries by design.
		rootType = b.aggType
		rootID = b.aggID
	}

	eventPayload := events.EventPayload{
		SourceService:  m.sourceService,
		EventID:        uuid.New(),
		ActorID:        actorID,
		CompanyID:      companyID,
		EntityType:     b.aggType,
		EntityID:       b.aggID,
		Action:         b.evtType,
		Data:           json.RawMessage(dataBytes),
		Changes:        changes,
		Timestamp:      time.Now(),
		RootEntityType: rootType,
		RootEntityID:   rootID,
	}

	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return err
	}

	event := &model.OutboxEvent{
		AggregateID:   b.aggID,
		AggregateType: b.aggType,
		EventType:     b.evtType,
		Payload:       payloadBytes,
		Status:        "PENDING",
		CreatedAt:     time.Now(),
	}

	// Uses the active transaction from context automatically
	return m.GetDB(ctx).Create(event).Error
}

func (m *GormTransactionManager) Filter(ctx context.Context, model interface{}) *FilterBuilder {
	return newFilterBuilder(m.GetDB(ctx), model)
}

// CalculateChanges compares two structs and returns a list of changes.
func CalculateChanges(oldVal, newVal interface{}) []events.Change {
	var changes []events.Change

	// 1. Safety check: if either is nil, return empty
	if oldVal == nil || newVal == nil {
		return changes
	}

	// 2. Use reflect.Indirect to safely handle both pointers and direct values
	vOld := reflect.ValueOf(oldVal).Elem()
	vNew := reflect.ValueOf(newVal).Elem()

	// 3. Ensure both are actually structs and of the same type
	if vOld.Kind() != reflect.Struct || vNew.Kind() != reflect.Struct || vOld.Type() != vNew.Type() {
		return changes
	}

	typeOf := vOld.Type()

	for i := 0; i < vOld.NumField(); i++ {
		field := typeOf.Field(i)

		// Skip unexported fields or fields tagged to be ignored
		if field.PkgPath != "" {
			continue
		}

		// Get the JSON tag (e.g., `json:"first_name,omitempty"`)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue // Skip explicitly ignored fields
		}

		// Extract the actual name from the json tag (ignore ",omitempty")
		fieldName := field.Name
		if jsonTag != "" {
			fieldName = strings.Split(jsonTag, ",")[0]
		}

		valOld := vOld.Field(i).Interface()
		valNew := vNew.Field(i).Interface()

		// DeepEqual handles nested structs, arrays, and basic types
		if !reflect.DeepEqual(valOld, valNew) {
			changes = append(changes, events.Change{
				Field:    fieldName,
				OldValue: valOld,
				NewValue: valNew,
			})
		}
	}

	return changes
}
