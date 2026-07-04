package model

import (
	"encoding/json"
	"testing"
)

// The 22P02 fix: Value must hand Postgres a string (JSON text), not a []byte.
func TestJSONRaw_ValueIsStringForSimpleProtocol(t *testing.T) {
	j := JSONRaw(`{"a":1}`)
	v, err := j.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("Value type = %T, want string (bytes trigger 22P02 under simple protocol)", v)
	}
	if s != `{"a":1}` {
		t.Fatalf("Value = %q, want %q", s, `{"a":1}`)
	}

	// Empty → SQL NULL.
	nv, err := JSONRaw(nil).Value()
	if err != nil || nv != nil {
		t.Fatalf("empty Value = (%v,%v), want (nil,nil)", nv, err)
	}
}

// The base64 trap: a struct field of type JSONRaw must serialize as raw JSON,
// exactly like json.RawMessage — not base64.
func TestJSONRaw_MarshalMatchesRawMessage(t *testing.T) {
	type payload struct {
		Meta JSONRaw `json:"meta"`
	}
	out, err := json.Marshal(payload{Meta: JSONRaw(`{"k":"v"}`)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != `{"meta":{"k":"v"}}` {
		t.Fatalf("marshal = %s, want raw JSON (a base64 string here means MarshalJSON is missing)", out)
	}

	// Empty field → null.
	out, err = json.Marshal(payload{})
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if string(out) != `{"meta":null}` {
		t.Fatalf("marshal empty = %s, want {\"meta\":null}", out)
	}
}

// Round-trip: DB Scan then JSON Marshal, and JSON Unmarshal then DB Value.
func TestJSONRaw_RoundTrip(t *testing.T) {
	var j JSONRaw
	if err := j.Scan([]byte(`{"x":[1,2,3]}`)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	b, err := j.MarshalJSON()
	if err != nil || string(b) != `{"x":[1,2,3]}` {
		t.Fatalf("MarshalJSON after Scan = (%s,%v)", b, err)
	}

	var k JSONRaw
	if err := k.UnmarshalJSON([]byte(`{"y":true}`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, err := k.Value()
	if err != nil || v.(string) != `{"y":true}` {
		t.Fatalf("Value after UnmarshalJSON = (%v,%v)", v, err)
	}
}
