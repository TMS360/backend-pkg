// Package searchcatalog is the single source of truth for the office global
// search contract (DEV-2044): which entities are searchable, who owns each
// one, which permission gates it, and which fields and relations a hit can be
// matched on.
//
// The catalog is deliberately declarative about the CONTRACT only — paths,
// labels and permissions, i.e. everything the client and the aggregator need.
// It carries no SQL: the owning service maps each Path to its own column or
// EXISTS sub-query in a local registry, and a test there asserts the local
// registry covers every path this catalog declares for its entities. That
// keeps a column rename inside the service that owns the column while the
// client-visible contract stays here, shared and testable.
//
// Adding an entity or a field is a reviewed code change with a test. There is
// no runtime path that adds a searchable field.
package searchcatalog

// Service names of the search providers. The aggregator compares these against
// its own name to decide what it can answer in-process and what needs a
// SearchService gRPC hop.
const (
	ServiceLoads      = "tms-loads"
	ServiceAuth       = "tms-auth"
	ServiceMediator   = "tms-mediator"
	ServiceTeams      = "tms-teams"
	ServiceTasks      = "backend-tasks"
	ServiceFiles      = "tms-files"
	ServiceAccounting = "backend-accounting"
)

// Resolver entity names for ResolverService.ResolveIDs — the vocabulary the
// aggregator uses when a relation lives in another database and only matching
// ids are needed (Relation.Remote). They are wire strings shared by caller and
// provider, so they live here next to the entities they resolve.
//
// A people resolve MUST carry a company_id filter: resolving a common name
// across every tenant and then capping the id list could drop the tenant's own
// match (search.MaxResolvedIDs), so the providers refuse an unscoped call.
const (
	ResolverEntityDrivers     = "drivers"
	ResolverEntityOfficeUsers = "office_users"
	ResolverEntityCustomers   = "customers"
	// ResolverEntityCompanies predates this ticket (tms-auth) and is listed
	// for completeness: it is the tenant lookup behind the shipment list's
	// cross-service company filter.
	ResolverEntityCompanies = "companies"
)

// Entity codes. These are the GraphQL enum values the client sees, so they are
// stable identifiers, not labels.
const (
	EntityLoad         = "LOAD"
	EntityTrip         = "TRIP"
	EntityTruck        = "TRUCK"
	EntityTrailer      = "TRAILER"
	EntityDriver       = "DRIVER"
	EntityOfficeUser   = "OFFICE_USER"
	EntityCustomer     = "CUSTOMER"
	EntityCrew         = "CREW"
	EntityTask         = "TASK"
	EntityFile         = "FILE"
	EntityInvoice      = "INVOICE"
	EntityPayStatement = "PAY_STATEMENT"
)

// FieldKind tells the query classifier whether a field is worth searching for
// a given piece of user text. A digits-only query must not drag every name and
// address column through a trigram scan, and a text query must not be matched
// against a VIN.
type FieldKind string

const (
	// KindText is free text: a name, a facility, a note, a make.
	KindText FieldKind = "TEXT"
	// KindNumber is a human-facing number that may be typed partially:
	// load #, trip #, truck #, MC, USDOT, statement #.
	KindNumber FieldKind = "NUMBER"
	// KindCode is an alphanumeric identifier where a partial tail is the usual
	// query: VIN, licence plate, licence number.
	KindCode FieldKind = "CODE"
	// KindEmail and KindPhone are matched only when the query looks like one.
	KindEmail FieldKind = "EMAIL"
	KindPhone FieldKind = "PHONE"
	// KindStatus is an enum value (a status name typed in full or in part).
	KindStatus FieldKind = "STATUS"
)

// Field is one searchable field of the entity itself.
type Field struct {
	// Path is what the client sees in SearchMatch.field, e.g.
	// "shipment.referenceNumbers". It is also the key the owning service's
	// local registry maps to a column.
	Path string
	// Label is the short human name shown next to the match, e.g. "Reference #".
	Label string
	Kind  FieldKind
}

// Relation is a searchable field that lives on a related record rather than on
// the entity itself: a load found by its driver's name, a truck found by the
// number of the load it is under.
type Relation struct {
	// Path is the dotted route from the entity to the matched field, e.g.
	// "trip.truck.number". Client-visible, and the local registry key.
	Path  string
	Label string
	Kind  FieldKind
	// Target is the entity code the matched value belongs to, e.g. "TRUCK".
	// Empty when the related record is not itself a searchable entity
	// (shipment legs, roles, document types).
	Target string
	// Remote marks a relation whose value lives in ANOTHER service's database,
	// so the owning service must resolve matching ids over gRPC
	// (ResolverService.ResolveIDs) before it can filter locally.
	Remote bool
}

// Entity is one searchable record type.
type Entity struct {
	Code  string
	Label string
	// Service is the name of the service that owns the rows (see the Service*
	// constants).
	Service string
	// Permission is the view permission that gates the whole group. An actor
	// without it gets no group at all — never a 403 on the whole search.
	Permission string
	Fields     []Field
	Relations  []Relation
}

// ByCode returns the catalog entry for an entity code.
func ByCode(code string) (Entity, bool) {
	for _, e := range Catalog {
		if e.Code == code {
			return e, true
		}
	}
	return Entity{}, false
}

// ForService returns every entity owned by one service, in catalog order.
func ForService(service string) []Entity {
	var out []Entity
	for _, e := range Catalog {
		if e.Service == service {
			out = append(out, e)
		}
	}
	return out
}

// Codes returns every entity code in catalog order.
func Codes() []string {
	out := make([]string, 0, len(Catalog))
	for _, e := range Catalog {
		out = append(out, e.Code)
	}
	return out
}

// Services returns the distinct owning services in catalog order.
func Services() []string {
	seen := make(map[string]struct{}, len(Catalog))
	out := make([]string, 0, 8)
	for _, e := range Catalog {
		if _, ok := seen[e.Service]; ok {
			continue
		}
		seen[e.Service] = struct{}{}
		out = append(out, e.Service)
	}
	return out
}

// Paths returns every field and relation path of an entity — the set the
// owning service's local registry must cover.
func (e Entity) Paths() []string {
	out := make([]string, 0, len(e.Fields)+len(e.Relations))
	for _, f := range e.Fields {
		out = append(out, f.Path)
	}
	for _, r := range e.Relations {
		out = append(out, r.Path)
	}
	return out
}

// LabelForPath returns the label declared for a field or relation path.
func (e Entity) LabelForPath(path string) (string, bool) {
	for _, f := range e.Fields {
		if f.Path == path {
			return f.Label, true
		}
	}
	for _, r := range e.Relations {
		if r.Path == path {
			return r.Label, true
		}
	}
	return "", false
}
