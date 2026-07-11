// Package address holds shared helpers for normalizing postal-address fields
// that every TMS service returns and persists.
//
// The state normalizer maps a US state / DC / territory or a Canadian province
// to its canonical 2-letter code (USPS for the US — the same set as FAA
// Appendix A — and Canada Post for Canada). It is deliberately conservative:
// anything it does not recognize (junk, Mexican/other states, empty) maps to
// the empty string so callers can surface it as null rather than a
// half-converted value.
package address

import "strings"

// stateNameToCode maps a normalized (lower-cased, whitespace-collapsed) full
// state/territory/province name to its canonical 2-letter code.
//
// US: 50 states + DC + territories (PR, GU, VI, AS, MP) + the freely-associated
// states and minor outlying islands that share the USPS abbreviation list
// (FM, MH, PW, UM). Canada: 10 provinces + 3 territories (Canada Post codes).
var stateNameToCode = map[string]string{
	// --- US states ---
	"alabama": "AL", "alaska": "AK", "arizona": "AZ", "arkansas": "AR",
	"california": "CA", "colorado": "CO", "connecticut": "CT", "delaware": "DE",
	"florida": "FL", "georgia": "GA", "hawaii": "HI", "idaho": "ID",
	"illinois": "IL", "indiana": "IN", "iowa": "IA", "kansas": "KS",
	"kentucky": "KY", "louisiana": "LA", "maine": "ME", "maryland": "MD",
	"massachusetts": "MA", "michigan": "MI", "minnesota": "MN", "mississippi": "MS",
	"missouri": "MO", "montana": "MT", "nebraska": "NE", "nevada": "NV",
	"new hampshire": "NH", "new jersey": "NJ", "new mexico": "NM", "new york": "NY",
	"north carolina": "NC", "north dakota": "ND", "ohio": "OH", "oklahoma": "OK",
	"oregon": "OR", "pennsylvania": "PA", "rhode island": "RI", "south carolina": "SC",
	"south dakota": "SD", "tennessee": "TN", "texas": "TX", "utah": "UT",
	"vermont": "VT", "virginia": "VA", "washington": "WA", "west virginia": "WV",
	"wisconsin": "WI", "wyoming": "WY",

	// --- US federal district + territories ---
	"district of columbia":                 "DC",
	"puerto rico":                          "PR",
	"guam":                                 "GU",
	"american samoa":                       "AS",
	"virgin islands":                       "VI",
	"u.s. virgin islands":                  "VI",
	"us virgin islands":                    "VI",
	"northern mariana islands":             "MP",
	"federated states of micronesia":       "FM",
	"micronesia":                           "FM",
	"marshall islands":                     "MH",
	"palau":                                "PW",
	"united states minor outlying islands": "UM",

	// --- Canadian provinces + territories ---
	"alberta": "AB", "british columbia": "BC", "manitoba": "MB",
	"new brunswick": "NB", "newfoundland and labrador": "NL", "newfoundland": "NL",
	"nova scotia": "NS", "northwest territories": "NT", "nunavut": "NU",
	"ontario": "ON", "prince edward island": "PE", "quebec": "QC", "québec": "QC",
	"saskatchewan": "SK", "yukon": "YT",
}

// validCodes is the set of accepted 2-letter codes, derived from the values of
// stateNameToCode so the two never drift apart.
var validCodes = func() map[string]struct{} {
	set := make(map[string]struct{}, len(stateNameToCode))
	for _, code := range stateNameToCode {
		set[code] = struct{}{}
	}
	return set
}()

// StateCode normalizes a US/Canadian state or province to its canonical 2-letter
// code:
//
//   - a full name in any casing / whitespace ("california ", "NEW YORK") is mapped;
//   - a valid 2-letter code in any casing ("ca", "Ca") is upper-cased;
//   - anything unrecognized (junk like "XX", a non-US/non-Canada state, or an
//     empty string) returns "" — never a partially-converted value, never a panic.
func StateCode(state string) string {
	// strings.Fields trims and collapses every run of whitespace, so
	// "  new   york " becomes "new york" before lookup.
	s := strings.Join(strings.Fields(state), " ")
	if s == "" {
		return ""
	}
	if len(s) == 2 {
		up := strings.ToUpper(s)
		if _, ok := validCodes[up]; ok {
			return up
		}
		return ""
	}
	if code, ok := stateNameToCode[strings.ToLower(s)]; ok {
		return code
	}
	return ""
}

// IsValidCode reports whether code (case-insensitive) is a recognized 2-letter
// US or Canadian state/province code.
func IsValidCode(code string) bool {
	_, ok := validCodes[strings.ToUpper(strings.TrimSpace(code))]
	return ok
}

// Normalize is the read/output-layer helper. It returns nil when the input is
// nil, empty, or unmappable, and otherwise a pointer to the canonical 2-letter
// code. Wire it into the address mapping the resolvers share so an unmappable
// stored value surfaces to clients as null instead of a half-converted string.
func Normalize(state *string) *string {
	if state == nil {
		return nil
	}
	if code := StateCode(*state); code != "" {
		return &code
	}
	return nil
}

// Clean is the write-path helper. It normalizes a value to its 2-letter code
// when mappable and PRESERVES the original otherwise, so a legitimately typed
// but out-of-scope value (e.g. a Mexican state) is never destroyed on write —
// the read layer (Normalize) is what surfaces unmappable values as null. nil
// stays nil.
func Clean(state *string) *string {
	if state == nil {
		return nil
	}
	if code := StateCode(*state); code != "" {
		return &code
	}
	return state
}

// CleanString is the string-valued sibling of Clean, for models whose state
// field is a plain string (e.g. FMCSA results, gRPC message fields): it returns
// the 2-letter code when mappable and the original value otherwise.
func CleanString(state string) string {
	if code := StateCode(state); code != "" {
		return code
	}
	return state
}
