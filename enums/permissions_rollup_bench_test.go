package enums_test

import (
	"testing"

	"github.com/TMS360/backend-pkg/enums"
)

// A realistic worst case: a user with every module fully granted (the whole
// catalog expanded to leaves) — the exact input that used to be returned raw.
func fullCatalogLeaves() []string {
	mods := enums.ModulePermissionCodes()
	return enums.ExpandPermissions(mods)
}

func BenchmarkRollupPermissions_FullCatalog(b *testing.B) {
	in := fullCatalogLeaves()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = enums.RollupPermissions(in)
	}
}

func BenchmarkExpandPermissions_AllModules(b *testing.B) {
	mods := enums.ModulePermissionCodes()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = enums.ExpandPermissions(mods)
	}
}
