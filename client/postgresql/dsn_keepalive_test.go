package postgresql

import (
	"strings"
	"testing"
)

// The pool hands out connections the platform may have silently killed. Without
// keepalives a write into one of those blocks until the CALLER's deadline —
// thirty seconds behind the router — and Postgres never sees the query, so the
// incident reads as a database fault while pg_locks and pg_stat_activity are
// clean. These settings are the only thing that makes a dead peer detectable,
// so pin them: dropping one is silent and only shows up in production.
func TestTCPKeepaliveDSNPinsTheSettings(t *testing.T) {
	want := map[string]string{
		"connect_timeout":     "5",  // a fresh dial must not inherit the open-ended wait
		"keepalives":          "1",  // off by default in libpq — this is the switch
		"keepalives_idle":     "30", // probe after 30s of silence
		"keepalives_interval": "10",
		"keepalives_count":    "3", // dead peer surfaces in ~60s, not at the next write
	}
	got := map[string]string{}
	for _, kv := range strings.Fields(tcpKeepaliveDSN) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			t.Fatalf("malformed DSN fragment %q", kv)
		}
		got[k] = v
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("DSN fragment has %d settings, want %d: %q", len(got), len(want), tcpKeepaliveDSN)
	}
}
