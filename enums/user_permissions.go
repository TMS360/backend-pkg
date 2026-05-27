package enums

// UserPermissionEnum is the canonical set of permission codes recognized by
// the system. Permissions are dotted identifier strings (`module.entity.action`)
// that GraphQL `@hasPerm` directives and service-layer checks consult; this
// file is the source of truth. New permissions are added to PermissionCatalog
// below — never via a runtime DB INSERT — so the validator, the tms-auth
// seeder, and the admin UI all stay in sync.
type UserPermissionEnum string

const (
	// Sample constants that Go-side callers and tests reference directly.
	// They are intentionally a small subset — the full grantable set lives
	// in PermissionCatalog. If you need a constant for a perm not listed
	// here, prefer adding it here over passing the raw string literal.
	PermSettingsOfficeUsersView UserPermissionEnum = "settings.office_users.view"
	PermSettingsOfficeUsersEdit UserPermissionEnum = "settings.office_users.edit"
	PermSettingsCompanyEdit     UserPermissionEnum = "settings.company.edit"
	PermDriversDriversView      UserPermissionEnum = "drivers.drivers.view"
	PermDriversDriversEdit      UserPermissionEnum = "drivers.drivers.edit"
)

// PermissionCatalogEntry describes one row written to the permissions table.
// Modules carry no actions; entities carry the CRUD verbs they support.
type PermissionCatalogEntry struct {
	Key       string
	ParentKey string
	Label     string
	Actions   []string
}

