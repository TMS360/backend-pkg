package toll

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// excelEpoch is the origin of the 1900 date system. Excel numbers 1900-01-01
// as serial 1 but also believes 1900 was a leap year, so serials at or below
// 60 are off by one; anchoring at 1899-12-30 makes every serial above that
// correct. Toll files are modern, so parseSerialTime simply refuses the
// ambiguous range rather than modelling the bug.
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// maxExcelSerial is 9999-12-31 — an upper sanity bound so a stray large number
// (an amount, a device id) is not silently read as a date.
const maxExcelSerial = 2958465

// textDateLayouts are the shapes seen when an aggregator writes a date as text
// instead of as a number. PrePass mixes both inside one file: Post Date
// arrives as a serial while Invoice Date arrives as "2024-08-31".
var textDateLayouts = []string{
	"2006-01-02",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05Z07:00",
	"01/02/2006",
	"1/2/2006",
	"01/02/2006 15:04",
	"1/2/2006 15:04",
	"01/02/2006 15:04:05",
	"1/2/2006 15:04:05",
	"01/02/2006 3:04:05 PM",
	"1/2/2006 3:04:05 PM",
	// Two-digit years, as the daily SFTP csv writes them: "8/20/26",
	// "08/19/26". LAST on purpose — time.Parse must consume the whole string,
	// so "8/20/2026" cannot reach these, but keeping the four-digit layouts
	// ahead of them removes the question entirely.
	"1/2/06",
	"01/02/06",
}

// ParseFileTime reads a timestamp in whichever form the file used — an Excel
// serial number or one of several text layouts — and returns it as a
// wall-clock time stamped UTC.
//
// The result is NOT an instant: source files carry no time zone, so the caller
// must re-interpret it in the company's zone before comparing against trips.
// See the TIME ZONES note on Row.
func ParseFileTime(s string) (time.Time, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return time.Time{}, false
	}
	if ts, ok := parseSerialTime(t); ok {
		return ts, true
	}
	for _, layout := range textDateLayouts {
		if ts, err := time.ParseInLocation(layout, t, time.UTC); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

// parseSerialTime converts an Excel serial ("45535.724837962996") to a time,
// rounding to whole seconds. The float carries more digits than the underlying
// value has meaning: 45535.724837962996 is 17:23:46 with noise in the tail,
// and rounding keeps a timestamp from rendering as :45.9999.
func parseSerialTime(s string) (time.Time, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return time.Time{}, false
	}
	if f < 61 || f > maxExcelSerial {
		return time.Time{}, false
	}
	secs := math.Round(f * 24 * 60 * 60)
	return excelEpoch.Add(time.Duration(secs) * time.Second), true
}

// ParseMoney reads a currency cell into an exact decimal rounded to cents.
//
// Never use a float here: spreadsheets hand back values like
// "0.56000000000000005" for what is $0.56, and accumulating those across a
// few thousand rows drifts the week total. Rounding to two places is safe
// because tolls are billed in whole cents.
//
// An empty cell is $0.00, not an error — agencies do post zero-dollar reads,
// and those rows must be stored (they simply are not selected by default).
func ParseMoney(s string) (decimal.Decimal, error) {
	t := strings.TrimSpace(s)
	t = strings.NewReplacer("$", "", ",", "", " ", "", " ", "").Replace(t)
	if t == "" {
		return decimal.Zero, nil
	}
	neg := false
	// Accounting-style negatives: (12.34) means -12.34.
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		neg = true
		t = t[1 : len(t)-1]
	}
	d, err := decimal.NewFromString(t)
	if err != nil {
		return decimal.Zero, fmt.Errorf("toll: not a money value %q: %w", s, err)
	}
	if neg {
		d = d.Neg()
	}
	return d.Round(2), nil
}

