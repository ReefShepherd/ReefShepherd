package httpx

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
)

// WriteJSON writes v as JSON with the given status; API responses are never cached.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":{"code":"internal","message":"internal error"}}`, http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// DecodeJSON reads one JSON value from the request body into dst, rejecting other content types, unknown fields, trailing data and oversized bodies with an *APIError.
func DecodeJSON(r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return NewError(http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "Content-Type must be application/json")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return classifyDecodeError(err, nil, dst)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return classifyDecodeError(err, body, dst)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return classifyDecodeError(err, body, dst)
		}
		return BadRequest("request body must contain a single JSON value")
	}
	return nil
}

// nullableField marks the Nullable fields of a request struct.
type nullableField interface {
	json.Unmarshaler
	nullable()
}

// wrongTypedNullables names the top-level Nullable fields of dst whose value in body has the wrong type, which the decoder cannot say because an UnmarshalJSON error carries no field name.
func wrongTypedNullables(body []byte, dst any) error {
	v := reflect.Indirect(reflect.ValueOf(dst))
	var raw map[string]json.RawMessage
	if v.Kind() != reflect.Struct || json.Unmarshal(body, &raw) != nil {
		return nil
	}
	fields := FieldErrors{Message: "invalid value type"}
	for i := range v.NumField() {
		sf := v.Type().Field(i)
		name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
		name = cmp.Or(name, sf.Name)
		if probe, ok := reflect.New(sf.Type).Interface().(nullableField); ok && raw[name] != nil && probe.UnmarshalJSON(raw[name]) != nil {
			fields.Add(name, CodeInvalid)
		}
	}
	if fields.Err() == nil {
		return nil
	}
	return Validation(fields)
}

// RequireJSONOrNoBody rejects a request to an endpoint that takes no body when it carries a non-JSON content type or a body of any other kind, so form and text posts are refused here as they are everywhere else.
func RequireJSONOrNoBody(r *http.Request) error {
	unsupported := NewError(http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "Content-Type must be application/json")
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mediaType, _, err := mime.ParseMediaType(ct); err != nil || mediaType != "application/json" {
			return unsupported
		}
		return nil
	}
	// A missing content type is fine only when there is nothing to interpret; an unknown length counts as a body.
	if r.ContentLength != 0 && r.Body != http.NoBody {
		return unsupported
	}
	return nil
}

func classifyDecodeError(err error, body []byte, dst any) error {
	var (
		syntaxErr  *json.SyntaxError
		typeErr    *json.UnmarshalTypeError
		tooLargeEr *http.MaxBytesError
	)
	switch {
	case errors.As(err, &tooLargeEr):
		return NewError(http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "request body is too large")
	case errors.Is(err, io.EOF):
		return BadRequest("request body is empty")
	case errors.As(err, &syntaxErr):
		return BadRequest(fmt.Sprintf("malformed JSON at byte %d", syntaxErr.Offset))
	case errors.Is(err, io.ErrUnexpectedEOF):
		return BadRequest("malformed JSON: unexpected end of body")
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			if wrong := wrongTypedNullables(body, dst); wrong != nil {
				return wrong
			}
			field = "body"
		}
		return rejection("invalid value type", field)
	}
	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return rejection("unknown field", strings.Trim(name, `"`))
	}
	return BadRequest("invalid JSON body")
}

// rejection is the 422 for one request field that cannot be used as sent.
func rejection(message, field string) *APIError {
	f := FieldErrors{Message: message}
	f.Add(field, CodeInvalid)
	return Validation(f)
}
