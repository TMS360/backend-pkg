package tmsdb

import (
	"context"

	"gorm.io/gorm"
)

type TransactionManager interface {
	// WithTransaction executes the fn function within a transaction.
	// If fn returns an error, Rollback occurs; if nil, Commit occurs.
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	GetDB(ctx context.Context) *gorm.DB
}
