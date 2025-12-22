package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/TMS360/backend-pkg/config"
	"github.com/google/uuid"
)

type Client struct {
	db clickhouse.Conn
}

type TruckTrajectory struct {
	Timestamp time.Time `json:"timestamp"`
	Vin       string    `json:"vin"`
	TruckID   uuid.UUID `json:"truck_id"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Speed     int       `json:"speed"`
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

func (c *Client) Store(ctx context.Context, trajectory *TruckTrajectory) error {
	return c.db.Exec(ctx, `
		INSERT INTO tms.truck_trajectory (timestamp, vin, truck_id, lat, lon, speed)
		VALUES (?, ?, ?, ?, ?, ?)
	`, trajectory.Timestamp, trajectory.Vin, trajectory.TruckID, trajectory.Lat, trajectory.Lon, trajectory.Speed)
}

func (c *Client) GetByVIN(ctx context.Context, vin string, from, to time.Time) ([]*TruckTrajectory, error) {
	rows, err := c.db.Query(ctx, `
		SELECT timestamp, vin, truck_id, lat, lon, speed
		FROM tms.truck_trajectory
		WHERE vin = ? AND timestamp BETWEEN ? AND ?
	`, vin, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trajectories []*TruckTrajectory
	for rows.Next() {
		var t TruckTrajectory
		if err := rows.Scan(&t.Timestamp, &t.Vin, &t.TruckID, &t.Lat, &t.Lon, &t.Speed); err != nil {
			return nil, err
		}
		trajectories = append(trajectories, &t)
	}

	return trajectories, nil
}
