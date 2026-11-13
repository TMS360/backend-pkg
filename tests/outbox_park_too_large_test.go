package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/eventlog/outbox"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/tmsdb"
	"github.com/TMS360/backend-pkg/tmsdb/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	kafkaGo "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DEV-3198. One week-lock event of 1.4 MB failed its whole topic's
// WriteMessages with "[10] Message Size Too Large". Nothing was deleted, so the
// relay retried it every 500ms forever: Sentry flooded, the 40 small week-lock
// events behind it never reached accounting, and once more than 50 such rows
// piled up the fetch window held only stuck rows and every event of the service
// stopped. These tests pin parking: a too-large event is put aside once, with
// one alert, and everything else keeps flowing.

// limitWriter behaves like kafka.Writer's own size check: any message over
// limit fails the whole call with MessageTooLargeError and nothing is written.
// Topics in down fail with a passing error.
type limitWriter struct {
	limit   int
	down    map[string]error
	written []kafkaGo.Message
	calls   int
}

func (w *limitWriter) WriteMessages(_ context.Context, msgs ...kafkaGo.Message) error {
	w.calls++
	for i, m := range msgs {
		if len(m.Value) > w.limit {
			rest := append(append([]kafkaGo.Message{}, msgs[:i]...), msgs[i+1:]...)
			return kafkaGo.MessageTooLargeError{Message: m, Remaining: rest}
		}
	}
	if len(msgs) > 0 {
		if err := w.down[msgs[0].Topic]; err != nil {
			return err
		}
	}
	w.written = append(w.written, msgs...)
	return nil
}

func sized(topic string, n int) kafkaGo.Message {
	return kafkaGo.Message{Topic: topic, Key: []byte(uuid.NewString()), Value: bytes.Repeat([]byte("x"), n)}
}

// --- relay fakes -----------------------------------------------------------

// fakeTM runs the transaction body inline; nothing else is used by the relay.
type fakeTM struct{ tmsdb.TransactionManager }

func (fakeTM) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// memRepo is an in-memory outbox table that honours the PENDING filter and
// created_at order of the real FetchPendingBatch.
type memRepo struct {
	rows []*model.OutboxEvent
}

