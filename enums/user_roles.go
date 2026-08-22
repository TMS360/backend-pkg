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
	UserRoleAuditor    UserRoleEnum = "auditor"
	UserRoleDispatcher UserRoleEnum = "dispatcher"
	// UserRoleTrackAndTrace (DEV-1824, BL-4 §4.17) is a dispatcher-equivalent
	// office role for Track & Trace staff. It exists precisely BECAUSE it is not
	// named "dispatcher": tms-teams reacts to the dispatcher role name by
	// creating a team and listing the user in the "add dispatcher to team"
	// pickers, and Track & Trace staff belong in neither. The name string is
	// therefore part of the contract — renaming it silently changes access.
	UserRoleTrackAndTrace UserRoleEnum = "track_and_trace"
	UserRoleDriver        UserRoleEnum = "driver"
	UserRoleCustomer      UserRoleEnum = "customer"
	UserRoleOther         UserRoleEnum = "other"

	// UserRoleBrokerAdmin / UserRoleBrokerUser (DEV-1857, BL-20) belong to the
	// broker portal — the third login surface, next to office web and driver
	// mobile — and NOT to a carrier office. They are deliberately absent from
	// UserRoleHierarchy: the hierarchy is the office authority ladder that
	// createUser and assignPermissionsTo* compare against, and a broker has no
	// standing on it (a broker admin must never be able to create or
	// re-permission a carrier's office user, and must never appear in an office
	// role picker). Use IsBrokerRole to tell the two families apart.
	UserRoleBrokerAdmin UserRoleEnum = "broker_admin"
	UserRoleBrokerUser  UserRoleEnum = "broker_user"
)

// BrokerRoles is the broker-portal role family, in descending authority.
var BrokerRoles = []UserRoleEnum{UserRoleBrokerAdmin, UserRoleBrokerUser}

// IsBrokerRole reports whether a role name belongs to the broker portal. It
// takes a plain string because callers usually hold JWT claim strings.
func IsBrokerRole(name string) bool {
	for _, r := range BrokerRoles {
		if string(r) == name {
			return true
		}
	}
	return false
}

// HasBrokerRole reports whether any of the given role names is a broker role —
// the check that decides a session is a broker session.
func HasBrokerRole(names []string) bool {
	for _, n := range names {
		if IsBrokerRole(n) {
			return true
		}
	}
	return false
}

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
	case UserRoleAuditor:
		return "auditor"
	case UserRoleDispatcher:
		return "dispatcher"
	case UserRoleTrackAndTrace:
		return "track_and_trace"
	case UserRoleDriver:
		return "driver"
	case UserRoleCustomer:
		return "customer"
	case UserRoleOther:
		return "other"
	case UserRoleBrokerAdmin:
		return "broker_admin"
	case UserRoleBrokerUser:
		return "broker_user"
	default:
		return "unknown"
	}
}

// IsValid checks if the status is a known value
func (s UserRoleEnum) IsValid() bool {
	switch s {
	case UserRoleSuperAdmin, UserRoleAdmin, UserRoleManager, UserRoleAccounting, UserRoleSafety, UserRoleFleet, UserRoleHr, UserRoleAuditor, UserRoleDispatcher, UserRoleTrackAndTrace, UserRoleDriver, UserRoleCustomer, UserRoleOther,
		UserRoleBrokerAdmin, UserRoleBrokerUser:
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
var UserRoleHierarchy = map[UserRoleEnum]int32{
	UserRoleSuperAdmin: 0,
	UserRoleAdmin:      1,
	UserRoleManager:    2,
	UserRoleAccounting: 3,
	UserRoleHr:         3,
	UserRoleFleet:      3,
	UserRoleSafety:     3,
	// The auditor's authority is governance over records (correcting a locked
	// statement, voiding a revised invoice, reading the audit log), not authority
	// over people — so it sits in the specialist band with its peers rather than
	// above them. Band 3 cannot create or re-permission band-3 users, which is
	// the separation of duties an auditor should have.
	UserRoleAuditor:    3,
	UserRoleDispatcher: 3,
	// DEV-1824: Track & Trace has the dispatcher's authority, so it shares the
	// dispatcher's band — anything else would make the two roles behave
	// differently in the strictly-below check of createUser /
	// assignPermissionsTo*.
	UserRoleTrackAndTrace: 3,
	UserRoleDriver:        4,
	UserRoleOther:         4,
}

// EffectiveHierarchy returns min(hierarchy) across the given role names.
// Roles not present in UserRoleHierarchy are ignored. If none of the roles
// are known, math.MaxInt16 is returned, which represents "no authority".
func EffectiveHierarchy(roles []string) int32 {
	best := int32(math.MaxInt32)
	for _, name := range roles {
		if h, ok := UserRoleHierarchy[UserRoleEnum(name)]; ok && h < best {
			best = h
		}
	}
	return best
}
