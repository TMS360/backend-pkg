package enums

import "sort"

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

	// DEV-1466 compliance documents — ONE grantable vocabulary, hierarchical under
	// the `settings` module so a `settings` grant implies both leaves (exactly like
	// every other settings.* entity). These constants MUST equal the
	// PermissionCatalog leaves for `settings.compliance` below.
	//
	//   settings.compliance.view — read the compliance dashboard: entity files,
	//                              cards, rollup status, and reminder mutes.
	//   settings.compliance.edit — upload, renew, delete, and mute/unmute reminders.
	//                              There is deliberately NO separate
	//                              upload/renew/configure leaf: edit covers them all.
	//
	// The earlier flat `compliance.view|upload|renew` codes and the FE-only
	// `compliance.configure|self-view` names are intentionally gone — none of them
	// were ever in the catalog, so they could never be granted or satisfied.
	// Office-user self-service has no leaf yet (not shipped): do not reintroduce a
	// `self-view` phantom; add a real catalog leaf if/when product commits to it.
	PermComplianceView UserPermissionEnum = "settings.compliance.view"
	PermComplianceEdit UserPermissionEnum = "settings.compliance.edit"

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
	// workspaces.values gates CELL-LEVEL access — the stored value inside a board
	// cell — as distinct from workspaces.boards, which gates the board STRUCTURE
	// (creating boards, adding/removing/retyping columns, groups, views). The two
	// separate cleanly: a user may be allowed to type into cells without being
	// allowed to reshape the board, or the reverse.
	//   - workspaces.values.edit authorises WRITING and CLEARING a cell; it is what
	//     the cell mutation enforces (backend-workspaces upsertBoardValue). It is
	//     also the board-side half of the write-back correction authority (DEC-B):
	//     correcting a value that writes back to an owning record needs BOTH this
	//     and the owning service's own write permission.
	//   - workspaces.values.view is the cell-level READ gate; the board read path
	//     currently admits cell values under workspaces.boards.view, so this action
	//     is reserved for a finer read split.
	PermWorkspacesValuesView UserPermissionEnum = "workspaces.values.view"
	PermWorkspacesValuesEdit UserPermissionEnum = "workspaces.values.edit"
	// workspaces.templates gates the ready-made board TEMPLATE catalog (DEV-1385):
	//   - workspaces.templates.view lists the templates (getBoardTemplates);
	//   - workspaces.templates.create instantiates one into a workspace
	//     (createBoardFromTemplate) — so .view ALONE cannot instantiate;
	//   - workspaces.templates.edit is reserved for future template authoring.
	// Granting the whole "workspaces" module implies all of these via hierarchical
	// prefix matching — no separate grant is needed.
	PermWorkspacesTemplatesView   UserPermissionEnum = "workspaces.templates.view"
	PermWorkspacesTemplatesCreate UserPermissionEnum = "workspaces.templates.create"
	PermWorkspacesTemplatesEdit   UserPermissionEnum = "workspaces.templates.edit"

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

	// PermFileDeleteAny gates deleting a file attachment the actor did not upload,
	// or one whose uploader window has closed (deleteUserFile in tms-auth,
	// deleteOrderFile in tms-loads). Without it an actor may only remove their OWN
	// upload, and only inside the delete window.
	//
	// It MUST be a flat custom permission, not a `module.entity.delete` code: every
	// built-in role — driver included — is seeded the full module baseline
	// (see DefaultRolePermissions / ModulePermissionCodes), and HasPermission
	// matches hierarchically, so holding "shipments" would satisfy
	// "shipments.trip_files.delete". A dotted code therefore cannot express
	// "office only". A flat code carries no dots, so it resolves by exact match and
	// is default-deny for everyone it is not explicitly seeded to.
	//
	// Enforced in the service layer rather than via @hasPerm, because the rule
	// depends on runtime row state (uploader identity + row age).
	PermFileDeleteAny UserPermissionEnum = "file_delete_any"

	// PermReportsRun / PermReportsManage gate the Dynamic Report Builder
	// (BL-22 §22, epic DEV-1111). run = execute a saved report config and export
	// it to CSV; manage = create / edit / delete report configs and read the
	// execution/export audit log.
	//
	// Both are FLAT custom permissions (no dots), exactly like PermFileDeleteAny:
	// a flat code resolves by EXACT match, so it is default-deny for every role it
	// is not explicitly seeded to and can never be satisfied via a module prefix.
	// That is the only shape that can honour BL-22 §22.1 "the broker portal role
	// must never be granted reports.run" — a hierarchical top-level `reports`
	// module would be swept into ModulePermissionCodes and auto-granted to EVERY
	// role at signup (the exact thing §22.1 forbids). Seeded to Admin + Accounting
	// only (see DefaultRolePermissions); external/portal roles (customer) get
	// neither, by construction.
	//
	// Financial columns inside a report are additionally masked by data
	// provenance: the reporting engine keys each money column on the OWNING
	// service's view permission (Phase-1 settlement data → accounting.pay_statements.view)
	// rather than a bare `accounting.view`, so a holder of reports_run without the
	// underlying accounting-view sees those columns as "—", never a raw number.
	PermReportsRun    UserPermissionEnum = "reports_run"
	PermReportsManage UserPermissionEnum = "reports_manage"
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
	// workspaces.values gates the cell VALUE (read/edit), not the board structure
	// (that is workspaces.boards). See the PermWorkspacesValues* constants above.
	{Code: "workspaces.values", ParentCode: "workspaces", Label: "Board values", Actions: []string{"view", "edit"}},
	// workspaces.templates gates the board-template catalog: view lists templates,
	// create instantiates one, edit is reserved for template authoring (DEV-1385).
	{Code: "workspaces.templates", ParentCode: "workspaces", Label: "Board templates", Actions: []string{"view", "create", "edit"}},

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
	{Code: "settings.pdf_layouts", ParentCode: "settings", Label: "PDF layouts", Actions: []string{"view", "edit"}},
	// DEV-1466: the ONLY compliance vocabulary. `view` = read dashboard/list/cards/
	// status/mutes; `edit` = upload/renew/delete/mute. A `settings` module grant
	// implies both leaves via HasPermission prefix-matching (see PermComplianceView /
	// PermComplianceEdit), so no separate per-role leaf seeding is required.
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
	// Registered and grantable via custom roles, but NOT default-seeded to any
	// role yet — reserved for a future maker-checker approval flow (see
	// DefaultRolePermissions).
	{Code: string(PermTripFinancialsApprove), Label: "Approve trip financial changes"},
	{Code: string(PermTripReassignCommitted), Label: "Reassign driver after trip accepted"},
	{Code: string(PermFileDeleteAny), Label: "Delete any uploaded file"},
	{Code: string(PermReportsRun), Label: "Run & export reports"},
	{Code: string(PermReportsManage), Label: "Manage report configs & view report audit log"},
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