func (r *memRepo) FetchPendingBatch(_ context.Context, limit int) ([]*model.OutboxEvent, error) {
	var out []*model.OutboxEvent
	for _, e := range r.rows {
		if e.Status == outbox.StatusPending && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *memRepo) DeleteBatch(_ context.Context, ids []string) error {
	drop := map[string]bool{}
	for _, id := range ids {
		drop[id] = true
	}
	kept := r.rows[:0]
	for _, e := range r.rows {
		if !drop[e.ID.String()] {
			kept = append(kept, e)
		}
	}
	r.rows = kept
	return nil
}

func (r *memRepo) MarkParked(_ context.Context, ids []string, status string) error {
	for _, id := range ids {
		for _, e := range r.rows {
			if e.ID.String() == id && e.Status == outbox.StatusPending {
				e.Status = status
			}
		}
	}
	return nil
}

func (r *memRepo) add(topic string, size int, at time.Time) *model.OutboxEvent {
	e := &model.OutboxEvent{
		ID: uuid.New(), EntityID: uuid.New(), EntityType: topic, EventType: "UPDATED",
		Payload: model.JSONRaw(bytes.Repeat([]byte("x"), size)), Status: outbox.StatusPending, CreatedAt: at,
	}
	r.rows = append(r.rows, e)
	return e
}

func (r *memRepo) byStatus(status string) int {
	n := 0
	for _, e := range r.rows {
		if e.Status == status {
			n++
		}
	}
	return n
}

type alertSpy struct{ errs []error }

func (a *alertSpy) capture(_ context.Context, err error) { a.errs = append(a.errs, err) }

// --- AC1: too-big event is put aside, the rest of its topic goes out ---------

func TestOutboxPark_TooLargeIsParkedAndTheRestOfTheTopicIsSentInTheSameCall(t *testing.T) {
	w := &limitWriter{limit: 1000}
	msgs := []kafkaGo.Message{sized("audit_week_locks", 10), sized("audit_week_locks", 5000), sized("audit_week_locks", 20)}

	res := outbox.PublishByTopic(context.Background(), w, msgs)

	assert.Empty(t, res.Failed, "a too-large event must not fail its topic")
	assert.Equal(t, []int{1}, res.TooLarge)
	require.Len(t, w.written, 2)
	assert.Equal(t, 10, len(w.written[0].Value))
	assert.Equal(t, 20, len(w.written[1].Value), "order of the others is kept")
}

func TestOutboxPark_WeekLockScenario_BigRowParkedSmallRowsDeleted(t *testing.T) {
	repo := &memRepo{}
	t0 := time.Now()
	big := repo.add("audit_week_locks", 1_400_000, t0)
	for i := 1; i <= 40; i++ {
		repo.add("audit_week_locks", 2_000, t0.Add(time.Duration(i)*time.Millisecond))
	}
	w := &limitWriter{limit: 1_048_576}
	alerts := &alertSpy{}
	relay := outbox.NewRelayWithRepository(fakeTM{}, repo, w, alerts.capture)

	require.NoError(t, relay.ProcessBatch(context.Background(), 50))

	assert.Equal(t, outbox.StatusParkedTooLarge, big.Status, "the big row stays in the table, put aside")
	require.Len(t, repo.rows, 1, "the 40 small week-lock events left the outbox")
	assert.Len(t, w.written, 40)
}

// --- AC2: one alert per parked event, never again on later ticks -------------

func TestOutboxPark_OneAlertNamingTopicIDAndSize_NotRepeatedOnNextTicks(t *testing.T) {
	repo := &memRepo{}
	big := repo.add("audit_week_locks", 1_400_000, time.Now())
	w := &limitWriter{limit: 1_048_576}
	alerts := &alertSpy{}
	relay := outbox.NewRelayWithRepository(fakeTM{}, repo, w, alerts.capture)

	for tick := 0; tick < 5; tick++ {
		require.NoError(t, relay.ProcessBatch(context.Background(), 50))
	}

	require.Len(t, alerts.errs, 1)
	msg := alerts.errs[0].Error()
	assert.Contains(t, msg, "topic=audit_week_locks")
	assert.Contains(t, msg, "event_id="+big.ID.String())
	assert.Contains(t, msg, "size_bytes=1400000")
	assert.Equal(t, 1, w.calls, "a parked row is never sent again")
}

func TestOutboxPark_TwoTooLargeInOneSend_BothParkedOneAlertEach(t *testing.T) {
	repo := &memRepo{}
	t0 := time.Now()
	a := repo.add("audit_week_locks", 2_000_000, t0)
	repo.add("audit_week_locks", 100, t0.Add(time.Millisecond))
	b := repo.add("trips", 3_000_000, t0.Add(2*time.Millisecond))
	alerts := &alertSpy{}
	relay := outbox.NewRelayWithRepository(fakeTM{}, repo, &limitWriter{limit: 1_048_576}, alerts.capture)

	require.NoError(t, relay.ProcessBatch(context.Background(), 50))

	assert.Equal(t, outbox.StatusParkedTooLarge, a.Status)
	assert.Equal(t, outbox.StatusParkedTooLarge, b.Status)
	assert.Len(t, alerts.errs, 2)
	assert.Equal(t, 2, len(repo.rows))
}

// --- AC3: parked rows never fill the "next 50" window ------------------------

func TestOutboxPark_SixtyParkedRowsDoNotBlockANewTripEvent(t *testing.T) {
	repo := &memRepo{}
	t0 := time.Now()
	for i := 0; i < 60; i++ {
		repo.add("audit_week_locks", 1_400_000, t0.Add(time.Duration(i)*time.Millisecond))
	}
	w := &limitWriter{limit: 1_048_576}
	relay := outbox.NewRelayWithRepository(fakeTM{}, repo, w, (&alertSpy{}).capture)

	// Two ticks park all 60 (50 + 10).
	require.NoError(t, relay.ProcessBatch(context.Background(), 50))
	require.NoError(t, relay.ProcessBatch(context.Background(), 50))
	require.Equal(t, 60, repo.byStatus(outbox.StatusParkedTooLarge))

	trip := repo.add("trips", 500, t0.Add(time.Second))
	require.NoError(t, relay.ProcessBatch(context.Background(), 50))

	require.Len(t, w.written, 1)
	assert.Equal(t, trip.EntityID.String(), string(w.written[0].Key), "the trip event reached Kafka on the next tick")
	assert.Equal(t, 0, repo.byStatus(outbox.StatusPending))
}

// The real repository is what keeps parked rows out of the window: its SQL.
func TestOutboxPark_RepositorySQL_FetchOnlyPendingAndParkOnlyPending(t *testing.T) {
	sqls := &sqlCapture{}
	db, err := gorm.Open(
		postgres.New(postgres.Config{
			DSN:                  "host=127.0.0.1 port=1 user=dryrun dbname=dryrun sslmode=disable",
			PreferSimpleProtocol: true,
		}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: sqls},
	)
	require.NoError(t, err)
	tm := tmsdb.NewGormTransactionManager(db, "pkg-test")
	repo := outbox.NewOutboxEventRepository(tm)
	parked := outbox.NewParkedRepository(tm)
	ctx := context.Background()

	_, err = repo.FetchPendingBatch(ctx, 50)
	require.NoError(t, err)
	require.NoError(t, repo.MarkParked(ctx, []string{uuid.NewString()}, outbox.StatusParkedTooLarge))
	_, err = parked.Requeue(ctx, uuid.NewString())
	require.NoError(t, err)

	require.Len(t, sqls.sql, 3)
	assert.Contains(t, sqls.sql[0], "status = 'PENDING'")
	assert.Contains(t, sqls.sql[0], "LIMIT 50")
	assert.Contains(t, sqls.sql[1], `"status"='PARKED_TOO_LARGE'`)
	assert.Contains(t, sqls.sql[1], "status = 'PENDING'", "only a pending row can be parked")
	assert.Contains(t, sqls.sql[2], `"status"='PENDING'`)
	assert.Contains(t, sqls.sql[2], "status = 'PARKED_TOO_LARGE'", "only a parked row can be re-queued")
}

type sqlCapture struct{ sql []string }

func (s *sqlCapture) LogMode(logger.LogLevel) logger.Interface      { return s }
func (s *sqlCapture) Info(context.Context, string, ...interface{})  {}
func (s *sqlCapture) Warn(context.Context, string, ...interface{})  {}
func (s *sqlCapture) Error(context.Context, string, ...interface{}) {}
func (s *sqlCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	s.sql = append(s.sql, sql)
}

// --- Edge: Kafka down / topic missing is NOT parked --------------------------

func TestOutboxPark_KafkaDownIsRetriedNotParked(t *testing.T) {
	repo := &memRepo{}
	e := repo.add("trips", 100, time.Now())
	w := &limitWriter{limit: 1_048_576, down: map[string]error{"trips": kafkaGo.UnknownTopicOrPartition}}
	alerts := &alertSpy{}
	relay := outbox.NewRelayWithRepository(fakeTM{}, repo, w, alerts.capture)

	require.NoError(t, relay.ProcessBatch(context.Background(), 50))

	assert.Equal(t, outbox.StatusPending, e.Status)
	assert.Len(t, repo.rows, 1)
	assert.Empty(t, alerts.errs, "the park alert is only for too-large events")
}

// Edge: the limit is the writer's own BatchBytes, not a number in the relay.
// A real kafka.Writer, pointed at a closed port: the too-large message is
// refused by the writer before any network, the small one fails on dial and is
// therefore retried, not parked.
func TestOutboxPark_LimitComesFromTheRealWriterBatchBytes(t *testing.T) {
	w := &kafkaGo.Writer{Addr: kafkaGo.TCP("127.0.0.1:1"), BatchBytes: 256, MaxAttempts: 1}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res := outbox.PublishByTopic(ctx, w, []kafkaGo.Message{sized("trips", 10), sized("trips", 1024)})

	assert.Equal(t, []int{1}, res.TooLarge)
	require.Contains(t, res.Failed, "trips", "the small one hit the dead broker: retried")
	var tl kafkaGo.MessageTooLargeError
	assert.False(t, errors.As(res.Failed["trips"], &tl))
}

// Edge: broker-side refusal (WriteErrors with code 10) parks only that message.
type brokerRefusal struct{ refuse int }

func (b *brokerRefusal) WriteMessages(_ context.Context, msgs ...kafkaGo.Message) error {
	we := make(kafkaGo.WriteErrors, len(msgs))
	for i, m := range msgs {
		if len(m.Value) > b.refuse {
			we[i] = fmt.Errorf("%w: broker limit", kafkaGo.MessageSizeTooLarge)
		}
	}
	if we.Count() == 0 {
		return nil
	}
	return we
}

func TestOutboxPark_BrokerSideMessageSizeTooLargeParksOnlyThatMessage(t *testing.T) {
	res := outbox.PublishByTopic(context.Background(), &brokerRefusal{refuse: 100},
		[]kafkaGo.Message{sized("trips", 10), sized("trips", 500), sized("trips", 20)})

	assert.Empty(t, res.Failed)
	assert.Equal(t, []int{1}, res.TooLarge)
}

// --- AC4: admin re-queues or deletes, logged with who and why ----------------

type parkedRepoFake struct {
	rows map[string]*model.OutboxEvent
}

func (f *parkedRepoFake) FindParked(_ context.Context, id string) (*model.OutboxEvent, error) {
	if e, ok := f.rows[id]; ok && e.Status == outbox.StatusParkedTooLarge {
		return e, nil
	}
	return nil, nil
}

func (f *parkedRepoFake) Requeue(_ context.Context, id string) (bool, error) {
	if e, ok := f.rows[id]; ok && e.Status == outbox.StatusParkedTooLarge {
		e.Status = outbox.StatusPending
		return true, nil
	}
	return false, nil
}

func (f *parkedRepoFake) DeleteParked(_ context.Context, id string) (bool, error) {
	if e, ok := f.rows[id]; ok && e.Status == outbox.StatusParkedTooLarge {
		delete(f.rows, id)
		return true, nil
	}
	return false, nil
}

func adminRouter(t *testing.T, repo outbox.ParkedRepository, roles ...string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	actorID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	r.Use(func(c *gin.Context) {
		actor := &consts.Actor{ID: actorID, Claims: &consts.UserClaims{UserID: actorID, Roles: roles}}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), consts.ActorCtx, actor))
		c.Next()
	})
	g := r.Group("/internal/admin").Use(middleware.RequireAdmin())
	outbox.NewAdminController(outbox.NewAdminService(repo)).Register(g)
	return r
}

