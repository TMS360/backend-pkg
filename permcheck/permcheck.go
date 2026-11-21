// Package permcheck is the build check behind DEV-4176: every permission code
// a service checks must exist in enums (the list the role editor reads), so a
// right that nobody can tick fails the service's tests instead of shutting an
// action for everyone in production.
//
// A service calls Unknown from one test in its tests/ tree:
//
//	func TestCheckedPermissionsAreKnown(t *testing.T) {
//		found, err := permcheck.Unknown("../..")
//		require.NoError(t, err)
//		assert.Empty(t, found)
//	}
package permcheck

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/TMS360/backend-pkg/enums"
)

// Finding is one checked code that enums does not know.
type Finding struct {
	Code string
	File string
	Line int
}

func (f Finding) String() string { return fmt.Sprintf("%s:%d %q", f.File, f.Line, f.Code) }

var (
	// hasPermRe captures the perms list of a @hasPerm directive; the list may
	// span lines, so it runs on the whole schema file.
	hasPermRe = regexp.MustCompile(`@hasPerm\s*\(\s*perms\s*:\s*\[([^\]]*)\]`)
	// goCallRe marks a Go line that checks a permission by literal.
	goCallRe = regexp.MustCompile(`\b(HasPermission|HasAnyPermission|HasAllPermissions|RequirePerms|GetUsersByPermission|UsersWithPermission)\(`)
	// codeRe matches a quoted permission code, hyphen included
	// (compliance.receive-alerts).
	codeRe = regexp.MustCompile(`"([a-z][a-z0-9_-]*(?:\.[a-z][a-z0-9_-]*)*)"`)
)

// skipDir lists directories that never hold a hand-written check.
func skipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "tests", "mocks":
		return true
	}
	return false
}

// generated reports files gqlgen/wire/mockgen write; their codes come from the
// schema files, which are scanned on their own.
func generated(name string) bool {
	return name == "generated.go" || name == "models_gen.go" || name == "federation.go" ||
		strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go")
}

// Unknown walks root and returns every permission code checked by a
// @hasPerm directive in *.graphql/*.graphqls, or passed as a string literal to
// a permission check in Go, that enums.IsKnownPermission rejects. Findings are
// sorted by file and line.
func Unknown(root string) ([]Finding, error) {
	var out []Finding
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		switch {
		case strings.HasSuffix(name, ".graphql") || strings.HasSuffix(name, ".graphqls"):
			found, err := scanSchema(path)
			if err != nil {
				return err
			}
			out = append(out, found...)
		case strings.HasSuffix(name, ".go") && !generated(name):
			found, err := scanGo(path)
			if err != nil {
				return err
			}
			out = append(out, found...)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, err
}

func scanSchema(path string) ([]Finding, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(src)
	var out []Finding
	for _, m := range hasPermRe.FindAllStringSubmatchIndex(text, -1) {
		line := 1 + strings.Count(text[:m[0]], "\n")
		for _, c := range codeRe.FindAllStringSubmatch(text[m[2]:m[3]], -1) {
			if !enums.IsKnownPermission(c[1]) {
				out = append(out, Finding{Code: c[1], File: path, Line: line})
			}
		}
	}
	return out, nil
}

func scanGo(path string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Finding
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "//") || !goCallRe.MatchString(line) {
			continue
		}
		for _, c := range codeRe.FindAllStringSubmatch(line, -1) {
			if !enums.IsKnownPermission(c[1]) {
				out = append(out, Finding{Code: c[1], File: path, Line: n})
			}
		}
	}
	return out, sc.Err()
}
