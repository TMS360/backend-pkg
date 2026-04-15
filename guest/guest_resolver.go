package guest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

// GuestResolver performs the lazy Redis lookup and publishes access log events.
type GuestResolver struct {
	redis       *redis.Client
	kafkaWriter *kafka.Writer
}

// GuestResolverConfig holds the dependencies for constructing a GuestResolver.
type GuestResolverConfig struct {
	Redis        *redis.Client
	KafkaBrokers []string
	KafkaTopic   string // e.g. "sharelink.access-logs"
}

// NewGuestResolver creates a resolver with a Redis client and an async Kafka writer.
func NewGuestResolver(cfg GuestResolverConfig) *GuestResolver {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.KafkaBrokers...),
		Topic:        cfg.KafkaTopic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    100,
		BatchTimeout: 1 * time.Second,
		Async:        true,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			slog.Error(fmt.Sprintf("kafka writer: "+msg, args...))
		}),
	}

	return &GuestResolver{
		redis:       cfg.Redis,
		kafkaWriter: writer,
	}
}

// Close shuts down the Kafka writer. Call during graceful shutdown.
func (gr *GuestResolver) Close() error {
	return gr.kafkaWriter.Close()
}

// Resolve performs the Redis allowlist lookup on first call and caches the result
// on the context for all subsequent calls within the same HTTP request.
func (gr *GuestResolver) Resolve(ctx context.Context) (context.Context, *ResolvedGuest, error) {
	// Return cached result if already resolved in this request.
	if g, ok := GetResolvedGuest(ctx); ok {
		return ctx, g, nil
	}

	pending, ok := GetPendingClaims(ctx)
	if !ok {
		return ctx, nil, fmt.Errorf("no guest claims on context")
	}

	// Redis allowlist lookup.
	key := fmt.Sprintf("share_link:%s", pending.ShareLinkID)
	val, err := gr.redis.Get(ctx, key).Result()
	if err != nil {
		return ctx, nil, fmt.Errorf("share link not found or revoked")
	}

	var data ShareLinkRedisData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return ctx, nil, fmt.Errorf("corrupt share link data in redis: %w", err)
	}

	resourceID, err := uuid.Parse(data.ResourceID)
	if err != nil {
		return ctx, nil, fmt.Errorf("invalid resource ID in redis: %w", err)
	}

	resolved := &ResolvedGuest{
		ShareLinkID: pending.ShareLinkID,
		Resource:    data.Resource,
		ResourceID:  resourceID,
	}

	// Cache on context for remaining resolvers in this request.
	ctx = WithResolvedGuest(ctx, resolved)

	// Fire debounced access log event.
	gr.maybeLogAccess(ctx, pending)

	return ctx, resolved, nil
}

// maybeLogAccess uses Redis SETNX as a cross-pod debouncer. If this (shareLinkID, ip)
// pair hasn't been seen in the last 3 minutes, publish an event to Kafka.
func (gr *GuestResolver) maybeLogAccess(ctx context.Context, pending *PendingGuestClaims) {
	ip := ResolveClientIP(pending.Request)
	dedupeKey := fmt.Sprintf("access_seen:%s:%s", pending.ShareLinkID, ip)

	set, err := gr.redis.SetNX(ctx, dedupeKey, "1", 3*time.Minute).Result()
	if err != nil || !set {
		return // already seen in this window, or Redis error — skip
	}

	event := AccessLogEvent{
		ShareLinkID: pending.ShareLinkID.String(),
		IPAddress:   ip,
		UserAgent:   pending.Request.UserAgent(),
		AccessedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	value, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal access log event", "err", err)
		return
	}

	// Kafka writer is async — this does not block the request.
	// Key = shareLinkID ensures ordering per link within a partition.
	if err := gr.kafkaWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(pending.ShareLinkID.String()),
		Value: value,
	}); err != nil {
		slog.Error("failed to write access log to kafka", "err", err)
	}
}
