package enums

type UserRoleEnum string

const (
	UserRoleSuperAdmin UserRoleEnum = "super_admin"
	UserRoleAdmin      UserRoleEnum = "admin"
	UserRoleManager    UserRoleEnum = "manager"
	UserRoleAccounting UserRoleEnum = "accounting"
	UserRoleSafety     UserRoleEnum = "safety"
	UserRoleFleet      UserRoleEnum = "fleet"
	UserRoleHr         UserRoleEnum = "hr"
	UserRoleDispatcher UserRoleEnum = "dispatcher"
	UserRoleDriver     UserRoleEnum = "driver"
	UserRoleCustomer   UserRoleEnum = "customer"
	UserRoleOther      UserRoleEnum = "other"
)

// String implements the fmt.Stringer interface
func (s UserRoleEnum) String() string {
	switch s {
	case UserRoleSuperAdmin:
		return "super_admin"
	case UserRoleAdmin:
		return "admin"
	case UserRoleManager:
		return "manager"
	case UserRoleAccounting:
		return "accounting"
	case UserRoleSafety:
		return "safety"
	case UserRoleFleet:
		return "fleet"
	case UserRoleHr:
		return "hr"
	case UserRoleDispatcher:
		return "dispatcher"
	case UserRoleDriver:
		return "driver"
	case UserRoleCustomer:
		return "customer"
	case UserRoleOther:
		return "other"
	default:
		return "unknown"
	}
}

// IsValid checks if the status is a known value
func (s UserRoleEnum) IsValid() bool {
	switch s {
	case UserRoleSuperAdmin, UserRoleAdmin, UserRoleManager, UserRoleAccounting, UserRoleSafety, UserRoleFleet, UserRoleHr, UserRoleDispatcher, UserRoleDriver, UserRoleCustomer, UserRoleOther:
		return true
	default:
		return false
	}
}
