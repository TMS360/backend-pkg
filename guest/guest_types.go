package guest

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ShareLinkRedisData struct {
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	CompanyID  string `json:"company_id"`
}

type AccessLogEvent struct {
	ShareLinkID string `json:"share_link_id"`
	IPAddress   string `json:"ip_address"`
	UserAgent   string `json:"user_agent"`
	AccessedAt  string `json:"accessed_at"`
}

type ResolvedGuest struct {
	ShareLinkID uuid.UUID
	Resource    string
	ResourceID  uuid.UUID
	CompanyID   uuid.UUID
}

type ShareLinkClaims struct {
	ShareLinkID uuid.UUID `json:"slid"`
	CompanyID   uuid.UUID `json:"cid"`
	jwt.RegisteredClaims
}
