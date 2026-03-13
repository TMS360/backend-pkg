package model

type TenantShared interface {
	IsSharedTenantModel() bool
}

type SharedTenantBase struct {
	IsSystem bool `json:"is_system" gorm:"not null;default:false" mapstructure:"is_system"`
}

func (stb *SharedTenantBase) IsSharedTenantModel() bool {
	return true
}
