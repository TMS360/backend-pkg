package tmsdb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	// Re-entrant: if a transaction is already active on the context, run inside
	// it instead of opening a second connection. A nested WithTransaction on a
	// fresh connection self-deadlocks against the outer tx's row locks — the
	// inner UPDATE waits on a lock the outer holds, while the outer waits in Go
	// for the inner call to return. Postgres can't detect it (outer is
	// idle-in-transaction), so the request hangs to a gateway 504. See DEV-703.
	if _, ok := ctx.Value(ctxTransactionKey{}).(*gorm.DB); ok {
		return fn(ctx)
	}
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
		return err // marshal-ошибка = реальный баг, до БД, txn не трогает — surface it
	}

	var changes []events.Change
	if b.oldData != nil {
		changes = CalculateChanges(b.oldData, b.data)
	}

	rootType := b.rootType
	rootID := b.rootID
	if rootType == "" {
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
		Reason:         b.reason,
		Timestamp:      time.Now(),
		RootEntityType: rootType,
		RootEntityID:   rootID,
	}

	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return err
	}

	event := &model.OutboxEvent{
		EntityID:   b.aggID,
		EntityType: b.aggType,
		EventType:  b.evtType,
		Payload:    payloadBytes,
		Status:     "PENDING",
		Topic:      b.topic,
		CreatedAt:  time.Now(),
	}

	db := m.GetDB(ctx)

	// Savepoint возможен только внутри explicit-транзакции. Если publish вызван
	// вне txn (auto-commit), сохраняем прежнее поведение — просто INSERT.
	// Это же закрывает edge-кейс с вложенными транзакциями: savepoint'ы в PG
	// стекаются на любую глубину, нам достаточно факта "мы в txn".
	inTx := false
	if db.Statement != nil {
		_, inTx = db.Statement.ConnPool.(gorm.TxCommitter)
	}
	if !inTx {
		return db.Create(event).Error
	}

	const sp = "publish_sp"
	if err := db.Exec("SAVEPOINT " + sp).Error; err != nil {
		return fmt.Errorf("outbox: savepoint: %w", err)
	}

	if createErr := db.Create(event).Error; createErr != nil {
		// ROLLBACK TO SAVEPOINT снимает aborted-состояние, оставленное упавшим
		// INSERT'ом, и возвращает txn к точке savepoint. Работа каллера до этого — жива.
		if rbErr := db.Exec("ROLLBACK TO SAVEPOINT " + sp).Error; rbErr != nil {
			// Провал rollback-to — единственный по-настоящему невосстановимый случай:
			// txn застряла в abort. Пробрасываем, чтобы каллер откатился чисто.
			return fmt.Errorf("outbox: rollback to savepoint (insert err: %v): %w", createErr, rbErr)
		}
		_ = db.Exec("RELEASE SAVEPOINT " + sp).Error // чистим, чтобы имя не шэдоуило при след. publish в этой же txn

		// Non-fatal by design: лог + nil, каллер коммитит бизнес-данные.
		// Trade-off: этот domain-event теряется.
		log.Printf("outbox publish skipped (non-fatal) entity=%s/%s action=%s: %v",
			event.EntityType, event.EntityID, event.EventType, createErr)
		return nil
	}

	if err := db.Exec("RELEASE SAVEPOINT " + sp).Error; err != nil {
		// INSERT уже прошёл; невыпущенный savepoint просто доживёт до COMMIT. Не валим каллера.
		log.Printf("outbox: release savepoint after successful insert: %v", err)
	}

	return nil
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
	vOld := reflect.Indirect(reflect.ValueOf(oldVal))
	vNew := reflect.Indirect(reflect.ValueOf(newVal))

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
		if jsonTag == "-" || isAssociationField(field.Type) {
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

var timeType = reflect.TypeOf(time.Time{})

// associations (pointer-to-struct, struct, slice/map of structs) не являются
// колонками — они утекают в changes как "object -> null", когда old/new
// загружены с разными preload'ами. Трекаем только скаляры.
func isAssociationField(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		return t != timeType // time.Time оставляем как скаляр
	case reflect.Slice, reflect.Array:
		elem := t.Elem()
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Uint8 {
			return false // []byte и uuid.UUID([16]byte) — это скаляры
		}
		return elem.Kind() == reflect.Struct
	case reflect.Map:
		return true
	default:
		return false
	}
}
