package tmsgraphql

import (
	"context"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/TMS360/backend-pkg/consts"
	"github.com/TMS360/backend-pkg/response"
)

// CodeReadOnlyToken / MsgReadOnlyToken are what a caller with a read-only token
// gets on a mutation or a subscription (extensions.code, status 403).
const (
	CodeReadOnlyToken = "READ_ONLY_TOKEN"
	MsgReadOnlyToken  = "this token is read-only: only queries are allowed"
)

// ReadOnlyGuard rejects every mutation and subscription made with a read-only
// token (UserClaims.ReadOnly), before any resolver runs. Only queries pass.
//
// The RMS service identity (DEV-3270) reads the gateway for one company and
// must never write. Its permissions are view-only, but that relies on every
// subgraph checking a permission on every mutation; one mutation behind only
// @auth would accept the token. This guard does not depend on that.
func ReadOnlyGuard() graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc == nil || oc.Operation == nil || oc.Operation.Operation == ast.Query {
			return next(ctx)
		}
		actor, err := consts.GetActor(ctx)
		if err != nil || !actor.IsReadOnly() {
			return next(ctx)
		}
		// The response context (and its presenter) is created after operation
		// middleware, so the shared presenter is called here directly: the
		// error has the same shape as every other coded error.
		err = response.NewCodedError(CodeReadOnlyToken, MsgReadOnlyToken, MsgReadOnlyToken, http.StatusForbidden, nil)
		return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{NewErrorPresenter(false)(ctx, err)}})
	}
}
