package utils

import (
	"regexp"
	"strings"
)

var (
	slugReplaceAmpersand = strings.NewReplacer("&", " and ")
	slugNonAlnum         = regexp.MustCompile(`[^A-Za-z0-9]+`)
	slugTrim             = regexp.MustCompile(`^_+|_+$`)
)

// SlugifySnake turns a display name into a stable snake_case key.
// Example: "DRIVER POLICY & SAFETY MANUAL" -> "driver_policy_and_safety_manual".
// The transformation must stay in sync with the SQL backfill in
// tms-files/database/migrations/...add_unique_name_doc_types.sql.
func SlugifySnake(s string) string {
	s = slugReplaceAmpersand.Replace(s)
	s = slugNonAlnum.ReplaceAllString(s, "_")
	s = slugTrim.ReplaceAllString(s, "")
	return strings.ToLower(s)
}
