package events

import "strings"

// Classify returns the DEFAULT sensitivity for an event from its structured
// shape — (source_service, entity_type, action, changed fields). It runs on the
// PRODUCING side (tmsdb.writeEvent) so the class travels with the event and the
// read side never re-derives it from wording. A producer that knows better —
// tms-teams telling a driver absence (HR) apart from a daily status
// (OPERATIONAL) — overrides via EventBuilder.WithSensitivity.
//
// The rule is "stricter wins": money detected either from an inherently
// financial entity OR from a changed field name escalates the WHOLE event to
// FINANCIAL. That is what closes the load-feed regression where a pay figure
// rode along on an otherwise-operational shipment row.
func Classify(sourceService, entityType, action string, changedFields []string) Sensitivity {
	c := baseClass(entityType, action)
	for _, f := range changedFields {
		if isMoneyField(f) {
			c = Stricter(c, SensitivityFinancial)
		}
	}
	return c
}

// baseClass maps an entity_type (and, where it matters, its action) to a class
// before per-field escalation. Entities not listed default to OPERATIONAL — the
// safe everyday class any office user may read.
func baseClass(entityType, action string) Sensitivity {
	if financialEntities[entityType] {
		return SensitivityFinancial
	}
	if complianceEntities[entityType] {
		return SensitivityCompliance
	}
	if hrEntities[entityType] {
		return SensitivityHR
	}
	if securityEntities[entityType] || isSecurityAction(action) {
		return SensitivitySecurity
	}
	return SensitivityOperational
}

// financialEntities are inherently about money — every event on them is
// FINANCIAL regardless of which field changed.
var financialEntities = map[string]bool{
	"invoices":                  true,
	"invoice_lines":             true,
	"invoice_credit_memos":      true,
	"invoice_events":            true,
	"payments":                  true,
	"pay_batches":               true,
	"pay_batch_items":           true,
	"pay_statements":            true,
	"statement_events":          true,
	"adjustments":               true,
	"driver_tariff_assignments": true,
}

// complianceEntities carry driver-qualification / safety documents. tms-files
// owns CDL / medical / MVR qualification documents on the "files" aggregate;
// proof-of-delivery lives on tms-loads' "order_files" aggregate and stays
// OPERATIONAL (a PoD is not qualification data).
var complianceEntities = map[string]bool{
	"files": true,
}

// hrEntities are personnel lifecycle. Driver employment (termination /
// reactivation) is HR. driver_pto_events is deliberately NOT here: it carries
// both time-off (HR) and daily status (OPERATIONAL), a distinction only the
// producer can make, so tms-teams sets the class explicitly there.
var hrEntities = map[string]bool{
	"drivers": true,
}

// securityEntities cover access/identity control surfaces. None of the
// in-scope producers emit these today; the map keeps the read side honest for
// tms-auth events flowing through the shared contract later.
var securityEntities = map[string]bool{
	"sessions":         true,
	"user_permissions": true,
	"role_permissions": true,
	"ip_rules":         true,
}

func isSecurityAction(action string) bool {
	a := strings.ToLower(action)
	switch {
	case strings.Contains(a, "permission"),
		strings.Contains(a, "session_revoked"),
		strings.Contains(a, "session_kicked"),
		strings.Contains(a, "role_changed"):
		return true
	}
	return false
}

// moneyFieldTokens are substrings that mark a changed field as financial. Matched
// against structured snake_case field names (NOT prose), which is what the spec
// permits on the producing side. Kept deliberately small and specific to avoid
// false positives.
var moneyFieldTokens = []string{
	"pay", "rate", "margin", "amount", "price", "cost", "charge",
	"invoice", "tariff", "deduction", "salary", "wage", "balance",
	"revenue", "linehaul", "gross_", "_gross", "net_", "_net", "fee",
}

// isMoneyField reports whether a changed field name denotes money. Case- and
// separator-insensitive so both `load_pay` and `loadPay` are caught.
func isMoneyField(field string) bool {
	f := strings.ToLower(field)
	for _, tok := range moneyFieldTokens {
		if strings.Contains(f, tok) {
			return true
		}
	}
	return false
}
