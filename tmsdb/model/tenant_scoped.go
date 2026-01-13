package model

import (
	"fmt"

	"github.com/TMS360/backend-pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TenantScoped is the marker interface.
// Any model implementing this will be automatically filtered by the Plugin.
type TenantScoped interface {
	IsTenantScoped() bool
}

// CompanyBase is the struct you embed in Users, Teams, Drivers, etc.
type CompanyBase struct {
	CompanyID uuid.UUID `json:"company_id" gorm:"type:uuid;not null;index"`
}

// IsTenantScoped satisfies the interface
func (cb *CompanyBase) IsTenantScoped() bool {
	return true
}

// BeforeCreate is the GORM hook that runs automatically for any model embedding CompanyBase.
func (cb *CompanyBase) BeforeCreate(tx *gorm.DB) error {
	ctx := tx.Statement.Context

	actor, _ := middleware.GetActor(ctx)
	if actor == nil {
		// Internal process (e.g., kafka consumer, cron job)
		if cb.CompanyID == uuid.Nil {
			return fmt.Errorf("system error: internal process tried to save entity without company_id")
		}
		return nil
	}

	if actor.IsSuperAdmin() {
		return nil
	}

	if actor.Claims.CompanyID == nil {
		return fmt.Errorf("actor has no company_id")
	}

	cb.CompanyID = *actor.Claims.CompanyID
	return nil
}
