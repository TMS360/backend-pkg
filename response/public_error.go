package response

type PublicError interface {
	Error() string
	UserMessage() string
	ErrorStatus() int
}

type publicError struct {
	Technical string
	User      string
	Status    int
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

func NewError(tech, user string, status int) PublicError {
	return &publicError{Technical: tech, User: user, Status: status}
}
