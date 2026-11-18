package savedfilter

import (
	"time"

	"github.com/TMS360/backend-pkg/tmsdb/model"
	"github.com/google/uuid"
)

type SavedFilter struct {
	model.CompanyBase

	ID        uuid.UUID     `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID    uuid.UUID     `gorm:"type:uuid;not null;index"`
	Entity    string        `gorm:"column:entity;type:varchar(50);not null"`
	Name      string        `gorm:"type:varchar(100);not null"`
	Filter    model.JSONRaw `gorm:"type:jsonb;not null;default:'{}'"`
	View      model.JSONRaw `gorm:"type:jsonb;not null;default:'{}'"`
	Pinned    bool          `gorm:"type:boolean;not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (*SavedFilter) IsEntity() {}
