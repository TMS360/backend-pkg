package tmsgraphql

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/response"
	"github.com/google/uuid"
	"github.com/golang-jwt/jwt/v5"
)

func TestHasPermDirective_UnresolvedReturns503(t *testing.T) {
	uid := uuid.New()
	ctx := consts.WithActor(context.Background(), &consts.Actor{
		ID: uid,
		Claims: &consts.UserClaims{
			UserID: uid,
			Roles:  []string{enums.UserRoleAdmin.String()},
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: uid.String(),
			},
		},
	})
	ctx = context.WithValue(ctx, consts.PermsUnresolvedCtx, true)
	ctx = consts.WithUserPerms(ctx, []string{})

	_, err := HasPermDirective(ctx, nil, func(ctx context.Context) (interface{}, error) {
		t.Fatal("next must not run when perms unresolved")
		return nil, nil
	}, []string{"accounting.invoices.view"})
	if err == nil {
		t.Fatal("expected error")
	}
	var pe response.PublicError
	if !errors.As(err, &pe) {
		t.Fatalf("want PublicError, got %T %v", err, err)
	}
	if pe.ErrorStatus() != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", pe.ErrorStatus())
	}
	if pe.UserMessage() != "permission service unavailable, please retry" {
		t.Fatalf("user message = %q", pe.UserMessage())
	}
	var coded response.CodedError
	if !errors.As(err, &coded) || coded.ErrorCodeString() != "PERMS_UNRESOLVED" {
		t.Fatalf("code = %v", err)
	}
}

func TestHasPermDirective_RealMissingStillForbidden(t *testing.T) {
	uid := uuid.New()
	ctx := consts.WithActor(context.Background(), &consts.Actor{
		ID: uid,
		Claims: &consts.UserClaims{
			UserID: uid,
			Roles:  []string{enums.UserRoleAdmin.String()},
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: uid.String(),
			},
		},
	})
	ctx = consts.WithUserPerms(ctx, []string{}) // resolved empty — real deny

	_, err := HasPermDirective(ctx, nil, func(ctx context.Context) (interface{}, error) {
		t.Fatal("next must not run")
		return nil, nil
	}, []string{"accounting.invoices.view"})
	if err == nil {
		t.Fatal("expected error")
	}
	var pe response.PublicError
	if !errors.As(err, &pe) {
		t.Fatalf("want PublicError, got %T", err)
	}
	if pe.ErrorStatus() != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", pe.ErrorStatus())
	}
	if pe.UserMessage() != "access denied: missing permission" {
		t.Fatalf("user message = %q", pe.UserMessage())
	}
}
