package model

import (
	"fmt"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TenantShared interface {
	IsSharedTenantModel() bool
}

type SharedTenantBase struct {
	CompanyID *uuid.UUID `json:"company_id" gorm:"type:uuid;index" mapstructure:"company_id"`
	IsSystem  bool       `json:"is_system" gorm:"not null;default:false" mapstructure:"is_system"`
}

func (stb *SharedTenantBase) IsSharedTenantModel() bool {
	return true
}

func (stb *SharedTenantBase) BeforeCreate(tx *gorm.DB) error {
	ctx := tx.Statement.Context
	actor, _ := middleware.GetActor(ctx)

	// -------------------------------------------------------------
	// 1. Internal/System Processes (Cron jobs, Seeders, Kafka)
	// -------------------------------------------------------------
	if actor == nil || actor.IsSystem {
		// We cannot inject a CompanyID, but we MUST defensively validate
		// that the developer's Go code constructed a valid state.
		if stb.IsSystem && stb.CompanyID != nil {
			return fmt.Errorf("system integrity error: internal process tried to create a system record with a company_id")
		}
		if !stb.IsSystem && stb.CompanyID == nil {
			return fmt.Errorf("system integrity error: internal process tried to create a tenant record without a company_id")
		}
		return nil // State is valid, proceed with insert
	}

	// -------------------------------------------------------------
	// 2. Handle System Records (HTTP Requests)
	// -------------------------------------------------------------
	if stb.IsSystem {
		if !actor.IsSuperAdmin() {
			return fmt.Errorf("security error: only super admins can create system records")
		}
		// STRICT ENFORCEMENT: System records MUST NOT have a company ID.
		stb.CompanyID = nil
		return nil
	}

	// -------------------------------------------------------------
	// 3. Handle Tenant Records (HTTP Requests)
	// -------------------------------------------------------------
	if actor.Claims.CompanyID == nil {
		if actor.IsSuperAdmin() {
			// A SuperAdmin is creating a tenant record but forgot to specify which tenant.
			return fmt.Errorf("superadmin must explicitly provide company_id for non-system records")
		}
		return fmt.Errorf("security error: actor missing company_id")
	}

	// STRICT ENFORCEMENT: Force the standard user's CompanyID to prevent cross-tenant injection.
	// This silently overwrites any malicious payload.
	stb.CompanyID = actor.Claims.CompanyID

	return nil
}
