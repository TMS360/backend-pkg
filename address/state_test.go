package address

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ptr is a tiny helper so table rows can express *string inputs inline.
func ptr(s string) *string { return &s }

// --- Acceptance Criteria ---------------------------------------------------

// AC1: getShipment / getShipments legs return state as 2 uppercase letters
// ("GA", not "Georgia" or "ga"). At the shared-normalizer level this is the
// guarantee that a full name or lower-cased code collapses to a 2-letter
// uppercase code.
func TestAC1_FullNameAndCasingBecomeTwoUppercaseLetters(t *testing.T) {
	cases := map[string]string{
		"Georgia": "GA",
		"georgia": "GA",
		"GA":      "GA",
		"ga":      "GA",
		"gA":      "GA",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			got := StateCode(in)
			assert.Equal(t, want, got)
			require.Len(t, got, 2)
			assert.Equal(t, want, got, "must be uppercase 2-letter")
		})
	}
}

// AC2: the same 2-letter shape is produced regardless of which entity's address
// is being normalized — the function is entity-agnostic, so one set of inputs
// standing in for customer/broker/driver/truck/trailer/invoice addresses all
// resolve identically.
func TestAC2_SameShapeAcrossAllEntities(t *testing.T) {
	// Values that might appear on any entity's stored address.
	cases := map[string]string{
		"California": "CA",
		"texas":      "TX",
		"NEW YORK":   "NY",
		"Illinois":   "IL",
		"fl":         "FL",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, StateCode(in))
		})
	}
}

// AC3: unknown or unmappable state strings return null rather than a
// half-converted value. StateCode returns "" and the read helper returns nil.
func TestAC3_UnknownReturnsNull(t *testing.T) {
	unknown := []string{"XX", "ZZ", "Freedonia", "Nuevo León", "12", "!!", "Californ"}
	for _, in := range unknown {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, "", StateCode(in), "StateCode should not half-convert")
			assert.Nil(t, Normalize(ptr(in)), "Normalize should surface null")
		})
	}
}

// AC4: existing rows persisted as full names still surface as 2-letter codes on
// read. The read helper converts a stored long-form value to its code.
func TestAC4_LegacyFullNameSurfacesAsCodeOnRead(t *testing.T) {
	legacy := map[string]string{
		"California": "CA",
		"New York":   "NY",
		"georgia":    "GA",
	}
	for stored, want := range legacy {
		t.Run(stored, func(t *testing.T) {
			got := Normalize(ptr(stored))
			require.NotNil(t, got)
			assert.Equal(t, want, *got)
		})
	}
}

// --- Edge Cases ------------------------------------------------------------

// Full name with non-standard casing or surrounding/internal whitespace.
func TestEdge_WhitespaceAndCasing(t *testing.T) {
	cases := map[string]string{
		"california ":  "CA",
		"  California": "CA",
		"NEW YORK":     "NY",
		"new    york":  "NY",
		"\tTexas\n":    "TX",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, StateCode(in))
		})
	}
}

// Already-correct 2-letter code in any casing returns uppercase.
func TestEdge_AlreadyTwoLetterAnyCasing(t *testing.T) {
	cases := map[string]string{
		"ca": "CA", "Ca": "CA", "CA": "CA", "cA": "CA",
		"ny": "NY", " ny ": "NY",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, StateCode(in))
		})
	}
}

// US territories are included in the map.
func TestEdge_USTerritories(t *testing.T) {
	// Full names map, and the codes themselves round-trip.
	names := map[string]string{
		"Puerto Rico":              "PR",
		"Guam":                     "GU",
		"American Samoa":           "AS",
		"Virgin Islands":           "VI",
		"U.S. Virgin Islands":      "VI",
		"Northern Mariana Islands": "MP",
	}
	for in, want := range names {
		t.Run("name/"+in, func(t *testing.T) {
			assert.Equal(t, want, StateCode(in))
		})
	}
	for _, code := range []string{"PR", "GU", "VI", "AS", "MP"} {
		t.Run("code/"+code, func(t *testing.T) {
			assert.Equal(t, code, StateCode(code))
			assert.True(t, IsValidCode(code))
		})
	}
}

