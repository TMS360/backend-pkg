package guest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/google/uuid"
)

type GuestResolver struct {
	tm tmsdb.TransactionManager
}

func NewGuestResolver(tm tmsdb.TransactionManager) *GuestResolver {
	return &GuestResolver{tm: tm}
}

func (gr *GuestResolver) Resolve(ctx context.Context) (context.Context, *ResolvedGuest, error) {
	if g, ok := GetResolvedGuest(ctx); ok {
		return ctx, g, nil
	}

	pending, ok := GetPendingClaims(ctx)
	if !ok {
		return ctx, nil, fmt.Errorf("no guest claims on context")
	}

	key := fmt.Sprintf("share_link:%s", pending.ShareLinkID)

	var data ShareLinkRedisData
	if err := cache.Get(ctx, key, &data); err != nil {
		return ctx, nil, fmt.Errorf("share link not found or revoked")
	}

	resourceID, err := uuid.Parse(data.ResourceID)
	if err != nil {
		return ctx, nil, fmt.Errorf("invalid resource ID in redis: %w", err)
	}

	companyID, err := uuid.Parse(data.CompanyID)
	if err != nil {
		return ctx, nil, fmt.Errorf("invalid company ID in redis: %w", err)
	}

	resolved := &ResolvedGuest{
		ShareLinkID: pending.ShareLinkID,
		CompanyID:   companyID,
		Resource:    data.Resource,
		ResourceID:  resourceID,
	}

	ctx = WithResolvedGuest(ctx, resolved)
	gr.maybeLogAccess(ctx, pending)

	return ctx, resolved, nil
}

func (gr *GuestResolver) maybeLogAccess(ctx context.Context, pending *PendingGuestClaims) {
	ip := ResolveClientIP(pending.Request)
	dedupeKey := fmt.Sprintf("access_seen:%s:%s", pending.ShareLinkID, ip)

	set, err := cache.SetNX(ctx, dedupeKey, "1", 3*time.Minute)
	if err != nil || !set {
		return
	}

	event := AccessLogEvent{
		ShareLinkID: pending.ShareLinkID.String(),
		IPAddress:   ip,
		UserAgent:   pending.Request.UserAgent(),
		AccessedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	if err := gr.tm.Publish(ctx, "share_links", "access_log", pending.ShareLinkID, event); err != nil {
		slog.Error("failed to publish access log event", "err", err)
	}
}
