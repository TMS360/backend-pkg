package postgresql

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/TMS360/backend-pkg/config"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/jackc/pgx/v5/pgconn"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Client struct {
}

func EnsureDatabase(cfg config.PostgresSQLConfig) error {
	initialDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=postgres port=%s sslmode=%s TimeZone=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.Port, cfg.SSLMode, cfg.TimeZone)

	db, err := gorm.Open(postgres.Open(initialDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to postgres database: %w", err)
	}

	var exists bool
	if err := db.Raw("SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = ?)", cfg.DBName).Scan(&exists).Error; err != nil {
		return fmt.Errorf("failed to check database %s existence: %w", cfg.DBName, err)
	}

	if !exists {
		if err := db.Exec(fmt.Sprintf("CREATE DATABASE \"%s\"", cfg.DBName)).Error; err != nil {
			return fmt.Errorf("failed to create database %s: %w", cfg.DBName, err)
		}
		log.Printf("Database %s created successfully", cfg.DBName)
	} else {
		log.Printf("Database %s already exists", cfg.DBName)
	}

	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	return nil
}

func NewClient(cfg config.PostgresSQLConfig) (*gorm.DB, error) {
	if err := EnsureDatabase(cfg); err != nil {
		log.Fatalf("%v", err)
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port, cfg.SSLMode, cfg.TimeZone)

	fmt.Println("dsn: ", dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	if err := db.Use(&tmsdb.TenantScopePlugin{}); err != nil {
		return nil, fmt.Errorf("failed to register tenant scope plugin: %w", err)
	}

	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)
		//sqlDB.SetMaxIdleConns(5)
		//sqlDB.SetMaxOpenConns(20)
		//sqlDB.SetConnMaxLifetime(30 * time.Minute)
		//sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	}

	return db, nil
}

const (
	// PgUniqueViolationCode is the PostgreSQL error code for unique constraint violation.
	PgUniqueViolationCode = "23505"
)

// IsUniqueConstraintError checks if the error is a PostgreSQL unique constraint violation.
func IsUniqueConstraintError(err error) bool {
	// 1. Unwrap the error to see if it's a *pgconn.PgError
	var pgErr *pgconn.PgError

	// errors.As finds the first error in the chain that matches the target type
	if errors.As(err, &pgErr) {
		return pgErr.Code == PgUniqueViolationCode
	}

	return false
}
