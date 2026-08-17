package tmsdb

import (
	"context"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/google/uuid"
)

// DEV-1732: the event writer resolved the company by reaching through
// actor.Claims, which a system actor does not have. The nil deref panicked and
// killed the whole workspaces process on every boot (the recurring column-group
// job runs immediately at startup), so Apollo could not introspect the subgraph
// and the page 504'd.
//
// This exercises the production resolution used by writeEvent, not the accessor
// next to it — reverting the fix must fail here. The DB is not involved: the
// panic happened before any query.
func TestActorIdentity_SystemActorHasNoCompanyAndDoesNotPanic(t *testing.T) {
	ctx := middleware.WithSystemActor(context.Background())

	actorID, companyID := actorIdentity(ctx)

	if companyID != nil {
		t.Fatalf("system actor must stamp no company, got %v", *companyID)
	}
	if actorID == nil || *actorID != uuid.Nil {
		t.Fatalf("system actor id must still be stamped, got %v", actorID)
	}
}

// A real user still stamps their company — the guard must not blank it out.
func TestActorIdentity_UserActorKeepsCompany(t *testing.T) {
	company := uuid.New()
	user := uuid.New()
	ctx := middleware.WithActor(context.Background(), &consts.Actor{
		ID:     user,
		Claims: &consts.UserClaims{UserID: user, CompanyID: &company},
	})

	actorID, companyID := actorIdentity(ctx)

	if companyID == nil || *companyID != company {
		t.Fatalf("user actor must stamp its company, got %v", companyID)
	}
	if actorID == nil || *actorID != user {
		t.Fatalf("user actor id must be stamped, got %v", actorID)
	}
}

// No actor at all (background job with a bare context) is not a crash either.
func TestActorIdentity_NoActor(t *testing.T) {
	actorID, companyID := actorIdentity(context.Background())
	if actorID != nil || companyID != nil {
		t.Fatalf("a context without an actor stamps nothing, got %v / %v", actorID, companyID)
	}
}

type changeTestStruct struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Note  string `json:"note,omitempty"`
}

func TestCalculateChanges_AcceptsValueAndPointerShapes(t *testing.T) {
	oldV := changeTestStruct{Name: "alice", Count: 1, Note: "before"}
	newV := changeTestStruct{Name: "alice", Count: 2, Note: "after"}

	cases := []struct {
		name string
		old  interface{}
		new  interface{}
	}{
		{"value,value", oldV, newV},
		{"pointer,pointer", &oldV, &newV},
		{"value,pointer", oldV, &newV},
		{"pointer,value", &oldV, newV},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changes := CalculateChanges(tc.old, tc.new)
			if len(changes) != 2 {
				t.Fatalf("expected 2 changes (count, note), got %d: %+v", len(changes), changes)
			}
			seen := map[string]bool{}
			for _, c := range changes {
				seen[c.Field] = true
			}
			if !seen["count"] || !seen["note"] {
				t.Fatalf("expected changes on count and note, got %+v", changes)
			}
		})
	}
}

func TestCalculateChanges_NilInputsReturnEmpty(t *testing.T) {
	v := changeTestStruct{Name: "alice"}

	if got := CalculateChanges(nil, &v); len(got) != 0 {
		t.Fatalf("nil old: expected empty, got %+v", got)
	}
	if got := CalculateChanges(&v, nil); len(got) != 0 {
		t.Fatalf("nil new: expected empty, got %+v", got)
	}
	if got := CalculateChanges(nil, nil); len(got) != 0 {
		t.Fatalf("both nil: expected empty, got %+v", got)
	}
}

func TestCalculateChanges_TypedNilPointerReturnsEmpty(t *testing.T) {
	var typedNil *changeTestStruct
	v := changeTestStruct{Name: "alice"}

	if got := CalculateChanges(typedNil, &v); len(got) != 0 {
		t.Fatalf("typed nil old: expected empty, got %+v", got)
	}
	if got := CalculateChanges(&v, typedNil); len(got) != 0 {
		t.Fatalf("typed nil new: expected empty, got %+v", got)
	}
}

func TestCalculateChanges_IdenticalReturnsEmpty(t *testing.T) {
	v := changeTestStruct{Name: "alice", Count: 1, Note: "same"}
	if got := CalculateChanges(v, v); len(got) != 0 {
		t.Fatalf("identical values: expected empty, got %+v", got)
	}
	if got := CalculateChanges(&v, &v); len(got) != 0 {
		t.Fatalf("identical pointers: expected empty, got %+v", got)
	}
}