// NormalizePlate strips a plate down to the form used for matching: upper
// case, no punctuation, and with a leading state code removed.
//
// Toll files print plates as "IL-P1264873" while the fleet stores "P1264873",
// so a literal comparison never matches. The state code is only stripped when
// two letters are followed by a separator — "ABC123" keeps its letters,
// because there is no way to tell a state prefix from the plate itself once
// the separator is gone.
func NormalizePlate(s string) string {
	up := strings.ToUpper(strings.TrimSpace(s))
	if up == "" {
		return ""
	}
	if len(up) > 3 && isAlpha(up[0]) && isAlpha(up[1]) && (up[2] == '-' || up[2] == ' ' || up[2] == '/') {
		up = up[3:]
	}
	var b strings.Builder
	b.Grow(len(up))
	for i := 0; i < len(up); i++ {
		if isAlpha(up[i]) || isDigit(up[i]) {
			b.WriteByte(up[i])
		}
	}
	return b.String()
}

// NormalizeTruckRef cleans the carrier's own truck number as printed in the
// file. Leading zeros are dropped ("008" and "8" are the same unit) but only
// for all-digit values, so an alphanumeric unit like "0A1" survives intact.
//
// Returns "" for the placeholders aggregators use when they do not know the
// unit. That empty result is the point: this value is a cross-check, never a
// match key, and an unknown unit must not be mistaken for unit "N/A".
func NormalizeTruckRef(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	switch strings.ToUpper(t) {
	case "N/A", "NA", "-", "--", "NONE", "NULL", "UNKNOWN":
		return ""
	}
	if !isAllDigits(t) {
		return t
	}
	trimmed := strings.TrimLeft(t, "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}

// NormalizeDeviceID cleans a transponder id. Unlike a truck number, leading
// zeros are significant here — the agency tag "01606029503" is not the number
// 1606029503 — so only surrounding whitespace is removed.
func NormalizeDeviceID(s string) string {
	return strings.TrimSpace(s)
}

const hashSep = "\x1f" // ASCII unit separator: cannot occur in spreadsheet text

// HashRow builds the stable dedup key for a toll row.
//
// The aggregator issues no transaction id of its own, so "the same crossing"
// has to be recognised from content. Only fields that cannot change between a
// file and its re-send are included:
//
//   - Cl and Mi are excluded because they are absent from some files entirely
//     — hashing them would give the same crossing two different keys.
//   - Entry data is excluded because it is frequently blank and gets
//     backfilled in corrected files.
//   - The carrier's truck number is excluded because the aggregator revises it.
//
// occurrence disambiguates rows that are otherwise identical, which can only
// happen when the file omits the exit timestamp. It is the row's index among
// its identical siblings in file order.
//
// ponytail: occurrence is stable for an identical re-send and for the
// chronologically-ordered files aggregators actually emit, but a re-ordered
// file with blank exit times would re-key those rows. Switch to a
// provider-supplied transaction id the moment any aggregator offers one.
func HashRow(pt ProviderType, r Row, occurrence int) string {
	exit := ""
	if r.ExitAt != nil {
		exit = r.ExitAt.UTC().Format(time.RFC3339)
	}
	parts := []string{
		string(pt),
		r.PostDate.UTC().Format("2006-01-02"),
		r.DeviceID,
		r.AgencyRef,
		r.Plate,
		exit,
		r.ExitPlaza,
		r.Agency,
		r.Amount.StringFixed(2),
		strconv.Itoa(occurrence),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, hashSep)))
	return hex.EncodeToString(sum[:])
}

// assignHashes fills Hash on every row, numbering identical rows so a file
// that omits exit timestamps still yields distinct keys.
func assignHashes(pt ProviderType, rows []Row) {
	seen := make(map[string]int, len(rows))
	for i := range rows {
		base := HashRow(pt, rows[i], 0)
		n := seen[base]
		seen[base] = n + 1
		if n == 0 {
			rows[i].Hash = base
			continue
		}
		rows[i].Hash = HashRow(pt, rows[i], n)
	}
}

func isAlpha(b byte) bool { return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') }

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}