// Canadian provinces / territories are handled the same way.
func TestEdge_CanadianProvinces(t *testing.T) {
	names := map[string]string{
		"Ontario":                   "ON",
		"Quebec":                    "QC",
		"Québec":                    "QC",
		"British Columbia":          "BC",
		"Alberta":                   "AB",
		"Newfoundland and Labrador": "NL",
	}
	for in, want := range names {
		t.Run("name/"+in, func(t *testing.T) {
			assert.Equal(t, want, StateCode(in))
		})
	}
	for _, code := range []string{"ON", "QC", "BC", "AB", "MB", "NB", "NL", "NS", "NT", "NU", "PE", "SK", "YT"} {
		t.Run("code/"+code, func(t *testing.T) {
			assert.Equal(t, code, StateCode(code))
			assert.True(t, IsValidCode(code))
		})
	}
}

// Junk / non-US-non-Canada / empty string return "" and never panic.
func TestEdge_JunkAndEmptyReturnEmptyWithoutPanic(t *testing.T) {
	junk := []string{"", "   ", "\t\n", "XX", "Zz", "1234", "Guangdong", "Bavaria", "Nuevo León", "c a"}
	for _, in := range junk {
		t.Run(in, func(t *testing.T) {
			assert.NotPanics(t, func() {
				assert.Equal(t, "", StateCode(in))
			})
		})
	}
}

// --- Helper semantics ------------------------------------------------------

// Normalize (read helper): nil -> nil, unmappable -> nil, mappable -> &code.
func TestNormalize_ReadHelperSemantics(t *testing.T) {
	assert.Nil(t, Normalize(nil), "nil stays nil")
	assert.Nil(t, Normalize(ptr("")), "empty -> nil")
	assert.Nil(t, Normalize(ptr("XX")), "junk -> nil")

	got := Normalize(ptr("california"))
	require.NotNil(t, got)
	assert.Equal(t, "CA", *got)
}

// Clean (write helper): nil -> nil, mappable -> &code, unmappable -> original
// preserved (no data loss).
func TestClean_WriteHelperPreservesUnmappable(t *testing.T) {
	assert.Nil(t, Clean(nil))

	mapped := Clean(ptr("California"))
	require.NotNil(t, mapped)
	assert.Equal(t, "CA", *mapped)

	// A legitimately typed but out-of-scope value is preserved, not nulled.
	preserved := Clean(ptr("Nuevo León"))
	require.NotNil(t, preserved)
	assert.Equal(t, "Nuevo León", *preserved)
}

// CleanString (string write helper): mappable -> code, unmappable -> original.
func TestCleanString_WriteHelperPreservesUnmappable(t *testing.T) {
	assert.Equal(t, "CA", CleanString("California"))
	assert.Equal(t, "CA", CleanString("ca"))
	assert.Equal(t, "ON", CleanString("Ontario"))
	assert.Equal(t, "Nuevo León", CleanString("Nuevo León")) // preserved
	assert.Equal(t, "", CleanString(""))                     // empty stays empty
}

// All 50 states + DC are present and each maps to a distinct valid code.
func TestMapIntegrity_FiftyStatesPlusDC(t *testing.T) {
	fiftyPlusDC := []string{
		"Alabama", "Alaska", "Arizona", "Arkansas", "California", "Colorado",
		"Connecticut", "Delaware", "Florida", "Georgia", "Hawaii", "Idaho",
		"Illinois", "Indiana", "Iowa", "Kansas", "Kentucky", "Louisiana",
		"Maine", "Maryland", "Massachusetts", "Michigan", "Minnesota",
		"Mississippi", "Missouri", "Montana", "Nebraska", "Nevada",
		"New Hampshire", "New Jersey", "New Mexico", "New York",
		"North Carolina", "North Dakota", "Ohio", "Oklahoma", "Oregon",
		"Pennsylvania", "Rhode Island", "South Carolina", "South Dakota",
		"Tennessee", "Texas", "Utah", "Vermont", "Virginia", "Washington",
		"West Virginia", "Wisconsin", "Wyoming", "District of Columbia",
	}
	require.Len(t, fiftyPlusDC, 51)
	seen := map[string]bool{}
	for _, name := range fiftyPlusDC {
		code := StateCode(name)
		require.Lenf(t, code, 2, "%s should map to a 2-letter code", name)
		assert.Falsef(t, seen[code], "duplicate code %s for %s", code, name)
		seen[code] = true
	}
}
