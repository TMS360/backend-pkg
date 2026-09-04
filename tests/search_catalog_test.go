package tests

import (
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/search"
	"github.com/TMS360/backend-pkg/searchcatalog"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-2044. The catalog is the contract three sides read: the client (entity
// codes and labels), the aggregator (owning service and permission) and each
// provider (its own field and relation paths). These tests pin the invariants
// that keep the three in agreement.

func TestCatalog_EntityInvariants(t *testing.T) {
	seenCode := map[string]bool{}

	for _, e := range searchcatalog.Catalog {
		t.Run(e.Code, func(t *testing.T) {
			assert.NotEmpty(t, e.Code, "entity code must be set")
			assert.NotEmpty(t, e.Label, "entity label is shown as the group header")
			assert.NotEmpty(t, e.Service, "every entity must name its owning service")
			assert.NotEmpty(t, e.Permission, "every group is gated by a view permission")

			assert.False(t, seenCode[e.Code], "duplicate entity code %s", e.Code)
			seenCode[e.Code] = true

			// A group must be findable by something.
			assert.NotEmpty(t, e.Paths(),
				"entity %s has neither fields nor relations", e.Code)

			// Codes are client-facing GraphQL enum values: upper snake case.
			assert.Equal(t, strings.ToUpper(e.Code), e.Code, "entity code must be upper case")
		})
	}
}

func TestCatalog_PathInvariants(t *testing.T) {
	seenPath := map[string]string{}

	for _, e := range searchcatalog.Catalog {
		for _, f := range e.Fields {
			assert.NotEmpty(t, f.Label, "%s: field %s needs a label for the match chip", e.Code, f.Path)
			assert.NotEmpty(t, f.Kind, "%s: field %s needs a kind for the query classifier", e.Code, f.Path)
			assert.Contains(t, f.Path, ".", "%s: field path %q must be entity-rooted", e.Code, f.Path)

			if owner, dup := seenPath[f.Path]; dup {
				t.Errorf("path %q declared twice (%s and %s)", f.Path, owner, e.Code)
			}
			seenPath[f.Path] = e.Code
		}

		for _, r := range e.Relations {
			assert.NotEmpty(t, r.Label, "%s: relation %s needs a label", e.Code, r.Path)
			assert.NotEmpty(t, r.Kind, "%s: relation %s needs a kind", e.Code, r.Path)
			assert.Contains(t, r.Path, ".", "%s: relation path %q must be entity-rooted", e.Code, r.Path)

			if owner, dup := seenPath[r.Path]; dup {
				t.Errorf("path %q declared twice (%s and %s)", r.Path, owner, e.Code)
			}
			seenPath[r.Path] = e.Code

			// A relation that names a target must name a real entity, so the
			// client can route a chip click to that record type.
			if r.Target != "" {
				_, ok := searchcatalog.ByCode(r.Target)
				assert.True(t, ok, "%s: relation %s targets unknown entity %q", e.Code, r.Path, r.Target)
			}
		}
	}
}

// Every gate must be a real permission code from the shared catalog: a typo
// here would silently hide a whole group from everyone.
func TestCatalog_PermissionsExist(t *testing.T) {
	require.NotEmpty(t, enums.PermissionCatalog, "permission catalog is empty")

	for _, e := range searchcatalog.Catalog {
		assert.True(t, enums.IsValidPermissionCode(e.Permission),
			"%s: permission %q is not in enums.PermissionCatalog", e.Code, e.Permission)
	}
}

// Money never reaches a search hit (epic DEV-1957: "no pay, rate, or balance
// in the row"). The cheapest durable guard is to refuse money-ish paths in the
// catalog itself.
func TestCatalog_NoMoneyFields(t *testing.T) {
	banned := []string{"pay", "rate", "balance", "total", "amount", "gross", "cost", "price"}

	for _, e := range searchcatalog.Catalog {
		for _, path := range e.Paths() {
			lower := strings.ToLower(path)
			for _, word := range banned {
				// payStatement.number is the statement's identifier, not money.
				if strings.HasPrefix(lower, "paystatement.") {
					continue
				}
				assert.NotContains(t, lower, word,
					"%s: path %q looks like money; search hits must not carry it", e.Code, path)
			}
		}
	}
}

func TestCatalog_Lookups(t *testing.T) {
	e, ok := searchcatalog.ByCode(searchcatalog.EntityLoad)
	require.True(t, ok)
	assert.Equal(t, searchcatalog.ServiceLoads, e.Service)
	assert.Equal(t, "shipments.shipments.view", e.Permission)

	label, ok := e.LabelForPath("load.trip.truck.number")
	require.True(t, ok)
	assert.Equal(t, "Truck #", label)

	_, ok = e.LabelForPath("load.nope")
	assert.False(t, ok)

	_, ok = searchcatalog.ByCode("NOPE")
	assert.False(t, ok)

	loads := searchcatalog.ForService(searchcatalog.ServiceLoads)
	assert.ElementsMatch(t,
		[]string{
			searchcatalog.EntityLoad, searchcatalog.EntityTrip,
			searchcatalog.EntityTruck, searchcatalog.EntityTrailer,
		},
		codesOf(loads),
		"tms-loads owns the shipment and fleet entities")

	assert.Contains(t, searchcatalog.Services(), searchcatalog.ServiceMediator)
	assert.Len(t, searchcatalog.Codes(), len(searchcatalog.Catalog))
}

func codesOf(in []searchcatalog.Entity) []string {
	out := make([]string, 0, len(in))
	for _, e := range in {
		out = append(out, e.Code)
	}
	return out
}

// ---------------------------------------------------------------------------
// Query normalization and classification
// ---------------------------------------------------------------------------

func TestQuery_Normalize(t *testing.T) {
	assert.Equal(t, "", search.Normalize("   "))
	assert.Equal(t, "marcus hale", search.Normalize("  marcus   hale  "))

	long := strings.Repeat("a", search.MaxQueryLen+40)
	assert.Len(t, search.Normalize(long), search.MaxQueryLen, "long paste is truncated, not rejected")

	// Truncation must not split a multi-byte rune.
	cyrillic := strings.Repeat("щ", search.MaxQueryLen+10)
	got := search.Normalize(cyrillic)
	assert.Equal(t, search.MaxQueryLen, len([]rune(got)))
	assert.True(t, isValidUTF8(got))
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestQuery_ValidLength(t *testing.T) {
	assert.False(t, search.Parse("").Valid(), "empty query never reaches the database")
	assert.False(t, search.Parse("  ").Valid(), "whitespace only never reaches the database")
	assert.False(t, search.Parse("a").Valid(), "one character never reaches the database")
	assert.True(t, search.Parse("ab").Valid())
	assert.True(t, search.Parse("67282").Valid())
}

func TestQuery_UUIDIsDirectLookup(t *testing.T) {
	id := uuid.New()
	q := search.Parse("  " + id.String() + "  ")
	require.NotNil(t, q.ID)
	assert.Equal(t, id, *q.ID)
	assert.True(t, q.Valid())

	// A pasted id is answered by primary key, so no text column is worth
	// scanning.
	for _, kind := range []searchcatalog.FieldKind{
		searchcatalog.KindText, searchcatalog.KindNumber, searchcatalog.KindCode,
		searchcatalog.KindEmail, searchcatalog.KindPhone, searchcatalog.KindStatus,
	} {
		assert.False(t, q.Wants(kind), "kind %s must not be scanned for a uuid query", kind)
	}
}

func TestQuery_WantsByKind(t *testing.T) {
	digits := search.Parse("67282")
	assert.True(t, digits.DigitsOnly)
	assert.True(t, digits.Wants(searchcatalog.KindNumber))
	assert.True(t, digits.Wants(searchcatalog.KindCode))
	assert.True(t, digits.Wants(searchcatalog.KindPhone))
	assert.False(t, digits.Wants(searchcatalog.KindText), "digits cannot match a name")
	assert.False(t, digits.Wants(searchcatalog.KindStatus))
	assert.False(t, digits.Wants(searchcatalog.KindEmail))

	phone := search.Parse("(312) 555-0134")
	assert.True(t, phone.DigitsOnly, "phone punctuation is not text")
	assert.True(t, phone.Wants(searchcatalog.KindPhone))
	assert.False(t, phone.Wants(searchcatalog.KindText))

	email := search.Parse("marcus@acme.com")
	assert.True(t, email.HasAt)
	assert.True(t, email.Wants(searchcatalog.KindEmail))
	assert.False(t, email.Wants(searchcatalog.KindText))
	assert.False(t, email.Wants(searchcatalog.KindNumber))

	text := search.Parse("marcus hale")
	assert.False(t, text.DigitsOnly)
	assert.True(t, text.Wants(searchcatalog.KindText))
	assert.True(t, text.Wants(searchcatalog.KindStatus))
	assert.True(t, text.Wants(searchcatalog.KindCode))
	assert.True(t, text.Wants(searchcatalog.KindNumber),
		"load_id / reference_numbers are varchar and carry letters")
	assert.False(t, text.Wants(searchcatalog.KindEmail))

	// A VIN tail mixes letters and digits and must reach VIN/plate columns.
	vin := search.Parse("1FUJG")
	assert.True(t, vin.Wants(searchcatalog.KindCode))
}

func TestQuery_Limits(t *testing.T) {
	assert.Equal(t, search.DefaultLimitPerEntity, search.LimitPerEntity(0))
	assert.Equal(t, search.DefaultLimitPerEntity, search.LimitPerEntity(-3))
	assert.Equal(t, 7, search.LimitPerEntity(7))
	assert.Equal(t, search.MaxLimitPerEntity, search.LimitPerEntity(1000))

	total, more := search.CapTotal(12)
	assert.Equal(t, 12, total)
	assert.False(t, more)

	total, more = search.CapTotal(5000)
	assert.Equal(t, search.MaxGroupTotal, total)
	assert.True(t, more)
}

// ---------------------------------------------------------------------------
// Scoring
// ---------------------------------------------------------------------------

func TestScore_Bands(t *testing.T) {
	q := search.Parse("1043")

	assert.Equal(t, search.ScoreExact, search.ScoreValue(q, "1043", false))
	assert.Equal(t, search.ScorePrefix, search.ScoreValue(q, "10435", false))
	assert.Equal(t, search.ScoreSubstring, search.ScoreValue(q, "T-1043-A", false))
	assert.Equal(t, search.ScoreFuzzy, search.ScoreValue(q, "1053", false),
		"a trigram-only match scores at the fuzzy band")

	// Case is irrelevant.
	assert.Equal(t, search.ScoreExact, search.ScoreValue(search.Parse("marcus"), "Marcus", false))
}

func TestScore_OwnFieldBeatsRelation(t *testing.T) {
	q := search.Parse("1043")

	own := search.ScoreValue(q, "1043", false)
	rel := search.ScoreValue(q, "1043", true)
	assert.Greater(t, own, rel,
		"the truck itself must outrank the loads found through that truck")
	assert.Equal(t, own*search.RelationFactor, rel)
}

func TestMatcher_BuildsChips(t *testing.T) {
	q := search.Parse("1043")
	m, ok := search.NewMatcher(searchcatalog.EntityLoad, q)
	require.True(t, ok)

	relation := m.Add("load.trip.truck.number", "1043")
	require.NotNil(t, relation)
	assert.Equal(t, "Truck #", relation.Label)

	own := m.Add("load.shipmentNumber", "1043")
	require.NotNil(t, own)
	assert.Equal(t, "Load #", own.Label)
	assert.Greater(t, own.Score, relation.Score)

	assert.Nil(t, m.Add("load.shipmentNumber", ""), "empty value is not a match")
	assert.Nil(t, m.Add("load.notInCatalog", "x"), "unknown path is dropped, not guessed")

	matches := []*search.Match{relation, nil, own}
	assert.Equal(t, own.Score, search.Best(matches), "hit score is the strongest match")

	proto := search.ToProto(matches)
	require.Len(t, proto, 2, "nil matches are dropped")
	assert.Equal(t, "load.shipmentNumber", proto[0].Field, "strongest match comes first")
	assert.Equal(t, "load.trip.truck.number", proto[1].Field)

	_, ok = search.NewMatcher("NOPE", q)
	assert.False(t, ok)
}
