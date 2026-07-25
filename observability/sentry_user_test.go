package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHubForTest returns a MockTransport-backed hub whose captured events can be
// inspected — the request hub that sentrygin would otherwise create per request.
func newHubForTest(t *testing.T) (*sentry.Hub, *sentry.MockTransport) {
	t.Helper()
	mt := &sentry.MockTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{Transport: mt, SampleRate: 1.0})
	require.NoError(t, err)
	return sentry.NewHub(client, sentry.NewScope()), mt
}

// ginCtxWithHub builds a gin.Context whose request context carries actor and
// whose per-request Sentry hub is hub — mirroring the state after GinMiddleware
// + IdentifyUser + the guest middleware have run.
func ginCtxWithHub(t *testing.T, hub *sentry.Hub, actor *consts.Actor) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/query", nil)
	if actor != nil {
		c.Request = c.Request.WithContext(middleware.WithActor(c.Request.Context(), actor))
	}
	sentrygin.SetHubOnContext(c, hub)
	return c
}

func enableSentryForTest(t *testing.T) {
	t.Helper()
	prev := enabled
	enabled = true
	t.Cleanup(func() { enabled = prev })
}

// AC #2: once the middleware has run, the calling user is on the per-request hub,
// so ANY later event on that hub (e.g. a panic recovered by sentrygin) carries
// the user id + actor_type — not just explicit CaptureWithCtx calls.
func TestSentryUserMiddleware_SetsUserOnRequestHub(t *testing.T) {
	enableSentryForTest(t)
	hub, mt := newHubForTest(t)
	companyID := uuid.New()
	actor := &consts.Actor{ID: uuid.New(), Claims: &consts.UserClaims{CompanyID: &companyID}}

	SentryUserMiddleware()(ginCtxWithHub(t, hub, actor))

	// Simulate an event emitted later in the request lifecycle on the same hub.
	hub.CaptureException(errors.New("kaboom"))

	require.Len(t, mt.Events(), 1)
	ev := mt.Events()[0]
	assert.Equal(t, actor.ID.String(), ev.User.ID)
	assert.Equal(t, "user", ev.Tags["actor_type"])
	assert.Equal(t, companyID.String(), ev.Tags["company_id"])
}

// Edge case: broker/guest share-link requests have a Nil user id. The middleware
// must tag actor_type + the shared resource and must NOT crash on the missing UUID.
func TestSentryUserMiddleware_GuestTagsShareResource(t *testing.T) {
	enableSentryForTest(t)
	hub, mt := newHubForTest(t)
	resourceID := uuid.New()
	actor := &consts.Actor{
		ID:      uuid.Nil,
		IsGuest: true,
		Claims:  &consts.UserClaims{Resource: "shipment", ResourceID: resourceID},
	}

	SentryUserMiddleware()(ginCtxWithHub(t, hub, actor))
	hub.CaptureException(errors.New("guest boom"))

	require.Len(t, mt.Events(), 1)
	ev := mt.Events()[0]
	assert.Equal(t, "guest", ev.Tags["actor_type"])
	assert.Equal(t, "shipment", ev.Tags["share_resource"])
	assert.Equal(t, resourceID.String(), ev.Tags["share_resource_id"])
	assert.Empty(t, ev.User.ID, "guest has a Nil uuid — no user id must be set")
}

// Edge case: no actor on the context (unauthenticated / pre-auth request) must
// not fail the set — it tags actor_type=system rather than panicking.
func TestSentryUserMiddleware_NoActorTagsSystem(t *testing.T) {
	enableSentryForTest(t)
	hub, mt := newHubForTest(t)

	SentryUserMiddleware()(ginCtxWithHub(t, hub, nil))
	hub.CaptureException(errors.New("anon boom"))

	require.Len(t, mt.Events(), 1)
	ev := mt.Events()[0]
	assert.Equal(t, "system", ev.Tags["actor_type"])
	assert.Empty(t, ev.User.ID)
}

// Edge case: empty SENTRY_DSN → Sentry disabled → the middleware is a passthrough
// no-op and never touches the hub, consistent with the rest of the package.
func TestSentryUserMiddleware_DisabledIsNoop(t *testing.T) {
	prev := enabled
	enabled = false
	t.Cleanup(func() { enabled = prev })

	hub, mt := newHubForTest(t)
	actor := &consts.Actor{ID: uuid.New(), Claims: &consts.UserClaims{}}

	SentryUserMiddleware()(ginCtxWithHub(t, hub, actor))
	hub.CaptureException(errors.New("x"))

	require.Len(t, mt.Events(), 1)
	ev := mt.Events()[0]
	assert.Empty(t, ev.User.ID, "disabled middleware must not enrich the scope")
	assert.Empty(t, ev.Tags["actor_type"])
}

// A broker actor (Claims.ActorType == ActorBroker) must be tagged actor_type=broker
// on the capture path too (shared enrichment used by Kafka/background captures).
func TestSetActorOnScope_BrokerActor(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())
	actor := &consts.Actor{ID: uuid.New(), Claims: &consts.UserClaims{ActorType: consts.ActorBroker}}
	ctx = middleware.WithActor(ctx, actor)

	CaptureWithCtx(ctx, errors.New("broker boom"))

	require.Len(t, mt.Events(), 1)
	ev := mt.Events()[0]
	assert.Equal(t, "broker", ev.Tags["actor_type"])
	assert.Equal(t, actor.ID.String(), ev.User.ID)
}

// Edge case: a Kafka-consumer / background capture with no actor on the context
// must be tagged actor_type=system rather than left unattributed or failing.
func TestSetActorOnScope_NoActorViaCaptureIsSystem(t *testing.T) {
	ctx, mt := mockHubCtx(t, context.Background())

	CaptureWithCtx(ctx, errors.New("kafka boom"))

	require.Len(t, mt.Events(), 1)
	assert.Equal(t, "system", mt.Events()[0].Tags["actor_type"])
}
