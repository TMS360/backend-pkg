package response

import (
	"fmt"
	"net/http"
)

func NewNotFound(resource, id string) PublicError {
	return NewError(
		fmt.Sprintf("%s not found: %s", resource, id),
		fmt.Sprintf("%s not found", resource),
		http.StatusNotFound,
	)
}

func NewBadRequest(tech, user string) PublicError {
	return NewError(tech, user, http.StatusBadRequest)
}

func NewConflict(tech, user string) PublicError {
	return NewError(tech, user, http.StatusConflict)
}

// NewConflictWithExtensions is NewConflict with a structured payload attached.
// The map is forwarded verbatim to gqlErr.Extensions by the GraphQL presenter.
func NewConflictWithExtensions(tech, user string, ext map[string]any) PublicError {
	return NewErrorWithExtensions(tech, user, http.StatusConflict, ext)
}

// NewCodedConflict is NewConflictWithExtensions plus a stable string code that
// the GraphQL presenter puts in extensions.code. Prefer it over embedding a
// token in the technical message when the client needs to branch on the
// rejection kind (e.g. "INVOICE_TRANSITION_INVALID").
func NewCodedConflict(code, tech, user string, ext map[string]any) PublicError {
	return NewCodedError(code, tech, user, http.StatusConflict, ext)
}

// CodeForbidden is the one machine-readable code for "not allowed" (DEV-4882).
// The @hasPerm/@hasRole directives, the gRPC PermissionDenied mapping and
// NewForbidden all answer with it, so a client branches on a single code no
// matter where the refusal came from.
const CodeForbidden = "FORBIDDEN"

// NewForbidden answers 403 with extensions.code CodeForbidden. Before DEV-4882
// it carried no string code, so every service refusal shipped `"code": 0`.
// A refusal that needs its own code still uses NewCodedError directly.
func NewForbidden(tech, user string) PublicError {
	return NewCodedError(CodeForbidden, tech, user, http.StatusForbidden, nil)
}

func NewUnauthorized(tech, user string) PublicError {
	return NewError(tech, user, http.StatusUnauthorized)
}

func NewInternalError(tech string) PublicError {
	return NewError(tech, "Something went wrong. Please try again.", http.StatusInternalServerError)
}