// catalogIndex is the derived, read-only view of PermissionCatalog that
// ExpandPermissions / RollupPermissions consult. It is built once at package
// init (permIndex) so neither function re-scans the catalog per call. The two
// maps double as the module/entity sets — a code is a module iff it keys
// moduleEntities, an entity iff it keys entityLeaves — so there is one source
// of truth per fact.
type catalogIndex struct {
	moduleEntities map[string][]string // module code -> its entity codes
	entityLeaves   map[string][]string // entity code -> its full leaf codes (entity.action)
	entityModule   map[string]string   // entity code -> its module code
}

func (ix catalogIndex) isModule(code string) bool { _, ok := ix.moduleEntities[code]; return ok }
func (ix catalogIndex) isEntity(code string) bool { _, ok := ix.entityLeaves[code]; return ok }

var permIndex = buildCatalogIndex()

func buildCatalogIndex() catalogIndex {
	ix := catalogIndex{
		moduleEntities: make(map[string][]string),
		entityLeaves:   make(map[string][]string),
		entityModule:   make(map[string]string),
	}
	for _, e := range PermissionCatalog {
		if e.ParentCode == "" {
			// Register the module even if it has no entities yet, so isModule
			// recognises it.
			if _, ok := ix.moduleEntities[e.Code]; !ok {
				ix.moduleEntities[e.Code] = nil
			}
			continue
		}
		ix.entityModule[e.Code] = e.ParentCode
		ix.moduleEntities[e.ParentCode] = append(ix.moduleEntities[e.ParentCode], e.Code)
		leaves := make([]string, 0, len(e.Actions))
		for _, a := range e.Actions {
			leaves = append(leaves, e.Code+"."+a)
		}
		ix.entityLeaves[e.Code] = leaves
	}
	return ix
}

// ExpandPermissions returns the concrete leaf codes implied by codes, per
// PermissionCatalog: a module code expands to every leaf of its entities, an
// entity code expands to its own leaves, and any other code (an already-leaf
// action, a flat custom code, or an unrecognised code) is kept unchanged. The
// result is deduplicated and sorted. It never drops an input grant, so it is
// the safe way to bring a mixed set of module/entity/leaf codes to a single
// granularity before set algebra (merge, diff).
func ExpandPermissions(codes []string) []string {
	set := make(map[string]struct{}, len(codes)*4)
	for _, c := range codes {
		switch {
		case c == "":
			// skip
		case permIndex.isModule(c):
			for _, ent := range permIndex.moduleEntities[c] {
				for _, leaf := range permIndex.entityLeaves[ent] {
					set[leaf] = struct{}{}
				}
			}
		case permIndex.isEntity(c):
			for _, leaf := range permIndex.entityLeaves[c] {
				set[leaf] = struct{}{}
			}
		default:
			set[c] = struct{}{}
		}
	}
	return sortedKeys(set)
}

