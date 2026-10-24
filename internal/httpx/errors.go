// Package httpx holds the HTTP helpers shared by every feature package: JSON, errors, middleware and the current user.
package httpx

import (
	"errors"
	"net/http"
)

// Error codes used in the "code" field of the JSON error body.
const (
	CodeBadRequest           = "bad_request"
	CodeValidation           = "validation"
	CodeUnauthorized         = "unauthorized"
	CodeForbidden            = "forbidden"
	CodeNotFound             = "not_found"
	CodeConflict             = "conflict"
	CodePayloadTooLarge      = "payload_too_large"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeInternal             = "internal"
)

// APIError is an error that maps to an HTTP status and the JSON error body.
type APIError struct {
	Status  int
	Code    string
	Message string
	// Fields maps a request field name to what is wrong with it.
	Fields map[string]string
}

func (e *APIError) Error() string {
	return e.Code + ": " + e.Message
}

// NewError builds an APIError with an arbitrary status and code.
func NewError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

// BadRequest is a 400 for a request that cannot be understood.
func BadRequest(message string) *APIError {
	return NewError(http.StatusBadRequest, CodeBadRequest, message)
}

// Validation is a 422 carrying per-field messages.
func Validation(message string, fields map[string]string) *APIError {
	return &APIError{Status: http.StatusUnprocessableEntity, Code: CodeValidation, Message: message, Fields: fields}
}

// Unauthorized is a 401 for a missing or invalid session.
func Unauthorized(message string) *APIError {
	return NewError(http.StatusUnauthorized, CodeUnauthorized, message)
}

// Forbidden is a 403 for a signed-in user who may not do this.
func Forbidden(message string) *APIError {
	return NewError(http.StatusForbidden, CodeForbidden, message)
}

// NotFound is a 404.
func NotFound(message string) *APIError {
	return NewError(http.StatusNotFound, CodeNotFound, message)
}

// Conflict is a 409 for a request that clashes with current state.
func Conflict(message string) *APIError {
	return NewError(http.StatusConflict, CodeConflict, message)
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// WriteError writes err as the JSON error body, turning any non-*APIError into a generic 500 so internals never leak.
func WriteError(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = NewError(http.StatusInternalServerError, CodeInternal, "internal error")
	}
	WriteJSON(w, apiErr.Status, errorBody{Error: errorDetail{
		Code:    apiErr.Code,
		Message: apiErr.Message,
		Fields:  apiErr.Fields,
	}})
}
