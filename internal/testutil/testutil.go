// Package testutil is the shared harness for tests: a migrated temporary database, request helpers and response assertions.
package testutil

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// OpenDB opens a fresh database in a temporary directory with every migration applied, and closes it when the test ends.
func OpenDB(t testing.TB) *db.DB {
	t.Helper()
	return OpenDBAt(t, filepath.Join(t.TempDir(), "reef.db"))
}

// OpenDBAt is OpenDB for a database file at path, with migration backups kept in a backups directory beside it.
func OpenDBAt(t testing.TB, path string) *db.DB {
	t.Helper()
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(t.Context(), filepath.Join(filepath.Dir(path), "backups")); err != nil {
		t.Fatal(err)
	}
	return d
}

// Logger returns a logger that discards everything.
func Logger(t testing.TB) *slog.Logger {
	t.Helper()
	return slog.New(slog.DiscardHandler)
}

// Exec runs a write statement and fails the test on error.
func Exec(t testing.TB, d *db.DB, query string, args ...any) {
	t.Helper()
	if _, err := d.W.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// Count runs a query that returns one integer and returns it.
func Count(t testing.TB, d *db.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := d.R.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// ReqOpt adjusts a request before it is served.
type ReqOpt func(*http.Request)

// AsUser attaches a signed-in user to the request context, as the sign-in middleware would.
func AsUser(id int64, role string) ReqOpt {
	return func(r *http.Request) {
		*r = *r.WithContext(httpx.WithUser(r.Context(), httpx.User{ID: id, Username: "user" + strconv.FormatInt(id, 10), Role: role}))
	}
}

// DoJSON serves a request with body encoded as JSON (no body when nil) and returns the recorded response.
func DoJSON(t testing.TB, h http.Handler, method, path string, body any, opts ...ReqOpt) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		return DoRaw(t, h, method, path, "", "", opts...)
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return DoRaw(t, h, method, path, "application/json", string(b), opts...)
}

// DoRaw serves a request with an unparsed body; an empty contentType leaves the header unset.
func DoRaw(t testing.TB, h http.Handler, method, path, contentType, body string, opts ...ReqOpt) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	for _, opt := range opts {
		opt(r)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// DecodeInto unmarshals the response body into v and fails the test on error.
func DecodeInto(t testing.TB, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
}

// Decode unmarshals the response body into a T.
func Decode[T any](t testing.TB, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	DecodeInto(t, rec, &v)
	return v
}

// ErrorBody is the JSON error envelope every failed API call returns.
type ErrorBody struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

// ExpectError returns the decoded error after failing the test unless it has the given status and code and, when
// fields are named, a message for exactly those fields.
func ExpectError(t testing.TB, rec *httptest.ResponseRecorder, status int, code string, fields ...string) ErrorBody {
	t.Helper()
	var b ErrorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if rec.Code != status || b.Error.Code != code {
		t.Fatalf("status=%d code=%q, want %d %q; body=%s", rec.Code, b.Error.Code, status, code, rec.Body)
	}
	if len(fields) == 0 {
		return b
	}
	if len(b.Error.Fields) != len(fields) {
		t.Fatalf("fields=%v, want exactly %v; body=%s", b.Error.Fields, fields, rec.Body)
	}
	for _, f := range fields {
		if b.Error.Fields[f] == "" {
			t.Fatalf("no message for field %q in %v", f, b.Error.Fields)
		}
	}
	return b
}

// ExpectStatus fails the test unless the response has the given status.
func ExpectStatus(t testing.TB, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, status, rec.Body)
	}
}

// Insert runs an INSERT statement and returns the new row id.
func Insert(t testing.TB, d *db.DB, query string, args ...any) int64 {
	t.Helper()
	res, err := d.W.ExecContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
