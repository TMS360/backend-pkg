package tmsdb

import (
	"fmt"

	"github.com/TMS360/backend-pkg/response"
)

// ============================================================================
// COLUMN REGISTRY
//
// A list declares its columns once — client name, SQL, type — and gets both
// generic per-column filtering and the sort whitelist out of that single
// table. The typed filter input each list already has keeps working unchanged;
// column filters are an additional AND on top of it.
// ============================================================================

// ColumnType picks which of the existing typed filters a column accepts.
type ColumnType string

const (
	ColumnText   ColumnType = "text"
	ColumnNumber ColumnType = "number"
	ColumnDate   ColumnType = "date"
	ColumnID     ColumnType = "id"
	ColumnBool   ColumnType = "bool"
)

// Column is one filterable and sortable column of a list.
//
// Field is the name the client sends (camelCase, e.g. "createdAt"). SQL is
// what the database is asked for: a column ("created_at"), a qualified column
// ("t.created_at"), or an expression the list already joins in — it never
// comes from the client, which is what keeps the whitelist meaningful.
type Column struct {
	Field string
	SQL   string
	Type  ColumnType
}

// ColumnRegistry is a list's column table, built once (usually a package-level
// var next to the repository) and shared by every request.
type ColumnRegistry struct {
	byField map[string]Column
	sort    map[string]string
}

// NewColumnRegistry builds the registry. Columns with an empty Field or SQL are
// dropped: a half-declared column must not become a filter nobody whitelisted.
func NewColumnRegistry(columns ...Column) *ColumnRegistry {
	r := &ColumnRegistry{
		byField: make(map[string]Column, len(columns)),
		sort:    make(map[string]string, len(columns)),
	}
	for _, c := range columns {
		if c.Field == "" || c.SQL == "" {
			continue
		}
		r.byField[c.Field] = c
		r.sort[c.Field] = c.SQL
	}
	return r
}

// Lookup resolves a client field name to its column. Like ApplySort it accepts
// both spellings: "created_at" finds the column registered as "createdAt".
func (r *ColumnRegistry) Lookup(field string) (Column, bool) {
	if r == nil {
		return Column{}, false
	}
	if c, ok := r.byField[field]; ok {
		return c, true
	}
	c, ok := r.byField[snakeToCamel(field)]
	return c, ok
}

// SortFields returns the allow-list for ApplySort, so filtering and sorting can
// never drift apart: pass it as `fb.ApplySort(sort, reg.SortFields())`.
func (r *ColumnRegistry) SortFields() map[string]string {
	if r == nil {
		return nil
	}
	return r.sort
}

// ============================================================================
// COLUMN FILTER INPUT
// ============================================================================

// ColumnFilterInput is one column filter as the client sends it: the column
// name plus exactly one of the typed filters we already support. Bound to the
// GraphQL input of the same name.
type ColumnFilterInput struct {
	Field  string          `json:"field"`
	Text   *StringFilter   `json:"text,omitempty"`
	Number *FloatFilter    `json:"number,omitempty"`
	Date   *DateTimeFilter `json:"date,omitempty"`
	ID     *IDFilter       `json:"id,omitempty"`
	Bool   *BoolFilter     `json:"bool,omitempty"`
}

// kind reports which typed filter was sent. ok is false when more than one was.
func (f *ColumnFilterInput) kind() (kind ColumnType, sent int, ok bool) {
	if f.Text != nil {
		kind, sent = ColumnText, sent+1
	}
	if f.Number != nil {
		kind, sent = ColumnNumber, sent+1
	}
	if f.Date != nil {
		kind, sent = ColumnDate, sent+1
	}
	if f.ID != nil {
		kind, sent = ColumnID, sent+1
	}
	if f.Bool != nil {
		kind, sent = ColumnBool, sent+1
	}
	return kind, sent, sent == 1
}

// ApplyColumnFilters applies generic per-column filters against the list's
// registry. Entries combine with AND, with each other and with whatever the
// list's own typed filter already applied.
//
// An entry naming a column the list did not register, or carrying a filter of
// the wrong type for it, is refused as a bad request rather than silently
// ignored — a filter that does nothing looks to the user like a broken list.
// An entry with no filter at all is a no-op.
func (fb *FilterBuilder) ApplyColumnFilters(filters []*ColumnFilterInput, registry *ColumnRegistry) error {
	for _, f := range filters {
		if f == nil {
			continue
		}

		col, ok := registry.Lookup(f.Field)
		if !ok {
			return response.NewBadRequest(
				fmt.Sprintf("column filter: unknown field %q", f.Field),
				fmt.Sprintf("This list cannot be filtered by %q.", f.Field),
			)
		}

		kind, sent, single := f.kind()
		if sent == 0 {
			continue
		}
		if !single {
			return response.NewBadRequest(
				fmt.Sprintf("column filter %q: more than one filter type sent", f.Field),
				fmt.Sprintf("Send one filter at a time for %q.", f.Field),
			)
		}
		if kind != col.Type {
			return response.NewBadRequest(
				fmt.Sprintf("column filter %q: got %s filter, column is %s", f.Field, kind, col.Type),
				fmt.Sprintf("%q cannot be filtered as %s.", f.Field, kind),
			)
		}

		switch col.Type {
		case ColumnText:
			fb.String(col.SQL, f.Text)
		case ColumnNumber:
			fb.Float(col.SQL, f.Number)
		case ColumnDate:
			fb.DateTime(col.SQL, f.Date)
		case ColumnID:
			fb.ID(col.SQL, f.ID)
		case ColumnBool:
			fb.Bool(col.SQL, f.Bool)
		}
	}
	return nil
}
