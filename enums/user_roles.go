package enums

type UserRoleEnum string

const (
	UserRoleSuperAdmin UserRoleEnum = "super_admin"
	UserRoleAdmin      UserRoleEnum = "admin"
	UserRoleDispatcher UserRoleEnum = "dispatcher"
	UserRoleDriver     UserRoleEnum = "driver"
	UserRoleCustomer   UserRoleEnum = "customer"
)

// String implements the fmt.Stringer interface
func (s UserRoleEnum) String() string {
	switch s {
	case UserRoleSuperAdmin:
		return "super_admin"
	case UserRoleAdmin:
		return "admin"
	case UserRoleDispatcher:
		return "dispatcher"
	case UserRoleDriver:
		return "driver"
	case UserRoleCustomer:
		return "customer"
	default:
		return "unknown"
	}
}

// IsValid checks if the status is a known value
func (s UserRoleEnum) IsValid() bool {
	switch s {
	case UserRoleSuperAdmin, UserRoleAdmin, UserRoleDispatcher, UserRoleDriver, UserRoleCustomer:
		return true
	default:
		return false
	}
}
