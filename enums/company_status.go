package enums

// CompanyStatus enum for company status
type CompanyStatus string

const (
	CompanyStatusInactive CompanyStatus = "INACTIVE"
	CompanyStatusActive   CompanyStatus = "ACTIVE"
	CompanyStatusBlocked  CompanyStatus = "BLOCKED"
)

// IsValid checks if the company status is valid
func (s CompanyStatus) IsValid() bool {
	switch s {
	case CompanyStatusInactive, CompanyStatusActive, CompanyStatusBlocked:
		return true
	default:
		return false
	}
}

// String returns string representation
func (s CompanyStatus) String() string {
	return string(s)
}
