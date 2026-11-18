package enums

import (
	"database/sql/driver"
	"fmt"
)

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

func (s UserStatus) Value() (driver.Value, error) {
	return int64(s), nil
}

func (s *UserStatus) Scan(v any) error {
	if v == nil {
		*s = UserStatusActive
		return nil
	}
	switch x := v.(type) {
	case int64:
		*s = UserStatus(x)
	case int:
		*s = UserStatus(x)
	default:
		return fmt.Errorf("cannot scan %T into UserStatus", v)
	}
	return nil
}
