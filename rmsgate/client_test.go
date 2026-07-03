package rmsgate

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/TMS360/backend-pkg/proto/rmsgate"
)

// fakeGate — управляемый сервер RmsGate для bufconn-тестов.
type fakeGate struct {
	pb.UnimplementedRmsGateServer
	resp  *pb.Decision
	delay time.Duration
	calls int
}

func (f *fakeGate) Evaluate(ctx context.Context, req *pb.EvaluateRequest) (*pb.Decision, error) {
	f.calls++
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.resp, nil
}

// newBufClient поднимает fakeGate на bufconn и возвращает подключённый Client.
func newBufClient(t *testing.T, fg *fakeGate, opts ...Option) *Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterRmsGateServer(srv, fg)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return NewFromConn(conn, opts...)
}

func TestDecideAllowAndStepsParsed(t *testing.T) {
	fg := &fakeGate{resp: &pb.Decision{
		Allow:         true,
		RequiredSteps: []string{"override:driver busy", "notify:sms:dispatch"},
	}}
	c := newBufClient(t, fg)

	dec := c.Decide(context.Background(), "shipment", "A->B", "acme", map[string]any{"x": 1})
	require.True(t, dec.Allow)
	assert.False(t, dec.FailedOpen)
	require.Len(t, dec.RequiredSteps, 2)
	assert.Equal(t, Step{Kind: "override", Arg: "driver busy"}, dec.RequiredSteps[0])
	assert.Equal(t, Step{Kind: "notify", Arg: "sms:dispatch"}, dec.RequiredSteps[1])
}

func TestDecideDeny(t *testing.T) {
	fg := &fakeGate{resp: &pb.Decision{Allow: false, Reasons: []string{"documents missing"}}}
	c := newBufClient(t, fg)
	dec := c.Decide(context.Background(), "shipment", "A->B", "acme", nil)
	assert.False(t, dec.Allow)
	assert.Equal(t, []string{"documents missing"}, dec.Reasons)
}

// Таймаут гейта → вердикт по FailMode процесса, error наружу не отдаётся.
func TestDecideTimeoutAppliesFailMode(t *testing.T) {
	fg := &fakeGate{resp: &pb.Decision{Allow: true}, delay: 200 * time.Millisecond}
	reg := NewRegistry(
		ProcessDef{Process: "shipment", Mode: FailOpen, Timeout: 30 * time.Millisecond},
		ProcessDef{Process: "billing", Mode: FailClosed, Timeout: 30 * time.Millisecond},
	)
	c := newBufClient(t, fg, WithFailModes(reg))

	open := c.Decide(context.Background(), "shipment", "A->B", "acme", nil)
	assert.True(t, open.Allow, "RiskSafe процесс fail-open")
	assert.True(t, open.FailedOpen)
	require.NotEmpty(t, open.Reasons)
	assert.Contains(t, open.Reasons[0], "fail-open")

	closed := c.Decide(context.Background(), "billing", "MARK_TONU", "acme", nil)
	assert.False(t, closed.Allow, "денежный процесс fail-closed")
	assert.True(t, closed.FailedOpen)
}

// Неизвестный процесс = FailClosed (безопасный дефолт).
func TestDecideUnknownProcessFailsClosed(t *testing.T) {
	fg := &fakeGate{resp: &pb.Decision{Allow: true}, delay: 200 * time.Millisecond}
	c := newBufClient(t, fg, WithTimeout(30*time.Millisecond)) // реестра нет вовсе
	dec := c.Decide(context.Background(), "mystery", "X->Y", "acme", nil)
	assert.False(t, dec.Allow)
	assert.True(t, dec.FailedOpen)
}

// Circuit: после порога подряд ошибок клиент отвечает мгновенно (не ходит в RMS).
func TestDecideCircuitOpensAfterConsecutiveFailures(t *testing.T) {
	fg := &fakeGate{resp: &pb.Decision{Allow: true}, delay: 100 * time.Millisecond}
	reg := NewRegistry(ProcessDef{Process: "shipment", Mode: FailOpen, Timeout: 10 * time.Millisecond})
	c := newBufClient(t, fg, WithFailModes(reg))

	for i := 0; i < circuitThreshold; i++ {
		_ = c.Decide(context.Background(), "shipment", "A->B", "acme", nil)
	}
	callsBefore := fg.calls
	start := time.Now()
	dec := c.Decide(context.Background(), "shipment", "A->B", "acme", nil)
	assert.True(t, dec.FailedOpen)
	assert.Contains(t, dec.Reasons[0], "circuit open")
	assert.Equal(t, callsBefore, fg.calls, "в открытом circuit RMS не вызывается")
	assert.Less(t, time.Since(start), 10*time.Millisecond, "ответ мгновенный")
}
