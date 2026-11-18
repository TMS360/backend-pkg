package importeddetail

import (
	"regexp"
	"strings"
)

const dots = "••••••"

var yearRe = regexp.MustCompile(`(19|20)\d{2}`)

// lastFourKey reports keys whose value is shown as its last 4 digits.
func lastFourKey(key string) bool {
	k := strings.ToLower(key)
	for _, w := range []string{"ssn", "ein", "tax_id", "account", "routing", "card"} {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

func dateKey(key string) bool {
	k := strings.ToLower(key)
	return k == "birth_date" || k == "dob" || strings.HasSuffix(k, "_date") || strings.HasSuffix(k, "date_of_birth")
}

// Mask is what a holder of private_data_view reads: SSN, EIN, account,
// routing and card numbers as their last 4 digits (SSN `•••-••-6789`), dates
// as `••/••/YYYY`, any other private text as dots.
func Mask(key, value string) string {
	switch {
	case lastFourKey(key):
		digits := onlyDigits(value)
		if len(digits) < 4 {
			return dots
		}
		last := digits[len(digits)-4:]
		if k := strings.ToLower(key); strings.Contains(k, "ssn") && len(digits) == 9 {
			return "•••-••-" + last
		}
		return dots + last
	case dateKey(key):
		if y := yearRe.FindString(value); y != "" {
			return "••/••/" + y
		}
		return dots
	default:
		return dots
	}
}

// MaskNone is what a reader without private_data_view reads: dots only.
func MaskNone(string, string) string { return dots }

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
