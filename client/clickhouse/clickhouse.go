package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/TMS360/backend-pkg/config"
)

type Client struct {
	db clickhouse.Conn
}

func New(cfg config.ClickHouseConfig) (*Client, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.DBName,
			Username: cfg.User,
			Password: cfg.Password,
		},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		return nil, err
	}

	return &Client{db: conn}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.db.Ping(ctx)
}

func (c *Client) Close() error {
	return c.db.Close()
}