// PermissionCatalog is the full source of truth for grantable permission
// keys. The tms-auth seeder writes these rows into the permissions table
// and synthesizes one leaf action row per (entity, action) pair. The runtime
// validator uses the same data so service-layer checks never drift from
// what is actually seeded.
//
// Module/entity layout mirrors frontend page boundaries (see
// /endpoints-permissions.json at the workspace root).
var PermissionCatalog = []PermissionCatalogEntry{
	// === Modules (top-level grants) ===
	{Key: "dashboard", Label: "Dashboard"},
	{Key: "shipments", Label: "Shipments"},
	{Key: "drivers", Label: "Drivers"},
	{Key: "teams", Label: "Teams"},
	{Key: "fleet", Label: "Fleet"},
	{Key: "accounting", Label: "Accounting"},
	{Key: "customers", Label: "Customers"},
	{Key: "settings", Label: "Settings"},

	// === dashboard entities ===
	{Key: "dashboard.stats", ParentKey: "dashboard", Label: "Stats", Actions: []string{"view"}},
	{Key: "dashboard.hierarchy", ParentKey: "dashboard", Label: "Company hierarchy", Actions: []string{"view"}},

	// === shipments entities ===
	{Key: "shipments.shipments", ParentKey: "shipments", Label: "Shipments", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "shipments.legs", ParentKey: "shipments", Label: "Shipment legs", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "shipments.trips", ParentKey: "shipments", Label: "Trips", Actions: []string{"view", "edit"}},
	{Key: "shipments.trip_stops", ParentKey: "shipments", Label: "Trip stops", Actions: []string{"view", "edit"}},
	{Key: "shipments.trip_files", ParentKey: "shipments", Label: "Trip files", Actions: []string{"view", "create", "edit"}},
	{Key: "shipments.other_pay", ParentKey: "shipments", Label: "Other pay", Actions: []string{"create", "edit", "delete"}},
	{Key: "shipments.driver_expense", ParentKey: "shipments", Label: "Driver expense", Actions: []string{"create", "edit", "delete"}},
	{Key: "shipments.rc_files", ParentKey: "shipments", Label: "RC files", Actions: []string{"view", "create"}},
	{Key: "shipments.share", ParentKey: "shipments", Label: "Share links", Actions: []string{"view", "create", "delete"}},
	{Key: "shipments.audit", ParentKey: "shipments", Label: "Shipment audit", Actions: []string{"view", "edit"}},

	// === drivers entities ===
	{Key: "drivers.drivers", ParentKey: "drivers", Label: "Drivers", Actions: []string{"view", "create", "edit"}},
	{Key: "drivers.tariff_assignment", ParentKey: "drivers", Label: "Tariff assignment", Actions: []string{"view", "edit"}},
	{Key: "drivers.balance", ParentKey: "drivers", Label: "Driver balance", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "drivers.scheduled_payments", ParentKey: "drivers", Label: "Scheduled payments", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "drivers.one_time_charges", ParentKey: "drivers", Label: "One-time charges", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "drivers.weekly_deductions", ParentKey: "drivers", Label: "Weekly deductions", Actions: []string{"view", "create", "edit", "delete"}},

	// === teams entities ===
	{Key: "teams.teams", ParentKey: "teams", Label: "Teams", Actions: []string{"view", "edit"}},
	{Key: "teams.crews", ParentKey: "teams", Label: "Crews", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "teams.dispatchers", ParentKey: "teams", Label: "Dispatchers", Actions: []string{"view", "create", "delete"}},

	// === fleet entities ===
	{Key: "fleet.trucks", ParentKey: "fleet", Label: "Trucks", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "fleet.trailers", ParentKey: "fleet", Label: "Trailers", Actions: []string{"view", "create", "edit"}},

	// === accounting entities ===
	{Key: "accounting.invoices", ParentKey: "accounting", Label: "Invoices", Actions: []string{"view", "create", "edit"}},
	{Key: "accounting.invoice_batches", ParentKey: "accounting", Label: "Invoice batches", Actions: []string{"view", "create", "edit"}},
	{Key: "accounting.credit_memos", ParentKey: "accounting", Label: "Credit memos", Actions: []string{"view", "create"}},
	{Key: "accounting.billing", ParentKey: "accounting", Label: "Billing", Actions: []string{"view"}},
	{Key: "accounting.pay_batches", ParentKey: "accounting", Label: "Pay batches", Actions: []string{"view", "create"}},
	{Key: "accounting.pay_statements", ParentKey: "accounting", Label: "Pay statements", Actions: []string{"view", "edit"}},
	{Key: "accounting.statement_trips", ParentKey: "accounting", Label: "Statement trips", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "accounting.statement_deductions", ParentKey: "accounting", Label: "Statement deductions", Actions: []string{"create", "edit", "delete"}},
	{Key: "accounting.statement_other_pay", ParentKey: "accounting", Label: "Statement other pay", Actions: []string{"create", "edit", "delete"}},
	{Key: "accounting.statement_balance_entries", ParentKey: "accounting", Label: "Statement balance entries", Actions: []string{"create", "edit", "delete"}},
	{Key: "accounting.comments", ParentKey: "accounting", Label: "Statement comments", Actions: []string{"view", "create"}},

	// === customers entities ===
	{Key: "customers.brokers", ParentKey: "customers", Label: "Brokers", Actions: []string{"view", "create"}},

	// === settings entities ===
	{Key: "settings.company", ParentKey: "settings", Label: "Company settings", Actions: []string{"view", "edit"}},
	{Key: "settings.doc_types", ParentKey: "settings", Label: "Document types", Actions: []string{"view", "create", "edit"}},
	{Key: "settings.driver_app", ParentKey: "settings", Label: "Driver app config", Actions: []string{"view", "edit"}},
	{Key: "settings.driver_tariffs", ParentKey: "settings", Label: "Driver tariffs", Actions: []string{"view", "create", "edit", "delete"}},
	{Key: "settings.integration", ParentKey: "settings", Label: "Integrations", Actions: []string{"view", "edit"}},
	{Key: "settings.reassignment", ParentKey: "settings", Label: "Reassignment", Actions: []string{"view", "delete"}},
	{Key: "settings.reward_plans", ParentKey: "settings", Label: "Reward plans", Actions: []string{"view", "edit"}},
	{Key: "settings.accounting_types", ParentKey: "settings", Label: "Accounting types", Actions: []string{"view", "create", "edit"}},
	{Key: "settings.office_users", ParentKey: "settings", Label: "Office users", Actions: []string{"view", "create", "edit"}},
	{Key: "settings.office_roles", ParentKey: "settings", Label: "Office roles", Actions: []string{"view"}},
	{Key: "settings.me", ParentKey: "settings", Label: "Current user", Actions: []string{"view"}},
}

// validPermissionCodes indexes every grantable key (modules, entities, and
// leaf action rows) for O(1) validation in IsValidPermissionCode.
var validPermissionCodes = buildValidPermissionCodes()

func buildValidPermissionCodes() map[string]struct{} {
	m := make(map[string]struct{}, len(PermissionCatalog)*5)
	for _, e := range PermissionCatalog {
		m[e.Key] = struct{}{}
		for _, a := range e.Actions {
			m[e.Key+"."+a] = struct{}{}
		}
	}
	return m
}

// String satisfies fmt.Stringer.
func (p UserPermissionEnum) String() string { return string(p) }

// IsValid reports whether the receiver is a known permission code.
func (p UserPermissionEnum) IsValid() bool { return IsValidPermissionCode(string(p)) }

// IsValidPermissionCode reports whether the given string matches a known
// permission code. Used by service-layer validation when accepting input
// from the assignPermissionsTo{User,Role} mutations.
func IsValidPermissionCode(code string) bool {
	_, ok := validPermissionCodes[code]
	return ok
}

// AllUserPermissions returns every grantable code in the catalog — modules,
// entities, and synthesized action leaves. Order is not stable; callers that
// need deterministic order should sort the result.
func AllUserPermissions() []string {
	out := make([]string, 0, len(validPermissionCodes))
	for k := range validPermissionCodes {
		out = append(out, k)
	}
	return out
}
