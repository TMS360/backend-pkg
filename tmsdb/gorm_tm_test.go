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

// nestedObj stands in for an association (e.g. a Shipment) whose value would
// render as an unreadable Go map dump if it ever leaked into a Change.
type nestedObj struct {
	Foo string `json:"foo"`
	Bar int    `json:"bar"`
}

// auditChangeStruct mixes scalar columns (which must diff) with object fields
// (nested struct / pointer-to-struct / map / slice-of-struct — which must be
// skipped). uuid.UUID ([16]byte) and []byte are scalars, NOT byte-slice objects.
type auditChangeStruct struct {
	// scalars — must diff
	Status    string    `json:"status"`
	Miles     int       `json:"miles"`
	UpdatedAt time.Time `json:"updated_at"`
	ID        uuid.UUID `json:"id"`
	Blob      []byte    `json:"blob"`

	// object fields — must be skipped (scalar-only contract)
	Shipment    nestedObj         `json:"shipment"`
	ShipmentPtr *nestedObj        `json:"shipment_ptr"`
	Meta        map[string]string `json:"meta"`
	Stops       []nestedObj       `json:"stops"`
}

// TestCalculateChanges_ObjectFieldsAreSkipped is the regression guard: a diff
// that touches ONLY object fields must yield zero Change entries. If the
// isAssociationField skip is removed from CalculateChanges, this fails.
func TestCalculateChanges_ObjectFieldsAreSkipped(t *testing.T) {
	old := auditChangeStruct{
		Shipment:    nestedObj{Foo: "a", Bar: 1},
		ShipmentPtr: &nestedObj{Foo: "a", Bar: 1},
		Meta:        map[string]string{"k": "v1"},
		Stops:       []nestedObj{{Foo: "s1"}},
	}
	newV := auditChangeStruct{
		Shipment:    nestedObj{Foo: "b", Bar: 2},           // struct changed
		ShipmentPtr: &nestedObj{Foo: "b", Bar: 2},          // pointer-to-struct changed
		Meta:        map[string]string{"k": "v2"},          // map changed
		Stops:       []nestedObj{{Foo: "s2"}, {Foo: "s3"}}, // slice-of-struct changed
	}

	if got := CalculateChanges(old, newV); len(got) != 0 {
		t.Fatalf("object fields must not produce changes, got %d: %+v", len(got), got)
	}
}

// TestCalculateChanges_ScalarsStillDiff pins the other half of the contract:
// scalar columns — including the tricky time.Time / uuid.UUID / []byte cases —
// must still diff even while object fields are skipped.
func TestCalculateChanges_ScalarsStillDiff(t *testing.T) {
	t1 := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// Identical object fields so any change can only come from scalars.
	obj := nestedObj{Foo: "same", Bar: 7}
	old := auditChangeStruct{
		Status: "NEW", Miles: 100, UpdatedAt: t1, ID: id1, Blob: []byte("a"),
		Shipment: obj, ShipmentPtr: &obj, Meta: map[string]string{"k": "v"}, Stops: []nestedObj{{Foo: "s"}},
	}
	newV := auditChangeStruct{
		Status: "PICKED_UP", Miles: 250, UpdatedAt: t2, ID: id2, Blob: []byte("b"),
		Shipment: obj, ShipmentPtr: &obj, Meta: map[string]string{"k": "v"}, Stops: []nestedObj{{Foo: "s"}},
	}

	changes := CalculateChanges(old, newV)
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Field] = true
	}

	for _, field := range []string{"status", "miles", "updated_at", "id", "blob"} {
		if !seen[field] {
			t.Errorf("expected scalar field %q to diff, got changes: %+v", field, changes)
		}
	}
	if len(changes) != 5 {
		t.Fatalf("expected exactly 5 scalar changes, got %d: %+v", len(changes), changes)
	}
}