func post(r http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func parkedFixture() (*parkedRepoFake, string) {
	e := &model.OutboxEvent{ID: uuid.New(), EntityID: uuid.New(), EntityType: "audit_week_locks",
		Payload: model.JSONRaw(`{}`), Status: outbox.StatusParkedTooLarge}
	return &parkedRepoFake{rows: map[string]*model.OutboxEvent{e.ID.String(): e}}, e.ID.String()
}

func TestOutboxAdmin_RequeueByAdmin_PendingAgainAndLoggedWithWhoAndWhy(t *testing.T) {
	logs := captureLogs(t)
	repo, id := parkedFixture()

	rec := post(adminRouter(t, repo, "admin"), "/internal/admin/outbox/parked/"+id+"/requeue", `{"reason":"producer fixed in DEV-3199"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, outbox.StatusPending, repo.rows[id].Status)
	var line map[string]any
	require.NoError(t, json.Unmarshal(lastLogLine(logs), &line))
	assert.Equal(t, "outbox admin action on parked event", line["msg"])
	assert.Equal(t, "requeue", line["action"])
	assert.Equal(t, id, line["event_id"])
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", line["actor_id"])
	assert.Equal(t, "producer fixed in DEV-3199", line["reason"])
}

func TestOutboxAdmin_DeleteByAdmin_RowGoneAndLogged(t *testing.T) {
	logs := captureLogs(t)
	repo, id := parkedFixture()

	rec := post(adminRouter(t, repo, "super_admin"), "/internal/admin/outbox/parked/"+id+"/delete", `{"reason":"test row"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, repo.rows, id)
	assert.Contains(t, logs.String(), `"action":"delete"`)
	assert.Contains(t, logs.String(), `"reason":"test row"`)
}

func TestOutboxAdmin_NonAdminRefused_RowUntouched(t *testing.T) {
	repo, id := parkedFixture()

	rec := post(adminRouter(t, repo, "dispatcher"), "/internal/admin/outbox/parked/"+id+"/delete", `{"reason":"x"}`)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, outbox.StatusParkedTooLarge, repo.rows[id].Status)
}

func TestOutboxAdmin_ReasonRequired_AndOnlyParkedRows(t *testing.T) {
	repo, id := parkedFixture()
	r := adminRouter(t, repo, "admin")

	assert.Equal(t, http.StatusBadRequest, post(r, "/internal/admin/outbox/parked/"+id+"/requeue", `{"reason":"  "}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(r, "/internal/admin/outbox/parked/not-a-uuid/requeue", `{"reason":"x"}`).Code)
	assert.Equal(t, http.StatusNotFound, post(r, "/internal/admin/outbox/parked/"+uuid.NewString()+"/requeue", `{"reason":"x"}`).Code)
	assert.Equal(t, outbox.StatusParkedTooLarge, repo.rows[id].Status, "refused calls change nothing")

	require.Equal(t, http.StatusOK, post(r, "/internal/admin/outbox/parked/"+id+"/requeue", `{"reason":"x"}`).Code)
	assert.Equal(t, http.StatusNotFound, post(r, "/internal/admin/outbox/parked/"+id+"/delete", `{"reason":"x"}`).Code,
		"a re-queued row is PENDING again, not parked: the admin cannot delete a live event")
}

func lastLogLine(buf *bytes.Buffer) []byte {
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	return lines[len(lines)-1]
}
