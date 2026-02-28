package notify

import (
	"context"
	"encoding/json"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/google/uuid"
)

// Publisher sends notifications through the transactional outbox → Kafka → notification service.
type Publisher struct {
	tm TransactionManager
}

// TransactionManager is the subset of tmsdb.TransactionManager needed for publishing.
type TransactionManager interface {
	Publish(ctx context.Context, aggType, evtType string, aggID uuid.UUID, data interface{}) error
}

// NewPublisher creates a notification publisher.
// Pass your service's TransactionManager (tmsdb.GormTransactionManager).
func NewPublisher(tm TransactionManager) *Publisher {
	return &Publisher{tm: tm}
}

// NotificationType defines the visual style of the notification.
type NotificationType string

const (
	TypeInfo           NotificationType = "INFO"
	TypeSuccess        NotificationType = "SUCCESS"
	TypeError          NotificationType = "ERROR"
	TypeWarning        NotificationType = "WARNING"
	TypeActionRequired NotificationType = "ACTION_REQUIRED"
)

// NotificationPriority defines the urgency level.
type NotificationPriority string

const (
	PriorityLow    NotificationPriority = "LOW"
	PriorityNormal NotificationPriority = "NORMAL"
	PriorityHigh   NotificationPriority = "HIGH"
	PriorityUrgent NotificationPriority = "URGENT"
)

// Notification is the payload for sending a direct notification.
type Notification struct {
	// UserIDs — list of recipient user UUIDs (required).
	UserIDs []uuid.UUID `json:"user_ids"`

	// Title — notification title (required).
	Title string `json:"title"`

	// Type — notification type (default: INFO).
	Type NotificationType `json:"type,omitempty"`

	// Body — notification body text (optional).
	Body string `json:"body,omitempty"`

	// Priority — notification priority (default: NORMAL).
	Priority NotificationPriority `json:"priority,omitempty"`

	// Metadata — extra data like redirect_url, entity references (optional).
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// EntityType — source entity type, e.g. "shipments" (optional, for tracking).
	EntityType string `json:"source_entity_type,omitempty"`

	// EntityID — source entity ID (optional, for tracking).
	EntityID *uuid.UUID `json:"source_entity_id,omitempty"`
}

// Send publishes a notification through the transactional outbox.
// The notification service will pick it up from Kafka, save to DB, and deliver via WebSocket.
//
// CompanyID is automatically extracted from the JWT context (actor.Claims.CompanyID).
//
// Usage:
//
//	notifier := notify.NewPublisher(tm)
//	err := notifier.Send(ctx, notify.Notification{
//	    UserIDs: []uuid.UUID{userID1, userID2},
//	    Title:   "Shipment LD-1234 delivered",
//	    Type:    notify.TypeSuccess,
//	    Body:    "Delivered at Dallas, TX",
//	    Metadata: map[string]interface{}{
//	        "redirect_url": "/shipments/" + shipmentID.String(),
//	    },
//	})
func (p *Publisher) Send(ctx context.Context, n Notification) error {
	// Build user_ids as string array
	userIDStrs := make([]string, len(n.UserIDs))
	for i, id := range n.UserIDs {
		userIDStrs[i] = id.String()
	}

	// Set defaults
	notifType := n.Type
	if notifType == "" {
		notifType = TypeInfo
	}
	priority := n.Priority
	if priority == "" {
		priority = PriorityNormal
	}

	// Extract company_id from context
	var companyID string
	if actor, _ := middleware.GetActor(ctx); actor != nil && actor.Claims != nil && actor.Claims.CompanyID != nil {
		companyID = actor.Claims.CompanyID.String()
	}

	// Build the data payload
	data := map[string]interface{}{
		"user_ids":   userIDStrs,
		"type":       string(notifType),
		"title":      n.Title,
		"priority":   string(priority),
		"company_id": companyID,
	}

	if n.Body != "" {
		data["body"] = n.Body
	}
	if n.Metadata != nil {
		data["metadata"] = n.Metadata
	}
	if n.EntityType != "" {
		data["source_entity_type"] = n.EntityType
	}
	if n.EntityID != nil {
		data["source_entity_id"] = n.EntityID.String()
	}

	// Use a deterministic aggregate ID for deduplication
	aggID := uuid.New()

	// Publish to outbox → Kafka topic "notifications" with action "send"
	return p.tm.Publish(ctx, "notifications", "send", aggID, data)
}

// SendJSON publishes a raw notification payload. Use this for advanced cases
// where you need full control over the data field.
func (p *Publisher) SendJSON(ctx context.Context, data json.RawMessage) error {
	return p.tm.Publish(ctx, "notifications", "send", uuid.New(), data)
}
