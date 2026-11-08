package tmsgraphql

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql/executor/testexecutor"
	"github.com/google/uuid"

	"github.com/TMS360/backend-pkg/consts"
)

// A mutation with the read-only token of the RMS service identity is refused
// by the shared handler before it reaches the schema; reads still work.
func TestReadOnlyTokenCannotMutate(t *testing.T) {
	srv := NewHandler(testexecutor.New().Schema(), false)
	company := uuid.New()

	do := func(readOnly bool, query string) map[string]any {
		t.Helper()
		claims := &consts.UserClaims{UserID: uuid.New(), CompanyID: &company, ReadOnly: readOnly}
		req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(`{"query":"`+query+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(consts.WithActor(req.Context(), &consts.Actor{ID: claims.UserID, Claims: claims}))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("bad response %q: %v", w.Body.String(), err)
		}
		return out
	}
	firstErr := func(out map[string]any) map[string]any {
		errs, _ := out["errors"].([]any)
		if len(errs) == 0 {
			return nil
		}
		return errs[0].(map[string]any)
	}

	e := firstErr(do(true, "mutation { name }"))
	if e == nil {
		t.Fatal("read-only mutation was not refused")
	}
	ext, _ := e["extensions"].(map[string]any)
	if ext["code"] != CodeReadOnlyToken || ext["status"] != float64(http.StatusForbidden) {
		t.Fatalf("want %s/403, got %v", CodeReadOnlyToken, e)
	}

	// The same mutation with an ordinary token reaches the schema (the test
	// schema answers mutations with its own error).
	if e := firstErr(do(false, "mutation { name }")); e == nil || e["message"] != "mutations are not supported" {
		t.Fatalf("ordinary token must reach the schema, got %v", e)
	}

	// Reads with the read-only token are not affected.
	out := do(true, "query { name }")
	if firstErr(out) != nil || out["data"] == nil {
		t.Fatalf("read-only query must pass, got %v", out)
	}
}
