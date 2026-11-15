package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWriteErrorFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, Validation("invalid input", map[string]string{"value": "required"}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d", rec.Code)
	}
	want := `{"error":{"code":"validation","message":"invalid input","fields":{"value":"required"}}}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body = %q, want %q", rec.Body.String(), want)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("content type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("cache control = %q", got)
	}
}

func TestWriteErrorOmitsEmptyFields(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, NotFound("no such tank"))
	want := `{"error":{"code":"not_found","message":"no such tank"}}` + "\n"
	if rec.Code != http.StatusNotFound || rec.Body.String() != want {
		t.Errorf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestWriteErrorHidesInternalErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errors.New("database password is hunter2"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") || !strings.Contains(rec.Body.String(), `"code":"internal"`) {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestWriteErrorUnwrapsWrappedAPIError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errors.Join(errors.New("context"), Conflict("name taken")))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d", rec.Code)
	}
}

type payload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func jsonRequest(contentType, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	return r
}

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
		wantCode    string
		wantField   string
	}{
		{"ok", "application/json", `{"name":"a","count":2}`, 0, "", ""},
		{"ok with charset and trailing newline", "application/json; charset=utf-8", "{\"name\":\"a\"}\n", 0, "", ""},
		{"missing content type", "", `{}`, 415, CodeUnsupportedMediaType, ""},
		{"wrong content type", "text/plain", `{}`, 415, CodeUnsupportedMediaType, ""},
		{"empty body", "application/json", ``, 400, CodeBadRequest, ""},
		{"syntax error", "application/json", `{"name":`, 400, CodeBadRequest, ""},
		{"garbage", "application/json", `nope`, 400, CodeBadRequest, ""},
		{"unknown field", "application/json", `{"colour":"a"}`, 422, CodeValidation, "colour"},
		{"wrong type", "application/json", `{"count":"x"}`, 422, CodeValidation, "count"},
		{"trailing value", "application/json", `{"name":"a"}{"name":"b"}`, 400, CodeBadRequest, ""},
		{"trailing garbage", "application/json", `{"name":"a"} x`, 400, CodeBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p payload
			err := DecodeJSON(jsonRequest(tt.contentType, tt.body), &p)
			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *APIError", err)
			}
			if apiErr.Status != tt.wantStatus || apiErr.Code != tt.wantCode {
				t.Errorf("got %d %s (%s), want %d %s", apiErr.Status, apiErr.Code, apiErr.Message, tt.wantStatus, tt.wantCode)
			}
			if tt.wantField != "" {
				if _, ok := apiErr.Fields[tt.wantField]; !ok {
					t.Errorf("fields = %v, want key %q", apiErr.Fields, tt.wantField)
				}
			}
		})
	}
}

func TestDecodeJSONValues(t *testing.T) {
	var p payload
	if err := DecodeJSON(jsonRequest("application/json", `{"name":"clownfish","count":3}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "clownfish" || p.Count != 3 {
		t.Errorf("decoded %+v", p)
	}
}

func TestDecodeJSONTooLarge(t *testing.T) {
	r := jsonRequest("application/json", `{"name":"`+strings.Repeat("x", 100)+`"}`)
	rec := httptest.NewRecorder()
	r.Body = http.MaxBytesReader(rec, r.Body, 20)

	var p payload
	err := DecodeJSON(r, &p)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusRequestEntityTooLarge || apiErr.Code != CodePayloadTooLarge {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteErrorSendsRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, TooManyRequests("slow down", 90*time.Second+time.Millisecond))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "91" {
		t.Errorf("Retry-After = %q, want 91", got)
	}
	if !strings.Contains(rec.Body.String(), `"code":"too_many_requests"`) {
		t.Errorf("body = %s", rec.Body)
	}
	rec = httptest.NewRecorder()
	WriteError(rec, TooManyRequests("slow down", 0))
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After for zero wait = %q, want 1", got)
	}
}

func TestFailLogsInternalErrorsOnly(t *testing.T) {
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/thing", nil)

	rec := httptest.NewRecorder()
	Fail(logger, rec, req, errors.New("disk on fire"))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "disk") {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), "disk on fire") || !strings.Contains(logs.String(), "/api/v1/thing") {
		t.Errorf("log = %q", logs.String())
	}

	logs.Reset()
	rec = httptest.NewRecorder()
	Fail(logger, rec, req, NotFound("nope"))
	if rec.Code != http.StatusNotFound || logs.Len() != 0 {
		t.Errorf("client error: status=%d log=%q", rec.Code, logs.String())
	}
}

func TestRequireJSONOrNoBody(t *testing.T) {
	chunked := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("data"))
	chunked.ContentLength = -1
	tests := []struct {
		name string
		req  *http.Request
		ok   bool
	}{
		{"no body, no type", httptest.NewRequest(http.MethodPost, "/x", nil), true},
		{"empty body, no type", httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("")), true},
		{"json type", withType(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{}")), "application/json"), true},
		{"json type with charset", withType(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{}")), "application/json; charset=utf-8"), true},
		{"json type, empty body", withType(httptest.NewRequest(http.MethodPost, "/x", nil), "application/json"), true},
		{"form type, empty body", withType(httptest.NewRequest(http.MethodPost, "/x", nil), "application/x-www-form-urlencoded"), false},
		{"form type with body", withType(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("a=b")), "application/x-www-form-urlencoded"), false},
		{"text type", withType(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{}")), "text/plain"), false},
		{"multipart type", withType(httptest.NewRequest(http.MethodPost, "/x", nil), "multipart/form-data; boundary=x"), false},
		{"malformed type", withType(httptest.NewRequest(http.MethodPost, "/x", nil), "json/"), false},
		{"body without a type", httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("a=b")), false},
		{"unknown length without a type", chunked, false},
	}
	for _, tc := range tests {
		err := RequireJSONOrNoBody(tc.req)
		var apiErr *APIError
		switch {
		case tc.ok && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case !tc.ok && (!errors.As(err, &apiErr) || apiErr.Status != http.StatusUnsupportedMediaType):
			t.Errorf("%s: err = %v, want 415", tc.name, err)
		}
	}
}

func withType(r *http.Request, ct string) *http.Request {
	r.Header.Set("Content-Type", ct)
	return r
}
