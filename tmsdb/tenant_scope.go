package tmsdb

import (
	"errors"
	"fmt"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"gorm.io/gorm"
)

type TenantScopePlugin struct{}

func (t *TenantScopePlugin) Name() string {
	return "TenantScopePlugin"
}

// Initialize registers the plugin with GORM
func (t *TenantScopePlugin) Initialize(db *gorm.DB) error {
	db.Callback().Query().Before("gorm:query").Register("tenant:query", t.addTenantCondition)
	db.Callback().Update().Before("gorm:update").Register("tenant:update", t.addTenantCondition)
	db.Callback().Delete().Before("gorm:delete").Register("tenant:delete", t.addTenantCondition)
	return nil
}

// addTenantCondition adds tenant filtering conditions to the DB query
func (t *TenantScopePlugin) addTenantCondition(db *gorm.DB) {
	// 1. Check Interface Implementation (Standard)
	if _, ok := db.Statement.Model.(model.TenantScoped); !ok {
		// Handle slice case if needed (see previous advice)
		return
	}

	// 2. Get Actor
	ctx := db.Statement.Context
	actor, err := middleware.GetActor(ctx)
	if err != nil {
		// If we can't identify the actor, this is an internal server error
		db.AddError(fmt.Errorf("tenant_plugin: failed to retrieve actor from context: %w", err))
		return
	}

	// 3. SuperAdmin Bypass
	if actor.IsSuperAdmin() {
		return
	}

	// 4. Standard User Scope
	if actor.Claims.CompanyID == nil {
		// CRITICAL: Raising an error here acts as a final safety net.
		// It prevents the query from running and returns the error to the Go code.
		db.AddError(errors.New("tenant_plugin: non-admin actor missing company_id"))
		return
	}

	// 5. Apply the Clause
	db.Where("company_id = ?", *actor.Claims.CompanyID)
}
