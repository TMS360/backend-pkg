package events

// Sensitivity classifies what KIND of information an audit event carries, so the
// audit read path can enforce field-level access on a per-event basis. It is
// assigned by the PRODUCING service from the event's structured shape
// (source_service, entity_type, changed fields) at emit time — never inferred on
// the read side from a rendered headline, which is how a renamed pay field would
// leak the first time someone edits its label.
type Sensitivity string

const (
	// SensitivityOperational — everyday operations any office user may read:
	// dispatch, trip status, check-in/out, miles, do-not-disturb.
	SensitivityOperational Sensitivity = "OPERATIONAL"
	// SensitivityFinancial — money: pay rate, statement approve/post,
	// adjustments, invoice, margin. Read requires accounting access.
	SensitivityFinancial Sensitivity = "FINANCIAL"
	// SensitivityHR — personnel: employment type, absence reason, termination,
	// discipline.
	SensitivityHR Sensitivity = "HR"
	// SensitivityCompliance — CDL / medical / MVR documents, DVIR defects, drug
	// tests.
	SensitivityCompliance Sensitivity = "COMPLIANCE"
	// SensitivitySecurity — session kick, permission change, IP rules.
	SensitivitySecurity Sensitivity = "SECURITY"
)

// sensitivityRank orders classes from least (0) to most restrictive. It gives
// "stricter wins" a deterministic answer when a single write touches fields of
// more than one class (an everyday field AND a money field → FINANCIAL for the
// whole event). It is NOT a claim that HR is universally "less secret" than
// FINANCIAL — only a fixed precedence so composition is stable and testable.
var sensitivityRank = map[Sensitivity]int{
	SensitivityOperational: 0,
	SensitivityHR:          1,
	SensitivityCompliance:  2,
	SensitivityFinancial:   3,
	SensitivitySecurity:    4,
}

// Valid reports whether s is one of the known classes. The empty string is NOT
// valid — it is the "unclassified" state the read side must treat as most
// restrictive and report for fixing.
func (s Sensitivity) Valid() bool {
	_, ok := sensitivityRank[s]
	return ok
}

// Stricter returns the more restrictive of two classes by sensitivityRank.
// Unknown/empty inputs rank 0 (least), so they never dominate a real class
// during producer-side composition; the read side handles "unclassified"
// separately and fails closed. This is what implements the spec's rule that on
// a multi-field write the stricter class wins for the entire event.
func Stricter(a, b Sensitivity) Sensitivity {
	if sensitivityRank[b] > sensitivityRank[a] {
		return b
	}
	return a
}
