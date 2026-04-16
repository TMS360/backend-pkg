package guest

import "github.com/google/uuid"

// ShareLinkRedisData is stored as JSON in Redis under key share_link:{slid}
type ShareLinkRedisData struct {
	CompanyID  string `json:"company_id"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
}

// AccessLogEvent is published to Kafka when a new guest visit is detected.
type AccessLogEvent struct {
	ShareLinkID string `json:"share_link_id"`
	CompanyID   string `json:"company_id"`
	IPAddress   string `json:"ip_address"`
	UserAgent   string `json:"user_agent"`
	AccessedAt  string `json:"accessed_at"`
}

// ResolvedGuest is the result of a successful Redis lookup, cached on context.
type ResolvedGuest struct {
	ShareLinkID uuid.UUID
	CompanyID   uuid.UUID
	Resource    string
	ResourceID  uuid.UUID
}
