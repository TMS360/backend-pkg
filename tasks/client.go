// Package tasks is the shared client (DEV-1666) for the tasks producer gRPC API
// (CreateTask / ResolveTask, shipped by DEV-1035). Any backend service — the
// rule engine, load, messaging, compliance — opens and closes a rule-driven
// task through this one client instead of copying protobuf dial glue.
//
// Auth and tenancy travel exactly like every other internal service-to-service
// call: nothing goes in the request body. The caller puts an Actor in the
// context (a real user's Actor, or a system Actor for cron / Kafka / rule-engine
// work) and middleware.AuthClientInterceptor attaches the JWT — or, for a system
// Actor, x-internal-token + x-actor-id + x-company-id — to the outgoing
// metadata. The tasks service's AuthServerInterceptor reconstructs the actor and
// scopes the write to that company, so a wrong company context can never create
// a task another company can see.
//
// Config (same names as other internal hosts):
//
//	TASKS_GRPC_ADDR      host:port of the tasks service gRPC endpoint
//	GRPC_INTERNAL_TOKEN  shared internal token (system-actor calls)
//
// Example (a system worker opening a task for a failed validation):
//
//	cli, err := tasks.Dial(os.Getenv(tasks.EnvAddr), os.Getenv("GRPC_INTERNAL_TOKEN"))
//	if err != nil { return err }
//	defer cli.Close()
//	// ctx must carry a (system) Actor for auth to be attached.
//	res, err := cli.CreateForValidation(ctx, tasks.CreateParams{
//	    Key: tasks.Key{
//	        RuleID: ruleID, EntityType: "load", EntityID: loadID.String(),
//	        ValidationInstance: "missing_rate_con",
//	    },
//	    Title:        "Load is missing a rate confirmation",
//	    AssigneeType: "department", AssigneeID: dispatchDeptID.String(),
//	})
//	// res.Created is false when the task was already open (retry / duplicate).
package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/TMS360/backend-pkg/middleware"
	pb "github.com/TMS360/backend-pkg/proto/tasks"
)

// EnvAddr is the env var other services read for the tasks gRPC host, matching
// the TEAMS_GRPC_ADDR / FILES_GRPC_ADDR convention.
const EnvAddr = "TASKS_GRPC_ADDR"

// defaultTimeout bounds a single Create/Resolve call so a hung tasks service
// surfaces as a clear deadline error rather than blocking the caller forever.
const defaultTimeout = 5 * time.Second

// Client is the shared tasks producer client. Callers depend on this interface
// so they can substitute a fake in tests without a live tasks service.
type Client interface {
	// CreateForValidation opens the task for a rule-driven validation, or returns
	// the one already open for the same DedupKey. Idempotent: calling twice while
	// the validation is live yields one task (Result.Created tells which happened).
	CreateForValidation(ctx context.Context, p CreateParams) (CreateResult, error)
	// ResolveForValidation auto-resolves the open task for a validation that no
	// longer holds. Idempotent: a no-op (Resolved=false, no error) when nothing
	// is open.
	ResolveForValidation(ctx context.Context, p ResolveParams) (ResolveResult, error)
	// Close releases the underlying connection (a no-op when the client wraps a
	// caller-owned connection or a raw generated client).
	Close() error
}

// Key identifies the specific validation occurrence a task belongs to — the
// whole key together is what "the same problem" means (server-side dedup).
type Key struct {
	RuleID             string // the rule that fired
	EntityType         string // load | trip | driver | document | ...
	EntityID           string // the TMS entity the task concerns (UUID)
	ValidationInstance string // the specific check that triggered
}

// CreateParams is the input to CreateForValidation.
type CreateParams struct {
	Key          Key
	Title        string
	Description  string
	Priority     string // low|normal|high|urgent — empty = normal
	AssigneeType string // user|department|role|all
	AssigneeID   string // UUID; required unless AssigneeType == "all"
	DueAt        *time.Time
}

// CreateResult is the outcome of CreateForValidation.
type CreateResult struct {
	TaskID  string
	Status  string
	Created bool // true = new task; false = existing one returned (dup/retry)
}

// ResolveParams is the input to ResolveForValidation.
type ResolveParams struct {
	Key    Key
	Reason string // optional; recorded on the system status change
}

// ResolveResult is the outcome of ResolveForValidation.
type ResolveResult struct {
	TaskID   string
	Resolved bool // true = a task was auto-resolved; false = nothing was open
}

type client struct {
	conn    *grpc.ClientConn // nil when wrapping a caller-owned conn / raw client
	svc     pb.TasksServiceClient
	timeout time.Duration
}

// Option configures the client.
type Option func(*client)

