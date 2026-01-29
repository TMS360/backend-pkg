package enums

import "fmt"

// CustomerType enum for company customer types
type CustomerType string

const (
	CustomerTypeBroker  CustomerType = "Broker"
	CustomerTypeShipper CustomerType = "Shipper"
)

// IsValid checks if the customer type is valid
func (c CustomerType) IsValid() bool {
	switch c {
	case CustomerTypeBroker, CustomerTypeShipper:
		return true
	default:
		return false
	}
}

// String returns string representation
func (c CustomerType) String() string {
	return string(c)
}

// CompanyStatus enum for company status
type CompanyStatus string

const (
	CompanyStatusInactive CompanyStatus = "Inactive"
	CompanyStatusActive   CompanyStatus = "Active"
	CompanyStatusBlocked  CompanyStatus = "Blocked"
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

// BillingType enum for billing types
type BillingType string

const (
	BillingTypeFactoringCompany BillingType = "Factoring company"
	BillingTypeEmail            BillingType = "Email"
	BillingTypeManual           BillingType = "Manual"
	BillingTypeWebPortal        BillingType = "Web-site portal"
	BillingTypeEdi              BillingType = "Edi"
)

// IsValid checks if the billing type is valid
func (b BillingType) IsValid() bool {
	switch b {
	case BillingTypeFactoringCompany, BillingTypeEmail, BillingTypeManual, BillingTypeWebPortal, BillingTypeEdi:
		return true
	default:
		return false
	}
}

// String returns string representation
func (b BillingType) String() string {
	return string(b)
}

// PaymentMethodType enum for payment methods
type PaymentMethodType string

const (
	PaymentMethodOther     PaymentMethodType = "Other"
	PaymentMethodBank      PaymentMethodType = "Bank"
	PaymentMethodZelle     PaymentMethodType = "Zelle"
	PaymentMethodFactoring PaymentMethodType = "Factoring"
	PaymentMethodCash      PaymentMethodType = "Cash"
	PaymentMethodCustomer  PaymentMethodType = "Customer"
)

// IsValid checks if the payment method is valid
func (p PaymentMethodType) IsValid() bool {
	switch p {
	case PaymentMethodOther, PaymentMethodBank, PaymentMethodZelle,
		PaymentMethodFactoring, PaymentMethodCash, PaymentMethodCustomer:
		return true
	default:
		return false
	}
}

// String returns string representation
func (p PaymentMethodType) String() string {
	return string(p)
}

// WarningType enum for warning types
type WarningType string

const (
	WarningTypeDoNotWork       WarningType = "Do Not Work!"
	WarningTypeWorkWithCaution WarningType = "Work with Caution"
	WarningTypeTemporaryHold   WarningType = "Temporary Hold"
	WarningTypeReviewRequired  WarningType = "Review Required"
	WarningTypeBlacklist       WarningType = "Blacklist"
)

// IsValid checks if the warning type is valid
func (w WarningType) IsValid() bool {
	switch w {
	case WarningTypeDoNotWork, WarningTypeWorkWithCaution, WarningTypeTemporaryHold,
		WarningTypeReviewRequired, WarningTypeBlacklist:
		return true
	default:
		return false
	}
}

// String returns string representation
func (w WarningType) String() string {
	return string(w)
}

// Scan implements the sql.Scanner interface for CustomerType
func (c *CustomerType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		*c = CustomerType(v)
	case []byte:
		*c = CustomerType(v)
	default:
		return fmt.Errorf("cannot scan %T into CustomerType", value)
	}
	return nil
}

// Scan implements the sql.Scanner interface for CompanyStatus
func (s *CompanyStatus) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		*s = CompanyStatus(v)
	case []byte:
		*s = CompanyStatus(v)
	default:
		return fmt.Errorf("cannot scan %T into CompanyStatus", value)
	}
	return nil
}

// Scan implements the sql.Scanner interface for BillingType
func (b *BillingType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		*b = BillingType(v)
	case []byte:
		*b = BillingType(v)
	default:
		return fmt.Errorf("cannot scan %T into BillingType", value)
	}
	return nil
}

// Scan implements the sql.Scanner interface for PaymentMethodType
func (p *PaymentMethodType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		*p = PaymentMethodType(v)
	case []byte:
		*p = PaymentMethodType(v)
	default:
		return fmt.Errorf("cannot scan %T into PaymentMethodType", value)
	}
	return nil
}

// Scan implements the sql.Scanner interface for WarningType
func (w *WarningType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		*w = WarningType(v)
	case []byte:
		*w = WarningType(v)
	default:
		return fmt.Errorf("cannot scan %T into WarningType", value)
	}
	return nil
}
