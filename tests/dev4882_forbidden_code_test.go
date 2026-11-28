package tests

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/TMS360/backend-pkg/client/tmsgraphql"
	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/response"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DEV-4882 — a refusal built with response.NewForbidden answered with
// extensions.code 0, while the @hasRole directive answered "FORBIDDEN". The
// same "not allowed" now carries one code, whichever layer refused.

func present4882(t *testing.T, err error) (code any, httpStatus any, message string) {
	t.Helper()
	_ = captureSlog(t)
	gqlErr := tmsgraphql.NewErrorPresenter(false)(context.Background(), err)
	require.NotNil(t, gqlErr)
	return gqlErr.Extensions["code"], gqlErr.Extensions["status"], gqlErr.Message
}

func adminWithPerms4882(perms ...string) context.Context {
	uid := uuid.New()
	ctx := consts.WithActor(context.Background(), &consts.Actor{
		ID: uid,
		Claims: &consts.UserClaims{
			UserID:           uid,
			Roles:            []string{enums.UserRoleAdmin.String()},
			RegisteredClaims: jwt.RegisteredClaims{Subject: uid.String()},
		},
	})
	return consts.WithUserPerms(ctx, perms)
}

// AC1: a service-code refusal (e.g. a driver on linkMessageToChat) answers
// 403, code FORBIDDEN, and the same message as before.
func TestDEV4882_ServiceRefusal_AnswersForbiddenCode(t *testing.T) {
	code, st, msg := present4882(t, response.NewForbidden("driver may not link messages", "Access denied"))

	assert.Equal(t, response.CodeForbidden, code)
	assert.Equal(t, "FORBIDDEN", code)
	assert.Equal(t, http.StatusForbidden, st)
	assert.Equal(t, "Access denied", msg)
}

// AC1: the code survives the wrapping a service does before returning.
func TestDEV4882_WrappedRefusal_KeepsForbiddenCode(t *testing.T) {
	cases := map[string]error{
		"fmt %w":     fmt.Errorf("link message: %w", response.NewForbidden("tech", "Access denied")),
		"unreported": response.Unreported(response.NewForbidden("tech", "Access denied")),
	}
	for name, err := range cases {
		t.Run(name, func(t *testing.T) {
			code, st, msg := present4882(t, err)
			assert.Equal(t, "FORBIDDEN", code)
			assert.Equal(t, http.StatusForbidden, st)
			assert.Equal(t, "Access denied", msg)
		})
	}
}

// AC2: a @hasPerm refusal and a service-code refusal carry the same code.
func TestDEV4882_PermRuleAndServiceRefusal_SameCode(t *testing.T) {
	_, permErr := tmsgraphql.HasPermDirective(adminWithPerms4882("loads.view"), nil,
		func(ctx context.Context) (any, error) {
			require.FailNow(t, "resolver ran without the permission")
			return nil, nil
		}, []string{"accounting.invoices.view"})
	require.Error(t, permErr)

	permCode, permStatus, _ := present4882(t, permErr)
	svcCode, svcStatus, _ := present4882(t, response.NewForbidden("tech", "Access denied"))

	assert.Equal(t, "FORBIDDEN", permCode)
	assert.Equal(t, permCode, svcCode)
	assert.Equal(t, http.StatusForbidden, permStatus)
	assert.Equal(t, svcStatus, permStatus)
}

// AC2: the shared consts.ErrForbidden and the gRPC PermissionDenied mapping use
// the same single name.
func TestDEV4882_SharedForbiddenAndGRPC_SameCode(t *testing.T) {
	code, st, _ := present4882(t, consts.ErrForbidden)
	assert.Equal(t, "FORBIDDEN", code)
	assert.Equal(t, http.StatusForbidden, st)

	grpcCode, _, _ := present4882(t, status.Error(codes.PermissionDenied, "no"))
	assert.Equal(t, response.CodeForbidden, grpcCode)

	assert.Equal(t, response.CodeForbidden, tmsgraphql.AccessDeniedCode)
}

// Edge: a 403 that sets its own code keeps it.
func TestDEV4882_OwnCode_IsKept(t *testing.T) {
	code, st, _ := present4882(t,
		response.NewCodedError("COMPANY_INACTIVE", "tech", "user", http.StatusForbidden, nil))
	assert.Equal(t, "COMPANY_INACTIVE", code)
	assert.Equal(t, http.StatusForbidden, st)
}

// Edge: the not-signed-in answer is out of scope and keeps its old code.
func TestDEV4882_Unauthorized_Untouched(t *testing.T) {
	code, st, _ := present4882(t, response.NewUnauthorized("no token", "Unauthorized"))
	assert.Equal(t, 0, code)
	assert.Equal(t, http.StatusUnauthorized, st)
}
