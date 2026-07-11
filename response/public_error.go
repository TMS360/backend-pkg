package response

import (
	"fmt"
	"log/slog"
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

type publicError struct {
	Technical string
	User      string
	Status    int
	Code      int
	Ext       map[string]any
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

func NewError(tech, user string, status int) PublicError {
	slog.Error(fmt.Sprintf("[tech=%s,user=%s]", tech, user))
	return &publicError{Technical: tech, User: user, Status: status}
}

// NewErrorWithExtensions is the payload-carrying variant of NewError. The
// extensions map is passed through to the GraphQL presenter so a caller can
// attach structured details (e.g. a blocking resource's id) that clients read
// without parsing the human message. A nil or empty map behaves like NewError.
func NewErrorWithExtensions(tech, user string, status int, ext map[string]any) PublicError {
	slog.Error(fmt.Sprintf("[tech=%s,user=%s]", tech, user))
	return &publicError{Technical: tech, User: user, Status: status, Ext: ext}
}
