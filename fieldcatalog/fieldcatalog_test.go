package fieldcatalog_test

import (
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/fieldcatalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC6: each value type is a declared, valid ValueType; an unknown one is not.
func TestValueType_IsValid_CoversAllConstants(t *testing.T) {
	for _, v := range []fieldcatalog.ValueType{
		fieldcatalog.ValueString, fieldcatalog.ValueNumber, fieldcatalog.ValueDate,
		fieldcatalog.ValueBool, fieldcatalog.ValueID, fieldcatalog.ValueStatus,
	} {
		assert.Truef(t, v.IsValid(), "%s should be a valid value type", v)
	}
	assert.False(t, fieldcatalog.ValueType("MYSTERY").IsValid())
	assert.False(t, fieldcatalog.ValueType("").IsValid())
}

// AC6 / structural: every catalogued entry is well-formed — non-empty record
// type / path / label, a valid value type, a record-prefixed unique path, and
// (since every first-version field is gated) a permission. Derived from the
// catalog itself so it cannot drift as fields are added.
func TestCatalog_EntriesWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range fieldcatalog.Entries() {
		assert.NotEmpty(t, e.RecordType, "record type")
		assert.NotEmpty(t, e.Path, "path for %s", e.RecordType)
		assert.NotEmpty(t, e.Label, "label for %s", e.Path)
		assert.Truef(t, e.ValueType.IsValid(), "value type %q for %s", e.ValueType, e.Path)
		assert.NotEmptyf(t, e.Permission, "every first-version field is gated: %s", e.Path)
		assert.Falsef(t, seen[e.Path], "duplicate path %s", e.Path)
		seen[e.Path] = true
		// Path is record-prefixed: "truck.number" for TRUCK, "driverCrew.status"
		// for DRIVER_CREW.
		prefix := strings.ReplaceAll(strings.ToLower(e.RecordType), "_", "")
		assert.Truef(t, strings.HasPrefix(strings.ToLower(e.Path), prefix),
			"path %s should be prefixed by its record type %s", e.Path, e.RecordType)
	}
}

// AC6: a valid path resolves; AC1 support: Get returns the entry for the right
// record type.
func TestGet_ValidPath(t *testing.T) {
	e, ok := fieldcatalog.Get("TRUCK", "truck.number")
	require.True(t, ok)
	assert.Equal(t, "Truck #", e.Label)
	assert.Equal(t, fieldcatalog.ValueString, e.ValueType)
	assert.True(t, e.Sortable)
	assert.True(t, e.Bindable())
}

// AC6 + AC1: an unknown path is not in the catalog (so column creation can
// refuse it).
func TestGet_InvalidPath(t *testing.T) {
	_, ok := fieldcatalog.Get("TRUCK", "truck.speedometerReading")
	assert.False(t, ok)
	_, ok = fieldcatalog.Lookup("truck.speedometerReading")
	assert.False(t, ok)
	assert.False(t, fieldcatalog.IsBindablePath("truck.speedometerReading"))
}

// AC1: a real path bound under the WRONG record type is refused — Get keys on
// both, so the caller's rejection can name the record type and the path.
func TestGet_PathUnderWrongRecordType(t *testing.T) {
	_, ok := fieldcatalog.Get("TRIP", "truck.number") // real path, wrong record type
	assert.False(t, ok)
}

// AC3: a gated field carries the permission its owning service enforces, so the
// masking layer has something to enforce.
func TestGatedFieldCarriesPermission(t *testing.T) {
	cases := map[string]string{
		"truck.number":                  "fleet.trucks.view",
		"trailer.number":                "fleet.trailers.view",
		"trip.grossRate":                "shipments.trips.view",
		"driverCrew.status":             "teams.crews.view",
		"driverCrew.primaryDriver.name": "teams.crews.view",
	}
	for path, wantPerm := range cases {
		e, ok := fieldcatalog.Lookup(path)
		require.Truef(t, ok, "%s should be catalogued", path)
		assert.Equalf(t, wantPerm, e.Permission, "permission for %s", path)
	}
}

// AC3 (integrity): every permission referenced by the field catalog is a REAL
// permission-catalog code — the field catalog can never gate on a permission
// that does not exist, so the two shared catalogs cannot drift.
func TestPermissionsExistInPermissionCatalog(t *testing.T) {
	for _, e := range fieldcatalog.Entries() {
		assert.Truef(t, enums.IsValidPermissionCode(e.Permission),
			"field %s gated by unknown permission %q", e.Path, e.Permission)
	}
}

// AC6: the catalog concretely exercises each value type.
func TestCatalog_CoversEachValueType(t *testing.T) {
	present := map[fieldcatalog.ValueType]bool{}
	for _, e := range fieldcatalog.Entries() {
		present[e.ValueType] = true
	}
	for _, v := range []fieldcatalog.ValueType{
		fieldcatalog.ValueString, fieldcatalog.ValueNumber, fieldcatalog.ValueDate,
		fieldcatalog.ValueBool, fieldcatalog.ValueID, fieldcatalog.ValueStatus,
	} {
		assert.Truef(t, present[v], "first-version catalog should include a %s field", v)
	}
}

// Edge case: a verified-but-not-a-proven-hop field (tms-teams crew) is
// catalogued but Held — never offered as a bindable board column, so a column
// that looks valid never fails at fetch.
func TestHeldFieldsAreNotBindable(t *testing.T) {
	for _, e := range fieldcatalog.ByRecordType("DRIVER_CREW") {
		assert.Truef(t, e.Held, "%s should be held", e.Path)
		assert.Falsef(t, e.Bindable(), "%s must not be bindable", e.Path)
		assert.Falsef(t, fieldcatalog.IsBindablePath(e.Path), "%s must not be a bindable path", e.Path)
	}
	// The proven-hop record types are bindable.
	for _, rt := range []string{"TRUCK", "TRAILER", "TRIP"} {
		for _, e := range fieldcatalog.ByRecordType(rt) {
			assert.Truef(t, e.Bindable(), "%s should be bindable", e.Path)
			assert.Truef(t, fieldcatalog.IsBindablePath(e.Path), "%s should be a bindable path", e.Path)
		}
	}
}

// AC2: the verified first-version fields are present (spot-check that the
// verification actually landed the intended fields).
func TestVerifiedFirstVersionPresent(t *testing.T) {
	want := []string{
		"truck.number", "truck.primaryDriverName",
		"trailer.number", "trailer.primaryDriverName",
		"trip.tripNumber", "trip.status", "trip.loadedMiles", "trip.emptyMiles", "trip.grossRate",
	}
	for _, p := range want {
		_, ok := fieldcatalog.Lookup(p)
		assert.Truef(t, ok, "verified field %s must be catalogued", p)
	}
}

func TestRecordTypes(t *testing.T) {
	rts := fieldcatalog.RecordTypes()
	assert.Contains(t, rts, "TRUCK")
	assert.Contains(t, rts, "TRAILER")
	assert.Contains(t, rts, "TRIP")
	assert.Contains(t, rts, "DRIVER_CREW")
}
