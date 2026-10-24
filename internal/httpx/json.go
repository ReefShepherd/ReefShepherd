package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
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

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return classifyDecodeError(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return classifyDecodeError(err)
		}
		return BadRequest("request body must contain a single JSON value")
	}
	return nil
}

func classifyDecodeError(err error) error {
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
			field = "body"
		}
		return Validation("invalid value type", map[string]string{field: "must be " + jsonTypeName(typeErr.Type.String())})
	}
	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return Validation("unknown field", map[string]string{strings.Trim(name, `"`): "unknown field"})
	}
	return BadRequest("invalid JSON body")
}

// jsonTypeName turns a Go type name into the JSON word a client would use.
func jsonTypeName(goType string) string {
	switch {
	case goType == "string":
		return "a string"
	case goType == "bool":
		return "a boolean"
	case strings.HasPrefix(goType, "[]"):
		return "an array"
	case strings.HasPrefix(goType, "map["), strings.HasPrefix(goType, "struct"):
		return "an object"
	case strings.HasPrefix(goType, "int"), strings.HasPrefix(goType, "uint"), strings.HasPrefix(goType, "float"):
		return "a number"
	}
	return "a different type"
}
