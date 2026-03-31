package enums

// CustomerType enum for company customer types
type CustomerType string

const (
	CustomerTypeBroker  CustomerType = "BROKER"
	CustomerTypeShipper CustomerType = "SHIPPER"
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

// BillingType enum for billing types
type BillingType string

const (
	BillingTypeFactoringCompany BillingType = "FACTORING_COMPANY"
	BillingTypeEmail            BillingType = "EMAIL"
	BillingTypeManual           BillingType = "MANUAL"
	BillingTypeWebPortal        BillingType = "WEB_PORTAL"
	BillingTypeEdi              BillingType = "EDI"
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

type AddressType string

const (
	AddressTypePhysical AddressType = "PHYSICAL_MAILING"
	AddressBilling      AddressType = "BILLING"
)

// PaymentMethodType enum for payment methods
type PaymentMethodType string

const (
	PaymentMethodOther     PaymentMethodType = "OTHER"
	PaymentMethodBank      PaymentMethodType = "BANK"
	PaymentMethodZelle     PaymentMethodType = "ZELLE"
	PaymentMethodFactoring PaymentMethodType = "FACTORING"
	PaymentMethodCash      PaymentMethodType = "CASH"
	PaymentMethodCustomer  PaymentMethodType = "CUSTOMER"
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
	WarningTypeDoNotWork       WarningType = "DO_NOT_WORK"
	WarningTypeWorkWithCaution WarningType = "WORK_WITH_CAUTION"
	WarningTypeTemporaryHold   WarningType = "TEMPORARY_HOLD"
	WarningTypeReviewRequired  WarningType = "REVIEW_REQUIRED"
	WarningTypeBlacklist       WarningType = "BLACKLIST"
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
