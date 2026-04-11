package tmsdb

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"gorm.io/gorm"
)

// tenantConfig holds the cached interface flags for a specific struct type
type tenantConfig struct {
	isScoped bool
	isShared bool
}

// TenantScopePlugin enforces tenant isolation on all database queries
type TenantScopePlugin struct {
	// typeCache prevents running expensive reflection on every query.
	// Key: reflect.Type, Value: tenantConfig
	typeCache sync.Map
}

func (t *TenantScopePlugin) Name() string {
	return "TenantScopePlugin"
}

// Initialize registers the plugin with GORM
func (t *TenantScopePlugin) Initialize(db *gorm.DB) error {
	db.Callback().Query().Before("gorm:query").Register("tenant:query", t.addTenantConditionQuery)
	db.Callback().Row().Before("gorm:row").Register("tenant:row", t.addTenantConditionQuery)
	db.Callback().Raw().Before("gorm:raw").Register("tenant:raw", t.addTenantConditionQuery)

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
	// 1. Respect GORM's .Unscoped()
	if db.Statement.Unscoped {
		return
	}

	// 2. Determine target type (Prioritize Model for aggregations, fallback to Dest)
	var targetType reflect.Type
	if db.Statement.Model != nil {
		targetType = reflect.TypeOf(db.Statement.Model)
	} else if db.Statement.Dest != nil {
		targetType = reflect.TypeOf(db.Statement.Dest)
	} else {
		return // Nothing to evaluate
	}

	// 3. FAST PATH: Check the Cache (Zero memory allocation)
	var config tenantConfig
	if cached, ok := t.typeCache.Load(targetType); ok {
		config = cached.(tenantConfig)
	} else {
		// SLOW PATH: First time seeing this type. Evaluate and cache.
		config = t.evaluateType(targetType)
		t.typeCache.Store(targetType, config)
	}

	// If the model doesn't implement either interface, exit cleanly (e.g., global admin tables)
	if !config.isScoped && !config.isShared {
		return
	}

	// 4. Actor Verification
	actor, _ := middleware.GetActor(db.Statement.Context)
	if actor == nil {
		return
	}
	if actor.IsSystem || actor.IsSuperAdmin() || actor.IsGuest {
		return
	}
	if actor.Claims.CompanyID == nil {
		db.AddError(errors.New("tenant_plugin: non-admin actor missing company_id"))
		return
	}

	// 5. Safely Resolve Table Name
	tableName := db.Statement.Table
	if tableName == "" {
		// Force GORM to parse the schema so we know the table name
		if db.Statement.Model != nil {
			_ = db.Statement.Parse(db.Statement.Model)
		} else {
			_ = db.Statement.Parse(db.Statement.Dest)
		}
		if db.Statement.Schema != nil {
			tableName = db.Statement.Schema.Table
		}
	}
	if tableName == "" {
		return // Safety fallback: cannot determine table
	}

	// 6. Apply Security Clause
	quotedTable := db.Statement.Quote(tableName)
	companyID := *actor.Claims.CompanyID

	if isRead && config.isShared {
		// READ on a Shared table: User sees their company records OR system records
		db.Where(fmt.Sprintf("(%s.company_id = ? OR %s.is_system = ?)", quotedTable, quotedTable), companyID, true)
	} else {
		// WRITE on a Shared table, OR ANY operation on a strict Tenant table
		db.Where(fmt.Sprintf("%s.company_id = ?", quotedTable), companyID)
	}
}

// evaluateType unwraps pointers/slices and checks for tenant interfaces
func (t *TenantScopePlugin) evaluateType(typ reflect.Type) tenantConfig {
	// Unwrap pointers, arrays, and slices to get the base struct
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return tenantConfig{isScoped: false, isShared: false}
	}

	// Create a single pointer instance to check pointer-receiver interfaces
	modelPtr := reflect.New(typ).Interface()

	_, isScoped := modelPtr.(model.TenantScoped)
	_, isShared := modelPtr.(model.TenantShared)

	return tenantConfig{
		isScoped: isScoped,
		isShared: isShared,
	}
}
