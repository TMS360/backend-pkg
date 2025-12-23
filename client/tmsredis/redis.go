package tmsredis

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"

	"github.com/TMS360/backend-pkg/config"
)

func NewClient(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       0, // use default DB
	})

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return rdb, nil
}
