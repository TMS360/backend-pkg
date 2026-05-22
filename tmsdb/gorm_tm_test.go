package tmsdb

import (
	"testing"
)

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
