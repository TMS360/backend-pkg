package enums

type UserStatus int

const (
	UserStatusInactive UserStatus = iota
	UserStatusActive
	UserStatusBanned
)

// String implements the fmt.Stringer interface
func (s UserStatus) String() string {
	switch s {
	case UserStatusInactive:
		return "inactive"
	case UserStatusActive:
		return "active"
	case UserStatusBanned:
		return "banned"
	default:
		return "unknown"
	}
}

// IsValid checks if the status is a known value
func (s UserStatus) IsValid() bool {
	switch s {
	case UserStatusActive, UserStatusInactive, UserStatusBanned:
		return true
	default:
		return false
	}
}
