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

	// DEV-1017 compliance documents. Mutations (upload/renew) gate on
	// PermComplianceUpload; reads gate on PermComplianceView.
	PermComplianceView   UserPermissionEnum = "compliance.view"
	PermComplianceUpload UserPermissionEnum = "compliance.upload"
	PermComplianceRenew  UserPermissionEnum = "compliance.renew"

	// Projects/Task-Management module (backend-tasks). The Task work-item
	// domain is a self-contained page, so its grants hang directly off the
	// `tasks` module rather than a sub-entity: the leaf codes are tasks.view,
	// tasks.create, tasks.assign, tasks.transition, tasks.reopen.
	PermTasksView       UserPermissionEnum = "tasks.view"
	PermTasksCreate     UserPermissionEnum = "tasks.create"
	PermTasksAssign     UserPermissionEnum = "tasks.assign"
	PermTasksTransition UserPermissionEnum = "tasks.transition"
	PermTasksReopen     UserPermissionEnum = "tasks.reopen"

	// Workspaces & custom boards module (backend-workspaces). These grants gate
	// the GraphQL surface only; board/workspace data visibility additionally
	// requires workspace membership (workspace_members roles, enforced in the
	// service layer). Entity data shown on boards is resolved through
	// apollo-router as the acting user, so the owning services' own @hasPerm
	// and tenancy still apply on top of these codes.
	PermWorkspacesView         UserPermissionEnum = "workspaces.workspaces.view"
	PermWorkspacesCreate       UserPermissionEnum = "workspaces.workspaces.create"
	PermWorkspacesEdit         UserPermissionEnum = "workspaces.workspaces.edit"
	PermWorkspacesDelete       UserPermissionEnum = "workspaces.workspaces.delete"
	PermWorkspacesBoardsView   UserPermissionEnum = "workspaces.boards.view"
	PermWorkspacesBoardsCreate UserPermissionEnum = "workspaces.boards.create"
	PermWorkspacesBoardsEdit   UserPermissionEnum = "workspaces.boards.edit"
	PermWorkspacesBoardsDelete UserPermissionEnum = "workspaces.boards.delete"
	PermWorkspacesValuesView   UserPermissionEnum = "workspaces.values.view"
	PermWorkspacesValuesEdit   UserPermissionEnum = "workspaces.values.edit"

	// PermTripFinancialsEdit (DEV-1256) gates the hand-typed trip miles /
	// gross-rate override (DEV-1257). It lives OUTSIDE the auto-granted module set
	// (see ModulePermissionCodes / FinanceModuleCode), so only the Accounting role
	// holds it by default (seeded by tms-auth); other roles receive it only via an
	// explicit custom grant. Super-admin bypasses the check as usual.
	PermTripFinancialsEdit    UserPermissionEnum = "trip_financials_edit"
	PermTripFinancialsApprove UserPermissionEnum = "trip_financials_approve"

	// PermTripReassignCommitted (DEV-1226) gates swapping/removing a trip's driver
	// AFTER the driver has accepted the trip (DriversAccepted=true). A regular
	// dispatcher can still edit other trip fields via shipments.trips.edit, but the
	// late-stage driver change is limited to holders of this flat custom permission
	// (dispatch managers, admins). Enforced inside the trip service layer, not via
	// an @hasPerm on updateTrip, because it depends on runtime trip state.
	PermTripReassignCommitted UserPermissionEnum = "trip_reassign_committed"
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
	{Code: "tasks", Label: "Tasks"},
	{Code: "workspaces", Label: "Workspaces"},

	// === tasks entities ===
	{Code: "tasks.teams", ParentCode: "tasks", Label: "Task teams", Actions: []string{"view", "create", "edit", "delete"}},

	// === workspaces entities (backend-workspaces custom boards) ===
	{Code: "workspaces.workspaces", ParentCode: "workspaces", Label: "Workspaces", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "workspaces.boards", ParentCode: "workspaces", Label: "Boards", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "workspaces.values", ParentCode: "workspaces", Label: "Board values", Actions: []string{"view", "edit"}},

	// === dashboard entities ===
	{Code: "dashboard.stats", ParentCode: "dashboard", Label: "Stats", Actions: []string{"view"}},
	{Code: "dashboard.hierarchy", ParentCode: "dashboard", Label: "Company hierarchy", Actions: []string{"view"}},

	// === shipments entities ===
	{Code: "shipments.shipments", ParentCode: "shipments", Label: "Shipments", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "shipments.legs", ParentCode: "shipments", Label: "Shipment legs", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "shipments.trips", ParentCode: "shipments", Label: "Trips", Actions: []string{"view", "edit"}},
	{Code: "shipments.trip_stops", ParentCode: "shipments", Label: "Trip stops", Actions: []string{"view", "edit"}},
	{Code: "shipments.trip_files", ParentCode: "shipments", Label: "Trip files", Actions: []string{"view", "create", "edit"}},
	{Code: "shipments.other_pay", ParentCode: "shipments", Label: "Other pay", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "shipments.driver_expense", ParentCode: "shipments", Label: "Driver expense", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "shipments.rc_files", ParentCode: "shipments", Label: "RC files", Actions: []string{"view", "create"}},
	{Code: "shipments.share", ParentCode: "shipments", Label: "Share links", Actions: []string{"view", "create", "delete"}},
	{Code: "shipments.audit", ParentCode: "shipments", Label: "Shipment audit", Actions: []string{"view", "edit"}},

	// === drivers entities ===
	{Code: "drivers.drivers", ParentCode: "drivers", Label: "Drivers", Actions: []string{"view", "create", "edit", "delete"}},
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
	{Code: "accounting.pay_batches", ParentCode: "accounting", Label: "Pay batches", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "accounting.pay_statements", ParentCode: "accounting", Label: "Pay statements", Actions: []string{"view", "create", "edit", "delete"}},
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
	{Code: "settings.reassignment", ParentCode: "settings", Label: "Reassignment", Actions: []string{"view", "create", "edit", "delete"}},
	{Code: "settings.reward_plans", ParentCode: "settings", Label: "Reward plans", Actions: []string{"view", "edit"}},
	{Code: "settings.accounting_types", ParentCode: "settings", Label: "Accounting types", Actions: []string{"view", "create", "edit"}},
	{Code: "settings.office_users", ParentCode: "settings", Label: "Office users", Actions: []string{"view", "create", "edit"}},
	{Code: "settings.office_roles", ParentCode: "settings", Label: "Office roles", Actions: []string{"view", "edit"}},
	{Code: "settings.pdf_layouts", ParentCode: "settings", Label: "Office roles", Actions: []string{"view", "edit"}},
	{Code: "settings.compliance", ParentCode: "settings", Label: "Compliance", Actions: []string{"view", "edit"}},
}

