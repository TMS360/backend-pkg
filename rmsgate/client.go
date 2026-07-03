package rmsgate

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	pb "github.com/TMS360/backend-pkg/proto/rmsgate"
)

// Decision — вердикт гейта для доменного сервиса.
type Decision struct {
	Allow         bool
	Reasons       []string
	RequiredSteps []Step // уже распарсенные (типизированный intent-write контракт)
	// FailedOpen=true означает: решение НЕ от RMS — применён FailMode процесса
	// (RMS недоступен/таймаут/circuit). Для логов и метрик вызывающего кода.
	FailedOpen bool
}

const (
	defaultTimeout = 150 * time.Millisecond // fact-only SLO гейта

	// circuit-ish: после circuitThreshold подряд транспортных ошибок клиент
	// circuitCooldown не ходит в RMS вовсе (сразу FailMode) — не копим latency
	// на каждом вызове, пока RMS лежит.
	circuitThreshold = 5
	circuitCooldown  = 2 * time.Second
)

type options struct {
	timeout time.Duration
	reg     *Registry
	log     *slog.Logger
	dialOpt []grpc.DialOption
}

type Option func(*options)

// WithTimeout меняет дефолтный дедлайн Evaluate/Decide (per-process Timeout из
// реестра имеет приоритет).
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// WithFailModes задаёт реестр процессов (FailMode + per-process timeout).
func WithFailModes(r *Registry) Option { return func(o *options) { o.reg = r } }

func WithLogger(l *slog.Logger) Option { return func(o *options) { o.log = l } }

// WithDialOptions добавляет опции grpc.NewClient (TLS и т.п.; дефолт — insecure,
// как принято во внутренней сети платформы).
func WithDialOptions(opts ...grpc.DialOption) Option {
	return func(o *options) { o.dialOpt = append(o.dialOpt, opts...) }
}

// Client — канонический клиент RmsGate для доменных сервисов.
type Client struct {
	conn *grpc.ClientConn
	gate pb.RmsGateClient
	opt  options

	failures  atomic.Int32 // подряд идущие транспортные ошибки
	openUntil atomic.Int64 // unix-nano, до которого circuit открыт
}

// Dial подключается к RmsGate (например ":7080" RMS).
func Dial(addr string, opts ...Option) (*Client, error) {
	o := options{timeout: defaultTimeout, log: slog.Default()}
	for _, fn := range opts {
		fn(&o)
	}
	dialOpts := append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, o.dialOpt...)
	conn, err := grpc.NewClient(addr, dialOpts...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, gate: pb.NewRmsGateClient(conn), opt: o}, nil
}

// NewFromConn — клиент поверх готового соединения (тесты/bufconn).
func NewFromConn(conn *grpc.ClientConn, opts ...Option) *Client {
	o := options{timeout: defaultTimeout, log: slog.Default()}
	for _, fn := range opts {
		fn(&o)
	}
	return &Client{conn: conn, gate: pb.NewRmsGateClient(conn), opt: o}
}

func (c *Client) Close() error { return c.conn.Close() }

// Evaluate — сырой вызов гейта: (Decision, error). error — транспортный сбой;
// решает, что с ним делать, вызывающий. Большинству сервисов нужен Decide.
func (c *Client) Evaluate(ctx context.Context, process, transition, tenant string, facts map[string]any) (Decision, error) {
	st, err := structpb.NewStruct(facts)
	if err != nil {
		return Decision{}, err
	}
	// Идентичность тенанта — и полем, и метадатой (identityFromMD на стороне RMS).
	ctx = metadata.AppendToOutgoingContext(ctx, "x-tenant-id", tenant)
	resp, err := c.gate.Evaluate(ctx, &pb.EvaluateRequest{
		Process: process, Transition: transition, Tenant: tenant, Facts: st,
	})
	if err != nil {
		return Decision{}, err
	}
	return Decision{
		Allow:         resp.GetAllow(),
		Reasons:       resp.GetReasons(),
		RequiredSteps: ParseSteps(resp.GetRequiredSteps()),
	}, nil
}

// Decide — канонический вход для сервисов: навешивает per-process deadline,
// а на ЛЮБУЮ ошибку (timeout/сеть/circuit) возвращает вердикт по FailMode
// процесса с reason-объяснением. Никогда не возвращает error — enforce в
// вызывающем коде всегда однозначен:
//
//	dec := gate.Decide(ctx, "shipment", "NEW->DISPATCHED", companyID, facts)
//	if !dec.Allow { return dec.Reasons }
//	applySteps(dec.RequiredSteps) // идемпотентно, в своей транзакции
func (c *Client) Decide(ctx context.Context, process, transition, tenant string, facts map[string]any) Decision {
	if until := c.openUntil.Load(); until > 0 && time.Now().UnixNano() < until {
		return c.failDecision(process, "gate circuit open")
	}

	timeout := c.opt.reg.Timeout(process)
	if timeout <= 0 {
		timeout = c.opt.timeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dec, err := c.Evaluate(cctx, process, transition, tenant, facts)
	if err != nil {
		n := c.failures.Add(1)
		if n >= circuitThreshold {
			c.openUntil.Store(time.Now().Add(circuitCooldown).UnixNano())
			c.failures.Store(0)
		}
		c.opt.log.Warn("rmsgate: evaluate failed, применяю FailMode процесса",
			"process", process, "transition", transition, "mode", c.opt.reg.Mode(process).String(), "err", err)
		return c.failDecision(process, "gate unavailable: "+err.Error())
	}
	c.failures.Store(0)
	return dec
}

// failDecision — вердикт по FailMode процесса при недоступном гейте.
func (c *Client) failDecision(process, reason string) Decision {
	mode := c.opt.reg.Mode(process)
	return Decision{
		Allow:      mode == FailOpen,
		Reasons:    []string{"rmsgate: " + reason + " (" + mode.String() + ")"},
		FailedOpen: true,
	}
}
