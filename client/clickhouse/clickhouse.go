package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/TMS360/backend-pkg/config"
)

type Client struct {
	db driver.Conn
}

// NewClient creates a new ClickHouse client with basic connection
func NewClient(cfg config.ClickHouseConfig) (*Client, error) {
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
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test connection
	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	return &Client{db: conn}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.db.Ping(ctx)
}

func (c *Client) Close() error {
	return c.db.Close()
}

// GetDB returns the underlying database connection
func (c *Client) GetDB() driver.Conn {
	return c.db
}

// Exec executes a query without returning rows
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	return c.db.Exec(ctx, query, args...)
}

// Query executes a query and returns rows
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.db.Query(ctx, query, args...)
}

// QueryRow executes a query and returns single row
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row {
	return c.db.QueryRow(ctx, query, args...)
}

// PrepareBatch prepares batch insert
func (c *Client) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	return c.db.PrepareBatch(ctx, query)
}
