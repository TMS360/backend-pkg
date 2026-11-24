package tmsgraphql

import (
	"context"
	"fmt"
	"sync"

	"github.com/99designs/gqlgen/graphql"
)

// RuleVerdict is one rule decision in the shape the app reads (DEV-3265):
// a warning in extensions.ruleWarnings, or an entry of a refusal's
// extensions.denials. Ref is the rule's number inside the company; it is
// omitted for rules that have none (rules declared in code).
type RuleVerdict struct {
	RuleID  string `json:"ruleId"`
	Ref     int    `json:"ref,omitempty"`
	Message string `json:"message"`
	Effect  string `json:"effect,omitempty"`
}

// ruleWarningsExtension is the response extensions key the app and the
// gateway script read.
const ruleWarningsExtension = "ruleWarnings"

type ruleWarningsKey struct{}

// ruleWarnings collects the warnings of one GraphQL operation. Resolvers may
// run in parallel, hence the mutex.
type ruleWarnings struct {
	mu   sync.Mutex
	list []RuleVerdict
}

// AddRuleWarnings records rule warnings for the current GraphQL response.
// Outside a GraphQL operation (Kafka consumers, workers) there is no answer to
// carry them, so it does nothing. The same rule and text are kept once.
func AddRuleWarnings(ctx context.Context, ws ...RuleVerdict) {
	c, ok := ctx.Value(ruleWarningsKey{}).(*ruleWarnings)
	if !ok || len(ws) == 0 {
		return
	}
	defer recoverRuleWarnings(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range ws {
		if !c.has(w) {
			c.list = append(c.list, w)
		}
	}
}

func (c *ruleWarnings) has(w RuleVerdict) bool {
	for _, x := range c.list {
		if x.RuleID == w.RuleID && x.Message == w.Message {
			return true
		}
	}
	return false
}

// RuleDenialExtensions is the extensions of a rule refusal: the rule's own
// payload (Ctx.DenyWith), then ruleId, ref and effect of the first refusal and
// every refusal in denials. The first refusal's text stays the error message.
func RuleDenialExtensions(payload map[string]any, denials []RuleVerdict) map[string]any {
	ext := make(map[string]any, len(payload)+4)
	for k, v := range payload {
		ext[k] = v
	}
	if len(denials) == 0 {
		return ext
	}
	first := denials[0]
	ext["ruleId"] = first.RuleID
	if first.Ref > 0 {
		ext["ref"] = first.Ref
	}
	if first.Effect != "" {
		ext["effect"] = first.Effect
	}
	ext["denials"] = denials
	return ext
}

// collectRuleWarnings gives every operation its own collector.
func collectRuleWarnings(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	return next(context.WithValue(ctx, ruleWarningsKey{}, &ruleWarnings{}))
}

// attachRuleWarnings puts the collected warnings into the response
// extensions. A failure here loses the warnings, never the answer: the named
// result keeps resp when recoverRuleWarnings swallows a panic.
func attachRuleWarnings(ctx context.Context, next graphql.ResponseHandler) (resp *graphql.Response) {
	resp = next(ctx)
	if resp == nil {
		return nil
	}
	c, ok := ctx.Value(ruleWarningsKey{}).(*ruleWarnings)
	if !ok {
		return resp
	}
	defer recoverRuleWarnings(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.list) == 0 {
		return resp
	}
	if resp.Extensions == nil {
		resp.Extensions = map[string]any{}
	}
	resp.Extensions[ruleWarningsExtension] = append([]RuleVerdict(nil), c.list...)
	return resp
}

func recoverRuleWarnings(ctx context.Context) {
	if r := recover(); r != nil {
		captureFunc(ctx, fmt.Errorf("rule warnings collector: %v", r))
	}
}