// CustomPermissionEntry is a standalone, NON-hierarchical permission: a flat
// code (no `module.entity.action` structure) that is resolved by exact match
// only. Because it carries no dots, middleware.HasPermission reduces to an
// exact-match for it — no module can imply it via prefix, and it implies
// nothing. Custom permissions live OUTSIDE PermissionCatalog (so they are never
// swept into ModulePermissionCodes → default-deny) yet are fully grantable:
// they validate, they can be assigned to roles/users, and @hasPerm checks them
// exactly like standard codes.
type CustomPermissionEntry struct {
	Code  string
	Label string
}

// CustomPermissionCatalog is the source of truth for flat, governed permissions
// that fall outside the standard module.entity.action grid. The admin UI renders
// them as standalone checkboxes (see getPermissions), and they are
// grantable/revocable per company via custom roles like any other code.
var CustomPermissionCatalog = []CustomPermissionEntry{
	{Code: string(PermTripFinancialsEdit), Label: "Edit trip miles & gross rate"},
	{Code: string(PermTripFinancialsApprove), Label: "Approve trip financial changes"},
	{Code: string(PermTripReassignCommitted), Label: "Reassign driver after trip accepted"},
}

// CustomPermissionCodes returns just the flat custom permission codes, in
// declaration order.
func CustomPermissionCodes() []string {
	out := make([]string, 0, len(CustomPermissionCatalog))
	for _, c := range CustomPermissionCatalog {
		out = append(out, c.Code)
	}
	return out
}

// IsCustomPermissionCode reports whether code is a registered flat custom
// permission (as opposed to a standard hierarchical catalog code).
func IsCustomPermissionCode(code string) bool {
	for _, c := range CustomPermissionCatalog {
		if c.Code == code {
			return true
		}
	}
	return false
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
	// Flat custom permissions are grantable too, so they must validate for the
	// assignPermissionsTo{User,Role} mutations.
	for _, c := range CustomPermissionCatalog {
		m[c.Code] = struct{}{}
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

// ModulePermissionCodes returns the top-level module codes the auth service
// grants to EVERY role on company signup (the all-modules default) so a fresh
// tenant has working defaults; hierarchical prefix matching covers every
// entity/action below each module.
//
// DEV-1256: the finance module (FinanceModuleCode) is deliberately excluded — a
// governed permission (finance.trip_miles.override) must sit OUTSIDE the set
// every role receives automatically, so nobody is swept in by this broad default.
// It is granted to Accounting by the seeder and to others only via custom roles.
func ModulePermissionCodes() []string {
	out := make([]string, 0, 8)
	for _, e := range PermissionCatalog {
		if e.ParentCode == "" {
			out = append(out, e.Code)
		}
	}
	return out
}

// DefaultRolePermissions maps each built-in role to the exact permission codes
// it receives by default at company signup (and, for existing tenants, via the
// back-fill migration). It replaces the former "every role gets every module"
// uniform grant: each role is listed explicitly, so a role's default set can
// diverge from the others.
//
// Today every office role's baseline is still the full module set
// (ModulePermissionCodes); on top of that, dispatcher and accounting receive
// their governed flat custom permission (DEV-1256) — trip_financials_edit and
// trip_financials_approve respectively. super_admin bypasses all permission
// checks, so it is intentionally omitted; a role absent from the map receives
// no default grant. Trim an individual role's slice here to narrow its defaults
// without touching any other role.
//
// Each role gets its own copy of the slice, so a caller may mutate the returned
// value without cross-contaminating other roles.
func DefaultRolePermissions() map[UserRoleEnum][]string {
	base := ModulePermissionCodes()
	withExtra := func(extra ...string) []string {
		out := make([]string, 0, len(base)+len(extra))
		out = append(out, base...)
		out = append(out, extra...)
		return out
	}
	return map[UserRoleEnum][]string{
		UserRoleAdmin:      withExtra(),
		UserRoleManager:    withExtra(string(PermTripReassignCommitted)),
		UserRoleAccounting: withExtra(string(PermTripFinancialsApprove)),
		UserRoleFleet:      withExtra(),
		UserRoleSafety:     withExtra(),
		UserRoleHr:         withExtra(),
		UserRoleDispatcher: withExtra(string(PermTripFinancialsEdit)),
		UserRoleDriver:     withExtra(),
		UserRoleOther:      withExtra(),
	}
}
