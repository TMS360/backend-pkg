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
}

type publicError struct {
	Technical string
	User      string
	Status    int
	Code      int
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

func NewError(tech, user string, status int) PublicError {
	slog.Error(fmt.Sprintf("[tech=%s,user=%s]", tech, user))
	return &publicError{Technical: tech, User: user, Status: status}
}
