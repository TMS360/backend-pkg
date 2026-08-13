package tests

import (
	"context"
	"net"
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	pb "github.com/TMS360/backend-pkg/proto/tasks"
	"github.com/TMS360/backend-pkg/tasks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Unit tests for the shared tasks client (DEV-1666). Wrapper logic (validation,
// idempotency surfacing, error wrapping) is tested against a fake generated
// client — no network. The auth/tenant propagation (AC4) is tested against a
// real gRPC call over an in-memory bufconn so the outgoing metadata is real.

// fakeTasksSvc is a fake pb.TasksServiceClient — the seam that lets a caller
// (and this test) exercise the client without a live tasks service (AC "mock
// server or interface").
type fakeTasksSvc struct {
	createResp  *pb.CreateTaskResponse
	createErr   error
	resolveResp *pb.ResolveTaskResponse
	resolveErr  error

	lastCreate   *pb.CreateTaskRequest
	lastResolve  *pb.ResolveTaskRequest
	createCalls  int
	resolveCalls int
}

func (f *fakeTasksSvc) CreateTask(_ context.Context, in *pb.CreateTaskRequest, _ ...grpc.CallOption) (*pb.CreateTaskResponse, error) {
	f.createCalls++
	f.lastCreate = in
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResp, nil
}

func (f *fakeTasksSvc) ResolveTask(_ context.Context, in *pb.ResolveTaskRequest, _ ...grpc.CallOption) (*pb.ResolveTaskResponse, error) {
	f.resolveCalls++
	f.lastResolve = in
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	return f.resolveResp, nil
}

func validCreate() tasks.CreateParams {
	return tasks.CreateParams{
		Key: tasks.Key{
			RuleID:             "rule-1",
			EntityType:         "load",
			EntityID:           uuid.New().String(),
			ValidationInstance: "missing_rate_con",
		},
		Title:        "Load is missing a rate confirmation",
		AssigneeType: "department",
		AssigneeID:   uuid.New().String(),
	}
}

// AC1: a caller depends on the shared client and calls CreateTask without any
// protobuf glue — params in, mapped result out.
func TestTasksClientCreateMapsResult(t *testing.T) {
	fake := &fakeTasksSvc{createResp: &pb.CreateTaskResponse{TaskId: "t-1", Status: "open", Created: true}}
	cli := tasks.NewWithClient(fake)

	res, err := cli.CreateForValidation(context.Background(), validCreate())
	require.NoError(t, err)
	require.Equal(t, "t-1", res.TaskID)
	require.Equal(t, "open", res.Status)
	require.True(t, res.Created)
	require.Equal(t, 1, fake.createCalls)
	// The whole DedupKey reached the wire.
	require.Equal(t, "rule-1", fake.lastCreate.GetKey().GetRuleId())
	require.Equal(t, "missing_rate_con", fake.lastCreate.GetKey().GetValidationInstance())
}

// AC2: the same rule+entity+validation twice yields one open task; the client
// surfaces created true then false (dedup/retry).
func TestTasksClientCreateIsIdempotentSurfaced(t *testing.T) {
	fake := &fakeTasksSvc{createResp: &pb.CreateTaskResponse{TaskId: "t-1", Status: "open", Created: true}}
	cli := tasks.NewWithClient(fake)
	p := validCreate()

	first, err := cli.CreateForValidation(context.Background(), p)
	require.NoError(t, err)
	require.True(t, first.Created, "first call created the task")

	// Same key again → server returns the existing open task.
	fake.createResp = &pb.CreateTaskResponse{TaskId: "t-1", Status: "open", Created: false}
	second, err := cli.CreateForValidation(context.Background(), p)
	require.NoError(t, err)
	require.False(t, second.Created, "second call returned the existing task")
	require.Equal(t, first.TaskID, second.TaskID)
}

// AC3: resolving when nothing is open is a clean no-op — no error, Resolved
// false.
func TestTasksClientResolveNoOpWhenNothingOpen(t *testing.T) {
	fake := &fakeTasksSvc{resolveResp: &pb.ResolveTaskResponse{TaskId: "", Resolved: false}}
	cli := tasks.NewWithClient(fake)

	res, err := cli.ResolveForValidation(context.Background(), tasks.ResolveParams{
		Key: tasks.Key{RuleID: "rule-1", EntityType: "load", EntityID: uuid.New().String(), ValidationInstance: "missing_rate_con"},
	})
	require.NoError(t, err)
	require.False(t, res.Resolved)
	require.Empty(t, res.TaskID)
}

// Edge: tasks service down → clear wrapped error, no panic.
func TestTasksClientCreateWrapsTransportError(t *testing.T) {
	fake := &fakeTasksSvc{createErr: status.Error(codes.Unavailable, "connection refused")}
	cli := tasks.NewWithClient(fake)

	_, err := cli.CreateForValidation(context.Background(), validCreate())
	require.Error(t, err)
	require.Contains(t, err.Error(), "tasks: create task")
	require.Contains(t, err.Error(), "connection refused")
}

// Edge: an incomplete DedupKey (empty rule_id / validation_instance) is refused
// BEFORE any network call.
func TestTasksClientRefusesIncompleteKeyBeforeNetwork(t *testing.T) {
	fake := &fakeTasksSvc{createResp: &pb.CreateTaskResponse{}}
	cli := tasks.NewWithClient(fake)

	for _, tc := range []struct {
		name  string
		mut   func(*tasks.CreateParams)
		field string
	}{
		{"empty rule_id", func(p *tasks.CreateParams) { p.Key.RuleID = "" }, "rule_id"},
		{"empty validation_instance", func(p *tasks.CreateParams) { p.Key.ValidationInstance = "" }, "validation_instance"},
		{"empty entity_id", func(p *tasks.CreateParams) { p.Key.EntityID = "" }, "entity_id"},
	} {
		p := validCreate()
		tc.mut(&p)
		_, err := cli.CreateForValidation(context.Background(), p)
		require.Error(t, err, tc.name)
		require.Contains(t, err.Error(), tc.field)
	}
	require.Zero(t, fake.createCalls, "no network call on a bad key")
}

// Edge: an invalid UUID in the entity or assignee is validated client-side,
// before the network.
func TestTasksClientValidatesUUIDsBeforeNetwork(t *testing.T) {
	fake := &fakeTasksSvc{createResp: &pb.CreateTaskResponse{}}
	cli := tasks.NewWithClient(fake)

	bad := validCreate()
	bad.Key.EntityID = "not-a-uuid"
	_, err := cli.CreateForValidation(context.Background(), bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "entity_id")

	bad2 := validCreate()
	bad2.AssigneeID = "nope"
	_, err = cli.CreateForValidation(context.Background(), bad2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "assignee_id")

	require.Zero(t, fake.createCalls, "no network call on a bad UUID")
}

// recordingServer captures the incoming metadata of the last CreateTask call.
type recordingServer struct {
	pb.UnimplementedTasksServiceServer
	md metadata.MD
}

func (s *recordingServer) CreateTask(ctx context.Context, _ *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	s.md, _ = metadata.FromIncomingContext(ctx)
	return &pb.CreateTaskResponse{TaskId: "t-1", Status: "open", Created: true}, nil
}

// AC4: the write is scoped to the ACTOR's company — the client attaches the
// system actor's company in x-company-id, so a wrong company context cannot
// create a task the server would attribute to another company. Tested over a
// real in-memory gRPC call so the metadata is real.
func TestTasksClientPropagatesActorCompany(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	rec := &recordingServer{}
	pb.RegisterTasksServiceServer(srv, rec)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(middleware.AuthClientInterceptor("internal-token")),
	)
	require.NoError(t, err)
	defer conn.Close()
	cli := tasks.NewFromConn(conn)

	companyA := uuid.New()
	actorA := &consts.Actor{ID: uuid.New(), IsSystem: true, Claims: &consts.UserClaims{CompanyID: &companyA}}
	_, err = cli.CreateForValidation(middleware.WithActor(context.Background(), actorA), validCreate())
	require.NoError(t, err)
	require.Equal(t, companyA.String(), lastMD(rec.md, "x-company-id"), "company A scoped")
	require.Equal(t, "internal-token", lastMD(rec.md, "x-internal-token"))

	// A different company context sends a different x-company-id — the server
	// scopes to whichever company the actor carries, never a fixed one.
	companyB := uuid.New()
	actorB := &consts.Actor{ID: uuid.New(), IsSystem: true, Claims: &consts.UserClaims{CompanyID: &companyB}}
	_, err = cli.CreateForValidation(middleware.WithActor(context.Background(), actorB), validCreate())
	require.NoError(t, err)
	require.Equal(t, companyB.String(), lastMD(rec.md, "x-company-id"), "company B scoped")
}

func lastMD(md metadata.MD, key string) string {
	v := md.Get(key)
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}
