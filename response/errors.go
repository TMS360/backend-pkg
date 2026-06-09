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

func NewForbidden(tech, user string) PublicError {
	return NewError(tech, user, http.StatusForbidden)
}

func NewUnauthorized(tech, user string) PublicError {
	return NewError(tech, user, http.StatusUnauthorized)
}

func NewInternalError(tech string) PublicError {
	return NewError(tech, "Something went wrong. Please try again.", http.StatusInternalServerError)
}
