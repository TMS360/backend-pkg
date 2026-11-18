package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/go-redis/redis/v8"
)

// LoadNotesLockOn reports whether the company locks load notes 5 minutes after
// they are written (DEV-3675). Requires an actor in ctx: cache.Get prefixes the
// key with "{companyID}:". See LoadNotesLockOnFrom for the rule. A process with
// no cache client reads off, like a cache that is down.
func LoadNotesLockOn(ctx context.Context) bool {
	if cache.Client() == nil {
		slog.WarnContext(ctx, "lock load notes: no cache client, treating as off")
		return false
	}
	return LoadNotesLockOnFrom(func(key string, dest *string) error {
		return cache.Get(ctx, fmt.Sprintf("setting:%s", key), dest)
	})
}

// LoadNotesLockOnFrom is the one rule behind LoadNotesLockOn: only a saved
// "true" turns the lock on. An unsaved switch is off (the default), and a cache
// read failure is off too — a Redis blip must never freeze every load note of a
// tenant. The failure is logged as a warning, not an error: it is degraded
// behaviour the next read heals, not something to page on.
func LoadNotesLockOnFrom(read SettingReader) bool {
	var v string
	err := read(string(enums.CompanySettingsGeneralKeyLockLoadNotes), &v)
	switch {
	case err == nil:
		return v == "true"
	case errors.Is(err, redis.Nil):
		return false
	default:
		slog.Warn("lock load notes: cache read failed, treating as off", "error", err)
		return false
	}
}
