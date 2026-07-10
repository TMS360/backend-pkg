package postgresql

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/TMS360/backend-pkg/config"
	"github.com/TMS360/backend-pkg/response"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/jackc/pgx/v5/pgconn"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Default Postgres pool sizes. Conservative so N services × pool ≪ Railway
// max_connections; override per-service via POSTGRES_MAX_OPEN_CONNS /
// POSTGRES_MAX_IDLE_CONNS when a service genuinely needs more headroom.
const (
	defaultPostgresMaxOpenConns = 8
	defaultPostgresMaxIdleConns = 2
)

type Client struct {
}

func EnsureDatabase(cfg config.PostgresSQLConfig) error {
	initialDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=postgres port=%s sslmode=%s TimeZone=%s",
		cfg.Host, cfg.User, cfg.Password, cfg.Port, cfg.SSLMode, cfg.TimeZone)

	db, err := openGorm(initialDSN)
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

	db, err := openGorm(dsn)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	if err := db.Use(&tmsdb.TenantScopePlugin{}); err != nil {
		return nil, fmt.Errorf("failed to register tenant scope plugin: %w", err)
	}

	if sqlDB, err := db.DB(); err == nil {
		maxOpen := cfg.MaxOpenConns
		if maxOpen <= 0 {
			maxOpen = defaultPostgresMaxOpenConns
		}
		maxIdle := cfg.MaxIdleConns
		if maxIdle <= 0 {
			maxIdle = defaultPostgresMaxIdleConns
		}
		if maxIdle > maxOpen {
			maxIdle = maxOpen
		}
		sqlDB.SetMaxOpenConns(maxOpen)
		sqlDB.SetMaxIdleConns(maxIdle)
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		log.Printf("postgres pool: max_open=%d max_idle=%d", maxOpen, maxIdle)
	}

	return db, nil
}

// openGorm — единственная точка, где задаётся протокол pgx, используется и в
// EnsureDatabase, и в NewClient. PreferSimpleProtocol отключает неявный кэш
// prepared statements pgx, поэтому ALTER TABLE во время деплоя не вернёт 0A000
// из протухшего плана на дренящемся поде. Держим это здесь, чтобы контракт был
// тестируемым (см. postgresql_test.go).
func openGorm(dsn string) (*gorm.DB, error) {
	fmt.Println("dsn: ", dsn)
	return gorm.Open(
		postgres.New(postgres.Config{
			DSN:                  dsn,
			PreferSimpleProtocol: true,
		}),
		&gorm.Config{},
	)
}

const (
	// PgUniqueViolationCode is the PostgreSQL error code for unique constraint violation.
	PgUniqueViolationCode = "23505"
	// PgForeignKeyViolationCode is the PostgreSQL error code for foreign key violation.
	PgForeignKeyViolationCode = "23503"
	// PgNotNullViolationCode is the PostgreSQL error code for not-null violation.
	PgNotNullViolationCode = "23502"
	// PgCheckViolationCode is the PostgreSQL error code for check constraint violation.
	PgCheckViolationCode = "23514"
	// PgExclusionViolationCode is the PostgreSQL error code for exclusion constraint violation.
	PgExclusionViolationCode = "23P01"
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

// AsPublicError translates a raw *pgconn.PgError for a client-facing constraint
// violation into a response.PublicError with a clean 4xx user message. It lets
// the GraphQL/REST layer degrade a routine user-input error (FK/unique/check/
// not-null/exclusion) into a proper 400/409 even when a service forgets to
// translate it, instead of leaking a generic 500.
//
// The constraint name is placed ONLY in the technical (first) argument and must
// never reach the user. Returns (nil, false) for anything that is not a
// recognized *pgconn.PgError so callers can fall back to their default handling.
func AsPublicError(err error) (response.PublicError, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil, false
	}

	switch pgErr.Code {
	case PgForeignKeyViolationCode:
		return response.NewBadRequest(
			"foreign key violation: "+pgErr.ConstraintName,
			"A referenced record was not found.",
		), true
	case PgUniqueViolationCode:
		return response.NewConflict(
			"unique violation: "+pgErr.ConstraintName,
			"This record already exists.",
		), true
	case PgExclusionViolationCode:
		return response.NewConflict(
			"exclusion violation: "+pgErr.ConstraintName,
			"This conflicts with an existing record.",
		), true
	case PgCheckViolationCode, PgNotNullViolationCode:
		return response.NewBadRequest(
			"constraint violation: "+pgErr.ConstraintName,
			"Some required information is missing or invalid.",
		), true
	default:
		return nil, false
	}
}
