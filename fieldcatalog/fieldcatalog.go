// Package fieldcatalog is the shared, code-defined whitelist of record fields a
// board column or a report column may bind to, together with the permission
// that gates each field in its owning service.
//
// It is the single source of truth for two consumers — the board builder
// (backend-workspaces) and the reporting engine (DEV-1113) — so a board and a
// report can never disagree about what a field means or which permission guards
// it (decision D-6). It deliberately mirrors the permission catalog
// (enums.PermissionCatalog): the whitelist is a package-level slice, an unknown
// path is rejected at column creation, and adding a field is a reviewed code
// change with a test — never a runtime setting and never schema discovery.
//
// What this package does NOT contain: how a field is fetched (GraphQL selection
// sets, list queries, filter translation, pagination). That is transport detail
// private to each consumer; only the semantic contract lives here.
package fieldcatalog

import "sort"

// ValueType is the rendered type of a field's value. It mirrors the board
// builder's column types; STATUS marks a typed enum meant to render as a badge
// (as opposed to a free-text STRING).
type ValueType string

const (
	ValueString ValueType = "STRING"
	ValueNumber ValueType = "NUMBER"
	ValueDate   ValueType = "DATE"
	ValueBool   ValueType = "BOOL"
	ValueID     ValueType = "ID"
	ValueStatus ValueType = "STATUS"
)

func (v ValueType) String() string { return string(v) }

// IsValid reports whether v is one of the declared value types.
func (v ValueType) IsValid() bool {
	switch v {
	case ValueString, ValueNumber, ValueDate, ValueBool, ValueID, ValueStatus:
		return true
	default:
		return false
	}
}

// Entry is one whitelisted, bindable field.
type Entry struct {
	// RecordType is the owning record class, uppercase to match the board grain
	// / report record-type vocabulary (e.g. "TRUCK", "TRIP", "DRIVER_CREW").
	RecordType string
	// Path is the stable, record-prefixed field path a column binds to and the
	// value stored in the consumer's column config (e.g. "truck.number",
	// "trip.grossRate"). Globally unique across the catalog.
	Path string
	// Label is the human-facing column header.
	Label string
	// ValueType is the field's rendered type.
	ValueType ValueType
	// Permission is the permission code that gates reading this field in its
	// owning service (e.g. "fleet.trucks.view"). Empty means the field is
	// ungated. This is what the masking layer enforces so a board/report cannot
	// become a way around field-level access — the owning service still enforces
	// its own @hasPerm; this is the declarative mirror so the consumer knows
	// which permission masks the cell.
	Permission string
	// Sortable reports whether the owning service can sort its list by this field.
	Sortable bool
	// WriteBackKey identifies the write-back binding when a field is editable
	// through the owning service; empty means read-only. No field carries one in
	// the first version (all entries are read-only); reserved so the entry shape
	// is complete for the write-back iteration.
	WriteBackKey string
	// Held marks a field that is VERIFIED to exist in its owning schema but is
	// NOT yet safely reachable through a consumer's proven federation hops (e.g.
	// tms-teams is not a board-data hop today). Such a field is catalogued — the
	// reporting engine may still reach it — but must NOT be offered as a bindable
	// board column, so a column that looks valid never fails at fetch time. This
	// is the "flag it rather than ship a broken binding" guard, expressed as data.
	Held bool
}

// Bindable reports whether the field may be offered as a board/report column
// today: it exists and is reachable through the proven hops.
func (e Entry) Bindable() bool { return !e.Held }

// byPath indexes the catalog by Path for O(1) lookup, built once at package load
// — the same discipline as enums.validPermissionCodes.
var byPath = buildIndex()

func buildIndex() map[string]Entry {
	m := make(map[string]Entry, len(Catalog))
	for _, e := range Catalog {
		m[e.Path] = e
	}
	return m
}

// Lookup returns the catalog entry for a field path.
func Lookup(path string) (Entry, bool) {
	e, ok := byPath[path]
	return e, ok
}

// Get returns the entry only when both the record type AND the path match. It is
// the validation entry point for column creation: the boolean is false for an
// unknown path OR a path that belongs to a different record type, so the caller
// can refuse with a message naming the record type and path.
func Get(recordType, path string) (Entry, bool) {
	e, ok := byPath[path]
	if !ok || e.RecordType != recordType {
		return Entry{}, false
	}
	return e, true
}

// IsBindablePath reports whether the path is a catalogued field that may be
// bound as a column today (exists and is not Held).
func IsBindablePath(path string) bool {
	e, ok := byPath[path]
	return ok && e.Bindable()
}

// ByRecordType returns the catalog entries for one record type, in catalog order.
func ByRecordType(recordType string) []Entry {
	out := make([]Entry, 0, 8)
	for _, e := range Catalog {
		if e.RecordType == recordType {
			out = append(out, e)
		}
	}
	return out
}

// RecordTypes returns the distinct record types present in the catalog, sorted.
func RecordTypes() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, e := range Catalog {
		if _, ok := seen[e.RecordType]; !ok {
			seen[e.RecordType] = struct{}{}
			out = append(out, e.RecordType)
		}
	}
	sort.Strings(out)
	return out
}

// Entries returns a copy of the full catalog (defensive; callers must not mutate
// the source of truth).
func Entries() []Entry {
	out := make([]Entry, len(Catalog))
	copy(out, Catalog)
	return out
}
