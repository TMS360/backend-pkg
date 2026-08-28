package response

import (
	"fmt"
	"log/slog"
	"net/http"
)

type PublicError interface {
	Error() string
	UserMessage() string
	ErrorCode() int
	ErrorStatus() int
	// Extensions returns an optional structured payload that the GraphQL
	// presenter merges into gqlErr.Extensions alongside code/status. Callers
	// that don't need a payload get an empty map.
	Extensions() map[string]any
}

// CodedError is the optional string-code extension of PublicError.
//
// Why it exists: ErrorCode() is an int that no constructor here ever populates,
// so every PublicError ships extensions.code = 0 — useless to branch on. The
// payload map cannot fill the gap either: the GraphQL presenter reserves the
// "code" and "status" keys so a caller-supplied payload can never shadow the
// shared contract. Services that need a stable, machine-readable code (e.g.
// "INVOICE_TRANSITION_INVALID") were therefore reduced to embedding a token in
// the technical message and having the client substring-match it.
//
// When an error in the chain implements this interface AND returns a non-empty
// string, the presenter uses that string as extensions.code in place of the
// legacy int. An empty string falls back to the int, so implementing this
// interface is never a breaking change for an existing caller.
type CodedError interface {
	PublicError
	ErrorCodeString() string
}

type publicError struct {
	Technical string
	User      string
	Status    int
	Code      int
	// CodeStr is the machine-readable string code surfaced as extensions.code.
	// Empty means "no string code" — the presenter then keeps the legacy int.
	CodeStr string
	Ext     map[string]any
}

func (e *publicError) Error() string {
	return e.Technical
}

func (e *publicError) UserMessage() string {
	return e.User
}

func (e *publicError) ErrorStatus() int {
	return e.Status
}

func (e *publicError) ErrorCode() int {
	return e.Code
}

func (e *publicError) Extensions() map[string]any {
	return e.Ext
}

func (e *publicError) ErrorCodeString() string {
	return e.CodeStr
}

func NewError(tech, user string, status int) PublicError {
	logPublicError(status, fmt.Sprintf("[tech=%s,user=%s]", tech, user))
	return &publicError{Technical: tech, User: user, Status: status}
}

// logPublicError writes the construction line at the level the status deserves
// (DEV-1970). A 4xx is a normal answer to the caller — "you are not allowed",
// "that name is taken" — and logging it at ERROR made every routine denial
// look like a fault in the logs, next to the real ones. 5xx keeps ERROR.
// Mirrors the split the GraphQL presenter already makes when it reports to
// Sentry: 5xx captures as an error, 4xx as a warning.
func logPublicError(status int, msg string) {
	if status >= http.StatusInternalServerError || status == 0 {
		slog.Error(msg)
		return
	}
	slog.Warn(msg)
}

// NewErrorWithExtensions is the payload-carrying variant of NewError. The
// extensions map is passed through to the GraphQL presenter so a caller can
// attach structured details (e.g. a blocking resource's id) that clients read
// without parsing the human message. A nil or empty map behaves like NewError.
func NewErrorWithExtensions(tech, user string, status int, ext map[string]any) PublicError {
	logPublicError(status, fmt.Sprintf("[tech=%s,user=%s]", tech, user))
	return &publicError{Technical: tech, User: user, Status: status, Ext: ext}
}

// NewCodedError is NewErrorWithExtensions plus a stable machine-readable code
// that the GraphQL presenter surfaces as extensions.code. Use it when the
// client must branch on the failure kind rather than on the message text —
// messages are user-facing prose and change; codes are a contract.
//
// The returned error satisfies CodedError. A nil or empty map behaves like
// NewError; an empty code behaves like NewErrorWithExtensions.
func NewCodedError(code, tech, user string, status int, ext map[string]any) PublicError {
	logPublicError(status, fmt.Sprintf("[code=%s,tech=%s,user=%s]", code, tech, user))
	return &publicError{Technical: tech, User: user, Status: status, CodeStr: code, Ext: ext}
}
