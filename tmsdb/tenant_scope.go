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
	actor, _ := middleware.GetActor(ctx)
	if actor == nil {
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

	tableName := db.Statement.Table
	if tableName == "" {
		tableName = db.Statement.Schema.Table
	}

	// 5. Apply the Clause
	db.Where(fmt.Sprintf("%s.company_id = ?", db.Statement.Quote(tableName)), *actor.Claims.CompanyID)
}
