package settings

import (
	"context"
	"fmt"

	"github.com/TMS360/backend-pkg/cache"
	"github.com/TMS360/backend-pkg/enums"
)

// GetCompanyTimezone reads the company's timezone setting (default UTC) so the daily
// cap resets on the company-local day.
func GetCompanyTimezone(ctx context.Context) string {
	var tz string
	_ = cache.Get(ctx, fmt.Sprintf("setting:%s", enums.CompanySettingsGeneralKeyTimezone), &tz)
	if tz == "" {
		return "UTC"
	}
	return tz
}
