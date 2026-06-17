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
// `Code` and `ParentCode` map to the `code` / `parent_code` columns.
type PermissionCatalogEntry struct {
	Code       string
	ParentCode string
	Label      string
	Actions    []string
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
	{Code: "dashboard", Label: "Dashboard"},
	{Code: "shipments", Label: "Shipments"},
	{Code: "drivers", Label: "Drivers"},
	{Code: "teams", Label: "Teams"},
	{Code: "fleet", Label: "Fleet"},
	{Code: "accounting", Label: "Accounting"},
	{Code: "customers", Label: "Customers"},
	{Code: "settings", Label: "Settings"},

	// === dashboard entities ===
	{Code: "dashboard.stats", ParentCode: "dashboard", Label: "Stats", Actions: []string{"view"}},
	{Code: "dashboard.hierarchy", ParentCode: "dashboard", Label: "Company hierarchy", Actions: []string{"view"}},

	// === shipments entities ===
	{Code: "shipments.shipments", ParentCode: "shipments", Label: "Shipments", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "shipments.legs", ParentCode: "shipments", Label: "Shipment legs", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "shipments.trips", ParentCode: "shipments", Label: "Trips", Actions: []string{"view", "edit"}},
	{Code: "shipments.trip_stops", ParentCode: "shipments", Label: "Trip stops", Actions: []string{"view", "edit"}},
	{Code: "shipments.trip_files", ParentCode: "shipments", Label: "Trip files", Actions: []string{"view", "create", "edit"}},
	{Code: "shipments.other_pay", ParentCode: "shipments", Label: "Other pay", Actions: []string{"create", "edit", "delete"}},
	{Code: "shipments.driver_expense", ParentCode: "shipments", Label: "Driver expense", Actions: []string{"create", "edit", "delete"}},
	{Code: "shipments.rc_files", ParentCode: "shipments", Label: "RC files", Actions: []string{"view", "create"}},
	{Code: "shipments.share", ParentCode: "shipments", Label: "Share links", Actions: []string{"view", "create", "delete"}},
	{Code: "shipments.audit", ParentCode: "shipments", Label: "Shipment audit", Actions: []string{"view", "edit"}},

	// === drivers entities ===
	{Code: "drivers.drivers", ParentCode: "drivers", Label: "Drivers", Actions: []string{"view", "create", "edit"}},
	{Code: "drivers.tariff_assignment", ParentCode: "drivers", Label: "Tariff assignment", Actions: []string{"view", "edit"}},
	{Code: "drivers.balance", ParentCode: "drivers", Label: "Driver balance", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "drivers.scheduled_payments", ParentCode: "drivers", Label: "Scheduled payments", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "drivers.one_time_charges", ParentCode: "drivers", Label: "One-time charges", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "drivers.weekly_deductions", ParentCode: "drivers", Label: "Weekly deductions", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "drivers.driver_absences", ParentCode: "drivers", Label: "Driver absences", Actions: []string{"view", "create", "edit", "delete"}},

	// === teams entities ===
	{Code: "teams.teams", ParentCode: "teams", Label: "Teams", Actions: []string{"view", "edit"}},
	{Code: "teams.crews", ParentCode: "teams", Label: "Crews", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "teams.dispatchers", ParentCode: "teams", Label: "Dispatchers", Actions: []string{"view", "create", "delete"}},

	// === fleet entities ===
	{Code: "fleet.trucks", ParentCode: "fleet", Label: "Trucks", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "fleet.trailers", ParentCode: "fleet", Label: "Trailers", Actions: []string{"view", "create", "edit"}},

	// === accounting entities ===
	{Code: "accounting.invoices", ParentCode: "accounting", Label: "Invoices", Actions: []string{"view", "create", "edit"}},
	{Code: "accounting.invoice_batches", ParentCode: "accounting", Label: "Invoice batches", Actions: []string{"view", "create", "edit"}},
	{Code: "accounting.credit_memos", ParentCode: "accounting", Label: "Credit memos", Actions: []string{"view", "create"}},
	{Code: "accounting.billing", ParentCode: "accounting", Label: "Billing", Actions: []string{"view"}},
	{Code: "accounting.pay_batches", ParentCode: "accounting", Label: "Pay batches", Actions: []string{"view", "create"}},
	{Code: "accounting.pay_statements", ParentCode: "accounting", Label: "Pay statements", Actions: []string{"view", "edit"}},
	{Code: "accounting.statement_trips", ParentCode: "accounting", Label: "Statement trips", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "accounting.statement_deductions", ParentCode: "accounting", Label: "Statement deductions", Actions: []string{"create", "edit", "delete"}},
	{Code: "accounting.statement_other_pay", ParentCode: "accounting", Label: "Statement other pay", Actions: []string{"create", "edit", "delete"}},
	{Code: "accounting.statement_balance_entries", ParentCode: "accounting", Label: "Statement balance entries", Actions: []string{"create", "edit", "delete"}},
	{Code: "accounting.comments", ParentCode: "accounting", Label: "Statement comments", Actions: []string{"view", "create"}},

	// === customers entities ===
	{Code: "customers.brokers", ParentCode: "customers", Label: "Brokers", Actions: []string{"view", "create"}},

	// === settings entities ===
	{Code: "settings.files", ParentCode: "settings", Label: "Company files", Actions: []string{"view"}},
	{Code: "settings.company", ParentCode: "settings", Label: "Company settings", Actions: []string{"view", "edit"}},
	{Code: "settings.doc_types", ParentCode: "settings", Label: "Document types", Actions: []string{"view", "create", "edit"}},
	{Code: "settings.team_settings", ParentCode: "settings", Label: "Team settings", Actions: []string{"view", "edit"}},
	{Code: "settings.driver_app", ParentCode: "settings", Label: "Driver app config", Actions: []string{"view", "edit"}},
	{Code: "settings.driver_tariffs", ParentCode: "settings", Label: "Driver tariffs", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "settings.load_status", ParentCode: "settings", Label: "Load status settings", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "settings.integration", ParentCode: "settings", Label: "Integrations", Actions: []string{"view", "edit"}},
	{Code: "settings.reassignment", ParentCode: "settings", Label: "Reassignment", Actions: []string{"view", "delete"}},
	{Code: "settings.reward_plans", ParentCode: "settings", Label: "Reward plans", Actions: []string{"view", "edit"}},
	{Code: "settings.accounting_types", ParentCode: "settings", Label: "Accounting types", Actions: []string{"view", "create", "edit"}},
	{Code: "settings.office_users", ParentCode: "settings", Label: "Office users", Actions: []string{"view", "create", "edit"}},
	{Code: "settings.office_roles", ParentCode: "settings", Label: "Office roles", Actions: []string{"view", "edit"}},
}

// validPermissionCodes indexes every grantable key (modules, entities, and
// leaf action rows) for O(1) validation in IsValidPermissionCode.
var validPermissionCodes = buildValidPermissionCodes()

func buildValidPermissionCodes() map[string]struct{} {
	m := make(map[string]struct{}, len(PermissionCatalog)*5)
	for _, e := range PermissionCatalog {
		m[e.Code] = struct{}{}
		for _, a := range e.Actions {
			m[e.Code+"."+a] = struct{}{}
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

// ModulePermissionCodes returns just the top-level module codes from the
// catalog (entries with no ParentCode) in declaration order. These are the
// codes the auth service grants to every role on company signup so the
// tenant has working defaults; hierarchical prefix matching covers every
// entity/action below each module.
func ModulePermissionCodes() []string {
	out := make([]string, 0, 8)
	for _, e := range PermissionCatalog {
		if e.ParentCode == "" {
			out = append(out, e.Code)
		}
	}
	return out
}
