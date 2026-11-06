package tmsgraphql

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/response"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// verdictSchema stands in for a service schema: exec runs where a resolver
// would, so the test drives the real NewHandler and presenter over HTTP.
type verdictSchema struct{ exec func(ctx context.Context) }

var verdictSDL = gqlparser.MustLoadSchema(&ast.Source{Input: "type Query { ok: Boolean } type Mutation { act: Boolean }"})

func (verdictSchema) Schema() *ast.Schema { return verdictSDL }
func (verdictSchema) Complexity(context.Context, string, string, int, map[string]any) (int, bool) {
	return 0, false
}
func (s verdictSchema) Exec(context.Context) graphql.ResponseHandler {
	return func(ctx context.Context) *graphql.Response {
		s.exec(ctx)
		return &graphql.Response{Data: []byte(`{"act":true}`)}
	}
}

type wireError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

type wireResponse struct {
	Errors     []wireError `json:"errors"`
	Extensions struct {
		RuleWarnings []RuleVerdict `json:"ruleWarnings"`
	} `json:"extensions"`
	raw map[string]any
}

func postMutation(t *testing.T, exec func(ctx context.Context)) wireResponse {
	t.Helper()
	srv := NewHandler(verdictSchema{exec: exec}, false)
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(`{"query":"mutation { act }"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var out wireResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.raw)
	return out
}

func TestRuleWarningsReachTheResponse(t *testing.T) {
	tricky := "Long trip\nneeds a \"team\" crew (rule 7)"
	resp := postMutation(t, func(ctx context.Context) {
		AddRuleWarnings(ctx, RuleVerdict{RuleID: "tenant:a", Ref: 7, Message: tricky, Effect: "WARN"})
		AddRuleWarnings(ctx, RuleVerdict{RuleID: "load.trip.hot", Message: "Hot load", Effect: "WARN"})
		// The same rule reached twice in one action (nested write) is one warning.
		AddRuleWarnings(ctx, RuleVerdict{RuleID: "tenant:a", Ref: 7, Message: tricky, Effect: "WARN"})
	})

	got := resp.Extensions.RuleWarnings
	if len(got) != 2 {
		t.Fatalf("want 2 warnings, got %+v", got)
	}
	if got[0] != (RuleVerdict{RuleID: "tenant:a", Ref: 7, Message: tricky, Effect: "WARN"}) {
		t.Errorf("first warning = %+v", got[0])
	}
	if got[1].RuleID != "load.trip.hot" || got[1].Ref != 0 {
		t.Errorf("second warning = %+v", got[1])
	}
}

func TestNoRuleWarningsNoExtension(t *testing.T) {
	resp := postMutation(t, func(context.Context) {})
	if ext, ok := resp.raw["extensions"].(map[string]any); ok {
		if _, has := ext["ruleWarnings"]; has {
			t.Fatalf("quiet rules must leave no ruleWarnings, got %v", ext)
		}
	}
}

func TestRuleDenialCarriesEveryRefusal(t *testing.T) {
	withCaptureSpy(t)
	denials := []RuleVerdict{
		{RuleID: "tenant:credit", Ref: 3, Message: "Customer is on credit stop (rule 3)", Effect: "BLOCK"},
		{RuleID: "tenant:rate", Ref: 9, Message: "Rate below minimum (rule 9)", Effect: "BLOCK"},
	}
	resp := postMutation(t, func(ctx context.Context) {
		AddRuleWarnings(ctx, RuleVerdict{RuleID: "tenant:w", Ref: 4, Message: "Heads up (rule 4)", Effect: "WARN"})
		ext := RuleDenialExtensions(map[string]any{"blockingTripId": "t-1"}, denials)
		graphql.AddError(ctx, response.NewCodedError("TENANT_RULE_DENIED", "denied", denials[0].Message, 422, ext))
	})

	if len(resp.Errors) != 1 {
		t.Fatalf("want one error, got %+v", resp.Errors)
	}
	e := resp.Errors[0]
	if e.Message != denials[0].Message {
		t.Errorf("message = %q, want the first refusal", e.Message)
	}
	want := map[string]any{"code": "TENANT_RULE_DENIED", "ruleId": "tenant:credit", "ref": float64(3),
		"effect": "BLOCK", "blockingTripId": "t-1"}
	for k, v := range want {
		if e.Extensions[k] != v {
			t.Errorf("extensions[%s] = %v, want %v", k, e.Extensions[k], v)
		}
	}
	all, _ := e.Extensions["denials"].([]any)
	if len(all) != 2 {
		t.Fatalf("denials = %v, want both refusals", e.Extensions["denials"])
	}
	if second, _ := all[1].(map[string]any); second["ruleId"] != "tenant:rate" || second["ref"] != float64(9) {
		t.Errorf("second denial = %v", all[1])
	}
	if len(resp.Extensions.RuleWarnings) != 1 {
		t.Errorf("a warning of the refused operation still reaches the answer, got %+v", resp.Extensions.RuleWarnings)
	}
}

func TestRuleDenialExtensionsForCodeRule(t *testing.T) {
	ext := RuleDenialExtensions(nil, []RuleVerdict{{RuleID: "load.x", Message: "no", Effect: "BLOCK"}})
	if _, has := ext["ref"]; has {
		t.Errorf("a rule without a number has no ref, got %v", ext)
	}
	if ext["ruleId"] != "load.x" || ext["effect"] != "BLOCK" {
		t.Errorf("ext = %v", ext)
	}
}

func TestRuleWarningsFromParallelResolvers(t *testing.T) {
	resp := postMutation(t, func(ctx context.Context) {
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				AddRuleWarnings(ctx, RuleVerdict{RuleID: "r", Message: strings.Repeat("x", i+1), Effect: "WARN"})
			}()
		}
		wg.Wait()
	})
	if len(resp.Extensions.RuleWarnings) != 20 {
		t.Fatalf("want 20 warnings, got %d", len(resp.Extensions.RuleWarnings))
	}
}

func TestAddRuleWarningsOutsideOperationIsNoop(t *testing.T) {
	AddRuleWarnings(context.Background(), RuleVerdict{RuleID: "r", Message: "m"})
}
