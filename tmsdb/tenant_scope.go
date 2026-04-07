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
	if db.Statement.Unscoped {
		return
	}

	isTenantScoped, isShared := checkTenantInterfaces(db)

	if !isTenantScoped && !isShared {
		return
	}

	actor, _ := middleware.GetActor(db.Statement.Context)
	if actor == nil || actor.IsSystem || actor.IsSuperAdmin() || actor.IsGuest {
		return
	}

	if actor.Claims.CompanyID == nil {
		db.AddError(errors.New("tenant_plugin: non-admin actor missing company_id"))
		return
	}

	// 1. SAFELY RESOLVE TABLE NAME
	tableName := db.Statement.Table
	if tableName == "" {
		// Force GORM to parse the schema based on the Model first, then Dest
		if db.Statement.Model != nil {
			_ = db.Statement.Parse(db.Statement.Model)
		} else if db.Statement.Dest != nil {
			_ = db.Statement.Parse(db.Statement.Dest)
		}
		if db.Statement.Schema != nil {
			tableName = db.Statement.Schema.Table
		}
	}

	if tableName == "" {
		return // Cannot determine table safely
	}

	quotedTable := db.Statement.Quote(tableName)
	companyID := *actor.Claims.CompanyID

	if isRead && isShared {
		db.Where(fmt.Sprintf("(%s.company_id = ? OR %s.is_system = ?)", quotedTable, quotedTable), companyID, true)
	} else {
		db.Where(fmt.Sprintf("%s.company_id = ?", quotedTable), companyID)
	}
}

func checkTenantInterfaces(db *gorm.DB) (isTenantScoped bool, isShared bool) {
	var targetType reflect.Type

	// 1. THE FIX: Prioritize explicitly set .Model() (Crucial for .Scan aggregations)
	if db.Statement.Model != nil {
		targetType = reflect.TypeOf(db.Statement.Model)
	} else if db.Statement.Dest != nil {
		targetType = reflect.TypeOf(db.Statement.Dest) // Fallback for standard queries
	} else {
		return false, false
	}

	// 2. Unwrap Pointers, Slices, and Arrays to get the base Struct
	for targetType.Kind() == reflect.Ptr || targetType.Kind() == reflect.Slice || targetType.Kind() == reflect.Array {
		targetType = targetType.Elem()
	}

	if targetType.Kind() != reflect.Struct {
		return false, false
	}

	// 3. Create pointer and assert interfaces
	modelPtr := reflect.New(targetType).Interface()

	_, isTenantScoped = modelPtr.(model.TenantScoped)
	_, isShared = modelPtr.(model.TenantShared)

	return isTenantScoped, isShared
}