// RollupPermissions is the inverse of ExpandPermissions: it replaces a complete
// set of children with their single parent code. If every catalog leaf of an
// entity is held, the entity code is emitted instead of the leaves; if every
// entity of a module is held, the module code is emitted instead of the
// entities. A code held via an already-present ancestor (e.g. an
// `accounting.invoices.view` leaf when `accounting` is present) is dropped as
// redundant — so this subsumes the old prefix-only CompactHierarchy. Codes with
// no catalog parent (flat custom codes, unrecognised codes) are preserved as-is.
// The result is deduplicated and sorted and never drops a grant.
func RollupPermissions(codes []string) []string {
	present := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		if c != "" {
			present[c] = struct{}{}
		}
	}

	// An entity is complete when every one of its leaves is held (present, or
	// implied by a present ancestor); a module is complete when every one of its
	// entities is complete.
	completeEntities := make(map[string]struct{})
	for ent, leaves := range permIndex.entityLeaves {
		if len(leaves) > 0 && allImpliedBy(leaves, present) {
			completeEntities[ent] = struct{}{}
		}
	}
	completeModules := make(map[string]struct{})
	for mod, ents := range permIndex.moduleEntities {
		if len(ents) > 0 && allIn(ents, completeEntities) {
			completeModules[mod] = struct{}{}
		}
	}

	// The rolled-up set is the minimal representatives — complete modules, plus
	// complete entities whose module isn't itself complete — together with every
	// present code they don't already cover (leaves of incomplete entities, flat
	// custom codes, unknown codes). Building it this way guarantees no grant is
	// dropped: a present code is emitted unless a representative implies it.
	out := make(map[string]struct{}, len(present))
	for mod := range completeModules {
		out[mod] = struct{}{}
	}
	for ent := range completeEntities {
		if _, moduleComplete := completeModules[permIndex.entityModule[ent]]; !moduleComplete {
			out[ent] = struct{}{}
		}
	}
	for c := range present {
		if !impliedBy(c, out) {
			out[c] = struct{}{}
		}
	}
	return sortedKeys(out)
}

// impliedBy reports whether code, or any of its dotted-prefix ancestors, is a
// member of set — the same self-or-ancestor rule middleware.HasPermission uses
// (`accounting` implies `accounting.invoices.view`). It scans dot boundaries in
// place, so it allocates nothing (it runs once per catalog leaf per rollup).
func impliedBy(code string, set map[string]struct{}) bool {
	if _, ok := set[code]; ok {
		return true
	}
	for i := len(code) - 1; i > 0; i-- {
		if code[i] == '.' {
			if _, ok := set[code[:i]]; ok {
				return true
			}
		}
	}
	return false
}

func allImpliedBy(codes []string, set map[string]struct{}) bool {
	for _, c := range codes {
		if !impliedBy(c, set) {
			return false
		}
	}
	return true
}

func allIn(codes []string, set map[string]struct{}) bool {
	for _, c := range codes {
		if _, ok := set[c]; !ok {
			return false
		}
	}
	return true
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
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
		// DEV-1256 / BL §7.5: hand-editing trip miles & gross rate
		// (trip_financials_edit) is held by default by admin and accounting only.
		// A regular dispatcher does NOT get it (a custom role may add it later).
		// trip_financials_approve is intentionally NOT seeded to any role — it stays
		// registered/grantable for a future maker-checker flow, but seeding it now
		// would leave accounting with approve-and-no-edit, the exact divergence this
		// matrix fixes.
		// DEV-1227 / BL §7.5: reassigning a committed trip's driver
		// (trip_reassign_committed) is held by default by manager AND admin.
		// A regular dispatcher does NOT get it (a custom role may add it later).
		// file_delete_any: removing someone else's upload (or one past the uploader
		// window) is an office-supervisor action, so it goes to admin and manager
		// only. Every other role — driver included — falls back to the
		// "own upload, inside the window" path.
		// reports_run / reports_manage (BL-22 §22.1): the report builder is an
		// admin + accounting capability. Both are flat custom perms, so every other
		// role — including any external/portal (customer) role — is default-deny.
		UserRoleAdmin:      withExtra(string(PermTripFinancialsEdit), string(PermTripReassignCommitted), string(PermFileDeleteAny), string(PermReportsRun), string(PermReportsManage)),
		UserRoleManager:    withExtra(string(PermTripReassignCommitted), string(PermFileDeleteAny)),
		UserRoleAccounting: withExtra(string(PermTripFinancialsEdit), string(PermReportsRun), string(PermReportsManage)),
		UserRoleFleet:      withExtra(),
		UserRoleSafety:     withExtra(),
		UserRoleHr:         withExtra(),
		// DEV-1409: the auditor gets the same module baseline as its peers. Its
		// governed powers (locked-statement corrections, revised-invoice voids,
		// reading the tenant audit log) are gated by role, not by a permission
		// code, so there is nothing extra to seed here.
		UserRoleAuditor:    withExtra(),
		UserRoleDispatcher: withExtra(),
		UserRoleDriver:     withExtra(),
		UserRoleOther:      withExtra(),
	}
}
