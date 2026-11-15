// Package httpx holds the HTTP helpers shared by every feature package: JSON, errors, middleware and the current user.
package httpx

import (
	"cmp"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"
)

// Error codes used in the "code" field of the JSON error body.
const (
	CodeBadRequest           = "bad_request"
	CodeValidation           = "validation"
	CodeUnauthorized         = "unauthorized"
	CodeForbidden            = "forbidden"
	CodeNotFound             = "not_found"
	CodeConflict             = "conflict"
	CodeTooManyRequests      = "too_many_requests"
	CodePayloadTooLarge      = "payload_too_large"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeInternal             = "internal"
)

// APIError is an error that maps to an HTTP status and the JSON error body.
type APIError struct {
	Status  int
	Code    string
	Message string
	// Fields maps a request field name to the code that says what is wrong with it.
	Fields map[string]string
	// Header holds extra response headers, such as Retry-After.
	Header http.Header
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

// Validation is a 422 carrying a code per refused field.
func Validation(f FieldErrors) *APIError {
	return &APIError{Status: http.StatusUnprocessableEntity, Code: CodeValidation, Message: cmp.Or(f.Message, defaultFieldsMessage), Fields: f.asMap()}
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

// TooManyRequests is a 429 that tells the client how long to wait.
func TooManyRequests(message string, retryAfter time.Duration) *APIError {
	e := NewError(http.StatusTooManyRequests, CodeTooManyRequests, message)
	seconds := max(1, int64(math.Ceil(retryAfter.Seconds())))
	e.Header = http.Header{"Retry-After": {strconv.FormatInt(seconds, 10)}}
	return e
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// apiErrorOf returns the response for err: an *APIError it wraps, a validation failure for field errors or a 404 for sql.ErrNoRows, and nil for anything else.
func apiErrorOf(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	var fields FieldErrors
	if fields.AddFrom(err) {
		return Validation(fields)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return NotFound("not found")
	}
	return nil
}

// WriteError writes err as the JSON error body, turning any error it cannot map into a generic 500 so internals never leak.
func WriteError(w http.ResponseWriter, err error) {
	apiErr := apiErrorOf(err)
	if apiErr == nil {
		apiErr = NewError(http.StatusInternalServerError, CodeInternal, "internal error")
	}
	for name, values := range apiErr.Header {
		w.Header()[name] = values
	}
	WriteJSON(w, apiErr.Status, errorBody{Error: errorDetail{
		Code:    apiErr.Code,
		Message: apiErr.Message,
		Fields:  apiErr.Fields,
	}})
}

// Fail writes err as the JSON error body; an error with no API form is logged first, since the client only sees "internal error".
func Fail(logger *slog.Logger, w http.ResponseWriter, r *http.Request, err error) {
	if apiErrorOf(err) == nil {
		logger.LogAttrs(r.Context(), slog.LevelError, "request failed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Any("error", err),
		)
	}
	WriteError(w, err)
}
