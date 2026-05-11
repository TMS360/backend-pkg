package enums

import "math"

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

// UserRoleHierarchy is the canonical hierarchy for office roles. Lower value
// means higher authority. UserRoleCustomer is intentionally omitted — it is
// not part of the office hierarchy. UserRoleAdmin stays in the map so JWTs
// that already carry the role report the correct level for the
// strictly-below check used by createUser and assignPermissionsTo*.
var UserRoleHierarchy = map[UserRoleEnum]int16{
	UserRoleSuperAdmin: 0,
	UserRoleAdmin:      1,
	UserRoleManager:    2,
	UserRoleAccounting: 3,
	UserRoleHr:         3,
	UserRoleFleet:      3,
	UserRoleSafety:     3,
	UserRoleDispatcher: 3,
	UserRoleDriver:     4,
	UserRoleOther:      4,
}

// EffectiveHierarchy returns min(hierarchy) across the given role names.
// Roles not present in UserRoleHierarchy are ignored. If none of the roles
// are known, math.MaxInt16 is returned, which represents "no authority".
func EffectiveHierarchy(roles []string) int16 {
	best := int16(math.MaxInt16)
	for _, name := range roles {
		if h, ok := UserRoleHierarchy[UserRoleEnum(name)]; ok && h < best {
			best = h
		}
	}
	return best
}
