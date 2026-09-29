package notify

import (
	"context"
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/google/uuid"
)

type captureTM struct{ data map[string]interface{} }

func (c *captureTM) Publish(_ context.Context, _, _ string, _ uuid.UUID, data interface{}, _ ...interface{}) error {
	c.data = data.(map[string]interface{})
	return nil
}

func TestSend_CompanyID(t *testing.T) {
	actorCo, explicitCo := uuid.New(), uuid.New()
	withActor := middleware.WithActor(context.Background(), &consts.Actor{
		IsSystem: true, Claims: &consts.UserClaims{CompanyID: &actorCo},
	})
	cases := []struct {
		name string
		ctx  context.Context
		n    *uuid.UUID
		want string
	}{
		{"headless producer uses the explicit company", context.Background(), &explicitCo, explicitCo.String()},
		{"actor company wins over the explicit one", withActor, &explicitCo, actorCo.String()},
		{"nothing set stays empty", context.Background(), nil, ""},
	}
	for _, tc := range cases {
		tm := &captureTM{}
		if err := NewPublisher(tm).Send(tc.ctx, Notification{Title: "t", CompanyID: tc.n}); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := tm.data["company_id"]; got != tc.want {
			t.Fatalf("%s: company_id = %v, want %s", tc.name, got, tc.want)
		}
	}
}