// WithTimeout overrides the per-call deadline (<=0 keeps the default).
func WithTimeout(d time.Duration) Option {
	return func(c *client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// Dial connects to the tasks service and returns a ready client. internalToken
// is GRPC_INTERNAL_TOKEN — it is used only for system-actor calls; a call
// carrying a real user Actor sends that user's JWT instead. Internal traffic is
// plaintext (insecure), like every other internal gRPC client.
func Dial(addr, internalToken string, opts ...Option) (Client, error) {
	if addr == "" {
		return nil, errors.New("tasks: gRPC address is empty (set " + EnvAddr + ")")
	}
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(middleware.AuthClientInterceptor(internalToken)),
	)
	if err != nil {
		return nil, fmt.Errorf("tasks: connect %s: %w", addr, err)
	}
	c := &client{conn: conn, svc: pb.NewTasksServiceClient(conn), timeout: defaultTimeout}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// NewFromConn wraps a caller-owned connection (bufconn tests, or sharing one
// connection). Close does not close the borrowed connection.
func NewFromConn(conn *grpc.ClientConn, opts ...Option) Client {
	c := &client{svc: pb.NewTasksServiceClient(conn), timeout: defaultTimeout}
	for _, o := range opts {
		o(c)
	}
	return c
}

// NewWithClient wraps a raw generated client — the seam for unit tests, which
// pass a fake pb.TasksServiceClient and never touch the network.
func NewWithClient(svc pb.TasksServiceClient, opts ...Option) Client {
	c := &client{svc: svc, timeout: defaultTimeout}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *client) CreateForValidation(ctx context.Context, p CreateParams) (CreateResult, error) {
	if err := p.Key.validate(); err != nil {
		return CreateResult{}, err
	}
	if p.Title == "" {
		return CreateResult{}, errors.New("tasks: title is required")
	}
	// Assignee is required unless the task targets everyone.
	if p.AssigneeType != "" && p.AssigneeType != "all" {
		if p.AssigneeID == "" {
			return CreateResult{}, fmt.Errorf("tasks: assignee_id is required for assignee_type %q", p.AssigneeType)
		}
		if _, err := uuid.Parse(p.AssigneeID); err != nil {
			return CreateResult{}, fmt.Errorf("tasks: assignee_id %q is not a valid UUID", p.AssigneeID)
		}
	}

	req := &pb.CreateTaskRequest{
		Key:          p.Key.toProto(),
		Title:        p.Title,
		Description:  p.Description,
		Priority:     p.Priority,
		AssigneeType: p.AssigneeType,
		AssigneeId:   p.AssigneeID,
	}
	if p.DueAt != nil {
		req.DueAt = timestamppb.New(*p.DueAt)
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	resp, err := c.svc.CreateTask(ctx, req)
	if err != nil {
		return CreateResult{}, fmt.Errorf("tasks: create task: %w", err)
	}
	return CreateResult{TaskID: resp.GetTaskId(), Status: resp.GetStatus(), Created: resp.GetCreated()}, nil
}

func (c *client) ResolveForValidation(ctx context.Context, p ResolveParams) (ResolveResult, error) {
	if err := p.Key.validate(); err != nil {
		return ResolveResult{}, err
	}

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	resp, err := c.svc.ResolveTask(ctx, &pb.ResolveTaskRequest{Key: p.Key.toProto(), Reason: p.Reason})
	if err != nil {
		return ResolveResult{}, fmt.Errorf("tasks: resolve task: %w", err)
	}
	return ResolveResult{TaskID: resp.GetTaskId(), Resolved: resp.GetResolved()}, nil
}

func (c *client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.timeout)
}

// validate refuses an incomplete key BEFORE any network call: the whole DedupKey
// must be present (it is what the server dedups on) and the entity id must be a
// real UUID.
func (k Key) validate() error {
	switch {
	case k.RuleID == "":
		return errors.New("tasks: rule_id is required")
	case k.EntityType == "":
		return errors.New("tasks: entity_type is required")
	case k.EntityID == "":
		return errors.New("tasks: entity_id is required")
	case k.ValidationInstance == "":
		return errors.New("tasks: validation_instance is required")
	}
	if _, err := uuid.Parse(k.EntityID); err != nil {
		return fmt.Errorf("tasks: entity_id %q is not a valid UUID", k.EntityID)
	}
	return nil
}

func (k Key) toProto() *pb.DedupKey {
	return &pb.DedupKey{
		RuleId:             k.RuleID,
		EntityType:         k.EntityType,
		EntityId:           k.EntityID,
		ValidationInstance: k.ValidationInstance,
	}
}
