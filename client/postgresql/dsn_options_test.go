package postgresql

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The DSN goes to pgx, not libpq. pgx forwards any key it does not recognise to
// the server as a runtime parameter, and Postgres answers
// "FATAL: unrecognized configuration parameter" — which takes the service down
// on start, not at first use. libpq's keepalives* keys did exactly that.
//
// So this pins the negative: only options pgx actually understands may appear.
func TestConnDSNOptionsCarryNothingPgxRejects(t *testing.T) {
	// libpq-only keys that pgx forwards to the server verbatim.
	forbidden := []string{"keepalives", "keepalives_idle", "keepalives_interval", "keepalives_count"}

	got := map[string]string{}
	for _, kv := range strings.Fields(connDSNOptions) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			t.Fatalf("malformed DSN fragment %q", kv)
		}
		got[k] = v
	}
	for _, k := range forbidden {
		if _, bad := got[k]; bad {
			t.Errorf("%q is a libpq-only key; pgx sends it to the server and the connection dies with SQLSTATE 42704", k)
		}
	}
	if got["connect_timeout"] != "5" {
		t.Errorf("connect_timeout = %q, want 5 — a dial must not wait open-endedly", got["connect_timeout"])
	}
}

// The negative list above only catches keys we already know about. This catches
// the CLASS: parse the real DSN with pgx and assert it kept nothing as a runtime
// parameter. Anything pgx does not understand lands in RuntimeParams and is sent
// to the server at startup, which is what turned a connection tweak into a
// service that could not boot.
func TestConnDSNOptionsParseAsClientSettings(t *testing.T) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s %s",
		"localhost", "u", "p", "d", "5432", "disable", "UTC", connDSNOptions)

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx cannot parse the DSN we build: %v", err)
	}
	for k, v := range cfg.RuntimeParams {
		// TimeZone is a genuine server setting and belongs here.
		if k == "timezone" || k == "TimeZone" {
			continue
		}
		t.Errorf("%q=%q would be sent to the server as a runtime parameter; Postgres answers FATAL for anything it does not know", k, v)
	}
	if cfg.ConnectTimeout == 0 {
		t.Error("connect_timeout did not reach pgx — a dial can wait open-endedly")
	}
}
