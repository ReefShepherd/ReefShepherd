package httpx

import (
	"errors"
	"strings"
)

// Code says why a field was refused; the browser turns each one into a sentence.
type Code string

// The closed list of field error codes.
const (
	CodeRequired   Code = "required"
	CodeInvalid    Code = "invalid"
	CodeTooShort   Code = "too_short"
	CodeTooLong    Code = "too_long"
	CodeOutOfRange Code = "out_of_range"
	CodeTaken      Code = "taken"
	CodeIncorrect  Code = "incorrect"
	CodeLocked     Code = "locked"
)

// known returns c when it is on the closed list and CodeInvalid otherwise, so a code reported by a package that cannot import this one never reaches the client unlisted.
func known(c Code) Code {
	switch c {
	case CodeRequired, CodeInvalid, CodeTooShort, CodeTooLong, CodeOutOfRange, CodeTaken, CodeIncorrect, CodeLocked:
		return c
	}
	return CodeInvalid
}

// defaultFieldsMessage is the summary of a validation failure that names none.
const defaultFieldsMessage = "invalid request"

// FieldErrors collects what is wrong with a request, keyed by the API field name, and is itself the error to return.
type FieldErrors struct {
	// Message is the summary sent beside the fields.
	Message string
	codes   map[string]Code
}

// Add records why field was refused; the first reason given for a field wins.
func (f *FieldErrors) Add(field string, code Code) {
	if _, taken := f.codes[field]; taken {
		return
	}
	if f.codes == nil {
		f.codes = map[string]Code{}
	}
	f.codes[field] = code
}

// AddFrom records the problems err carries, whether it is a FieldErrors, an error with a FieldError method or several of those joined, and reports whether it found any.
func (f *FieldErrors) AddFrom(err error) bool {
	before := len(f.codes)
	f.collect(err)
	return len(f.codes) > before
}

func (f *FieldErrors) collect(err error) {
	switch e := err.(type) { //nolint:errorlint // the walk below unwraps by hand so every joined error is visited
	case nil:
	case *FieldErrors:
		if f.Message == "" {
			f.Message = e.Message
		}
		for field, code := range e.codes {
			f.Add(field, code)
		}
	case interface{ FieldError() (field, code string) }:
		field, code := e.FieldError()
		f.Add(field, known(Code(code)))
	case interface{ Unwrap() []error }:
		for _, inner := range e.Unwrap() {
			f.collect(inner)
		}
	default:
		f.collect(errors.Unwrap(err))
	}
}

// Err returns f as an error, nil when no field was refused.
func (f *FieldErrors) Err() error {
	if len(f.codes) == 0 {
		return nil
	}
	return f
}

func (f *FieldErrors) Error() string {
	parts := make([]string, 0, len(f.codes))
	for field, code := range f.codes {
		parts = append(parts, field+": "+string(code))
	}
	return "validation: " + strings.Join(parts, ", ")
}

// Reject is the error for one refused field, with message as the summary.
func Reject(message, field string, code Code) error {
	f := FieldErrors{Message: message}
	f.Add(field, code)
	return &f
}

// asMap returns the codes as the JSON body carries them.
func (f FieldErrors) asMap() map[string]string {
	out := make(map[string]string, len(f.codes))
	for field, code := range f.codes {
		out[field] = string(code)
	}
	return out
}
