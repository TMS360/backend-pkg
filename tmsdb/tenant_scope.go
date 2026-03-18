package tmsdb

import (
	"errors"
	"fmt"
	"reflect"

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
	db.Callback().Query().Before("gorm:query").Register("tenant:query", t.addTenantConditionQuery)
	db.Callback().Update().Before("gorm:update").Register("tenant:update", t.addTenantConditionWrite)
	db.Callback().Delete().Before("gorm:delete").Register("tenant:delete", t.addTenantConditionWrite)
	return nil
}

// addTenantConditionQuery is triggered on SELECT statements
func (t *TenantScopePlugin) addTenantConditionQuery(db *gorm.DB) {
	t.applyScope(db, true)
}

// addTenantConditionWrite is triggered on UPDATE and DELETE statements
func (t *TenantScopePlugin) addTenantConditionWrite(db *gorm.DB) {
	t.applyScope(db, false)
}

// addTenantCondition adds tenant filtering conditions to the DB query
func (t *TenantScopePlugin) applyScope(db *gorm.DB, isRead bool) {
	// 1. NEW: Respect GORM's .Unscoped()
	if db.Statement.Unscoped {
		return
	}

	// 1. Safe Interface Implementation Check (Handles single structs AND slices)
	isTenantScoped, isShared := checkTenantInterfaces(db)

	// If the model doesn't implement either interface, do nothing (e.g., global admin tables)
	if !isTenantScoped && !isShared {
		return
	}

	// 2. Get Actor
	actor, _ := middleware.GetActor(db.Statement.Context)
	if actor == nil {
		return
	}

	// 3. SuperAdmin / System Bypass
	if actor.IsSystem || actor.IsSuperAdmin() || actor.IsGuest {
		return
	}

	// 4. Standard User Scope
	if actor.Claims.CompanyID == nil {
		db.AddError(errors.New("tenant_plugin: non-admin actor missing company_id"))
		return
	}

	// Resolve the table name safely
	tableName := db.Statement.Table
	if tableName == "" && db.Statement.Schema != nil {
		tableName = db.Statement.Schema.Table
	}
	if tableName == "" {
		return // Cannot determine table, fail safely
	}

	quotedTable := db.Statement.Quote(tableName)
	companyID := *actor.Claims.CompanyID

	// 5. Apply the Clause based on Operation Type and Interface
	if isRead && isShared {
		// READ on a Shared table: User can see their own company records OR system records
		db.Where(fmt.Sprintf("(%s.company_id = ? OR %s.is_system = ?)", quotedTable, quotedTable), companyID, true)
	} else {
		// WRITE on a Shared table, OR ANY operation on a strict Tenant table:
		// User is strictly locked to their own company_id.
		db.Where(fmt.Sprintf("%s.company_id = ?", quotedTable), companyID)
	}
}

func checkTenantInterfaces(db *gorm.DB) (isTenantScoped bool, isShared bool) {
	// 1. Safety check: If GORM hasn't parsed a schema yet, we can't determine the type.
	if db.Statement.Schema == nil {
		return false, false
	}

	// 2. Use the Schema's ModelType.
	// This is the underlying struct type (e.g., 'User') even if you passed '[]User'.
	modelType := db.Statement.Schema.ModelType
	if modelType == nil {
		return false, false
	}

	// 3. Create a pointer to the type and assert.
	// Most GORM models implement interfaces on the pointer receiver (*User).
	modelPtr := reflect.New(modelType).Interface()

	_, isTenantScoped = modelPtr.(model.TenantScoped)
	_, isShared = modelPtr.(model.TenantShared)

	return isTenantScoped, isShared
}
