package tmsdb

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"gorm.io/gorm"
)

// tenantConfig holds the cached interface flags for a specific struct type
type tenantConfig struct {
	isScoped bool
	isShared bool
}

// TenantScopePlugin without the typeCache
type TenantScopePlugin struct{}

func NewTenantScopePlugin() *TenantScopePlugin {
	return &TenantScopePlugin{}
}

func (t *TenantScopePlugin) Name() string {
	return "TenantScopePlugin"
}

func (t *TenantScopePlugin) Initialize(db *gorm.DB) error {
	db.Callback().Query().Before("gorm:query").Register("tenant:query", t.addTenantConditionQuery)
	db.Callback().Update().Before("gorm:update").Register("tenant:update", t.addTenantConditionWrite)
	db.Callback().Delete().Before("gorm:delete").Register("tenant:delete", t.addTenantConditionWrite)
	return nil
}

func (t *TenantScopePlugin) addTenantConditionQuery(db *gorm.DB) {
	t.applyScope(db, true)
}

func (t *TenantScopePlugin) addTenantConditionWrite(db *gorm.DB) {
	t.applyScope(db, false)
}

func (t *TenantScopePlugin) applyScope(db *gorm.DB, isRead bool) {
	fmt.Println("--- TENANT PLUGIN TRIGGERED ---")

	if db.Statement.Unscoped {
		fmt.Println("EXIT: Statement is Unscoped")
		return
	}

	var targetType reflect.Type
	if db.Statement.Model != nil {
		targetType = reflect.TypeOf(db.Statement.Model)
		fmt.Printf("TARGET: Model detected as %v\n", targetType)
	} else if db.Statement.Dest != nil {
		targetType = reflect.TypeOf(db.Statement.Dest)
		fmt.Printf("TARGET: Dest detected as %v\n", targetType)
	} else {
		fmt.Println("EXIT: Both Model and Dest are nil")
		return
	}

	// Always evaluate (Cache bypassed)
	config := t.evaluateType(targetType)
	fmt.Printf("EVALUATED: %v -> isScoped: %v, isShared: %v\n", targetType, config.isScoped, config.isShared)

	if !config.isScoped && !config.isShared {
		fmt.Println("EXIT: Type does not implement Tenant interfaces")
		return
	}

	actor, _ := middleware.GetActor(db.Statement.Context)
	if actor == nil {
		fmt.Println("EXIT: No Actor found in Context")
		return
	}

	if actor.IsSystem || actor.IsSuperAdmin() || actor.IsGuest {
		fmt.Printf("EXIT: Bypassing for privileged actor role. SuperAdmin: %v\n", actor.IsSuperAdmin())
		return
	}

	if actor.Claims.CompanyID == nil {
		fmt.Println("ERROR: Actor found but Claims.CompanyID is nil")
		db.AddError(errors.New("tenant_plugin: non-admin actor missing company_id"))
		return
	}

	tableName := db.Statement.Table
	if tableName == "" {
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
		fmt.Println("EXIT: Could not resolve table name")
		return
	}

	quotedTable := db.Statement.Quote(tableName)
	companyID := *actor.Claims.CompanyID

	fmt.Printf("SUCCESS: Applying WHERE clause for table %s with CompanyID %s\n", tableName, companyID)

	if isRead && config.isShared {
		db.Where(fmt.Sprintf("(%s.company_id = ? OR %s.is_system = ?)", quotedTable, quotedTable), companyID, true)
	} else {
		db.Where(fmt.Sprintf("%s.company_id = ?", quotedTable), companyID)
	}
}

func (t *TenantScopePlugin) evaluateType(typ reflect.Type) tenantConfig {
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return tenantConfig{isScoped: false, isShared: false}
	}

	modelPtr := reflect.New(typ).Interface()

	// Ensure the interfaces being checked exactly match the package where they are defined
	_, isScoped := modelPtr.(model.TenantScoped)
	_, isShared := modelPtr.(model.TenantShared)

	return tenantConfig{
		isScoped: isScoped,
		isShared: isShared,
	}
}
