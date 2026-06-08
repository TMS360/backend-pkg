package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestLoadConfigFromEnv_PrefersSentryEnvironment(t *testing.T) {
	t.Setenv("SENTRY_ENVIRONMENT", "stage")
	t.Setenv("SENTRY_ENV", "should-be-ignored")
	t.Setenv("SENTRY_DSN", "")
	t.Setenv("SENTRY_RELEASE", "")

	got := LoadConfigFromEnv("svc")
	if got.Env != "stage" {
		t.Fatalf("Env = %q, want %q (SENTRY_ENVIRONMENT must win over SENTRY_ENV)", got.Env, "stage")
	}
}

func TestLoadConfigFromEnv_FallsBackToSentryEnv(t *testing.T) {
	t.Setenv("SENTRY_ENVIRONMENT", "")
	t.Setenv("SENTRY_ENV", "legacy")
	t.Setenv("SENTRY_DSN", "")
	t.Setenv("SENTRY_RELEASE", "")

	got := LoadConfigFromEnv("svc")
	if got.Env != "legacy" {
		t.Fatalf("Env = %q, want %q (SENTRY_ENV must be used when SENTRY_ENVIRONMENT is empty)", got.Env, "legacy")
	}
}

func TestInit_MissingDSN_WarnsAndReturnsFalse(t *testing.T) {
	buf := &bytes.Buffer{}
	swapDefaultLogger(t, buf)

	ok := Init(Config{Service: "svc", Env: "dev"})
	if ok {
		t.Fatal("Init should return false when DSN is empty")
	}
	out := buf.String()
	if !strings.Contains(out, `"level":"WARN"`) {
		t.Errorf("expected WARN-level log, got: %s", out)
	}
	if !strings.Contains(out, "Sentry disabled") {
		t.Errorf("expected disabled message, got: %s", out)
	}
}

func TestInit_MissingDSN_ProdEscalatesToError(t *testing.T) {
	buf := &bytes.Buffer{}
	swapDefaultLogger(t, buf)

	ok := Init(Config{Service: "svc", Env: "prod"})
	if ok {
		t.Fatal("Init should return false when DSN is empty")
	}
	out := buf.String()
	if !strings.Contains(out, `"level":"ERROR"`) {
		t.Errorf("expected ERROR-level log in prod, got: %s", out)
	}
}

func TestRecoverGoroutine_NoPanicIsNoop(t *testing.T) {
	defer RecoverGoroutine(context.Background())
	// No panic — RecoverGoroutine must return cleanly.
}

func TestRecoverGoroutine_RecoversFromPanic(t *testing.T) {
	buf := &bytes.Buffer{}
	swapDefaultLogger(t, buf)

	func() {
		defer RecoverGoroutine(context.Background())
		panic("kaboom")
	}()

	out := buf.String()
	if !strings.Contains(out, "goroutine recovered from panic") {
		t.Errorf("expected recovered log, got: %s", out)
	}
	if !strings.Contains(out, "kaboom") {
		t.Errorf("expected panic value in log, got: %s", out)
	}
}

func swapDefaultLogger(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
}
