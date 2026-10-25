package testutil

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/reefshepherd/reefshepherd/internal/db"
	"github.com/reefshepherd/reefshepherd/internal/httpx"
)

// recorder stands in for *testing.T so a test can observe a helper failing without failing itself.
type recorder struct {
	testing.TB
	failed bool
	msg    string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatal(args ...any) { r.stop(fmt.Sprint(args...)) }

func (r *recorder) Fatalf(format string, args ...any) { r.stop(fmt.Sprintf(format, args...)) }

func (r *recorder) stop(msg string) {
	r.failed, r.msg = true, msg
	runtime.Goexit()
}

// failure runs fn against a recorder on its own goroutine, because a failing helper ends the goroutine it runs on.
func failure(t *testing.T, fn func(tb testing.TB)) *recorder {
	t.Helper()
	r := &recorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(r)
	}()
	<-done
	return r
}

func mustPass(t *testing.T, fn func(tb testing.TB)) {
	t.Helper()
	if r := failure(t, fn); r.failed {
		t.Fatalf("helper failed: %s", r.msg)
	}
}

func mustFail(t *testing.T, want string, fn func(tb testing.TB)) {
	t.Helper()
	r := failure(t, fn)
	if !r.failed {
		t.Fatalf("helper passed, want a failure mentioning %q", want)
	}
	if !strings.Contains(r.msg, want) {
		t.Fatalf("failure = %q, want it to mention %q", r.msg, want)
	}
}

func TestOpenDBIsMigratedAndWritable(t *testing.T) {
	d := OpenDB(t)
	if n := Count(t, d, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'users'`); n != 1 {
		t.Fatalf("users table count = %d, want 1", n)
	}
	if n := Count(t, d, `SELECT COUNT(*) FROM parameter_definitions`); n == 0 {
		t.Error("the seeded parameter presets are missing")
	}
	first := Insert(t, d, `INSERT INTO tanks (name) VALUES (?)`, "a")
	second := Insert(t, d, `INSERT INTO tanks (name) VALUES (?)`, "b")
	if second <= first {
		t.Errorf("insert ids = %d then %d, want increasing", first, second)
	}
	Exec(t, d, `UPDATE tanks SET name = 'c' WHERE id = ?`, first)
	if n := Count(t, d, `SELECT COUNT(*) FROM tanks WHERE name = 'c'`); n != 1 {
		t.Errorf("renamed tanks = %d, want 1", n)
	}
	if OpenDB(t) == d {
		t.Error("each call must open its own database")
	}
}

func TestOpenDBClosesWhenTheTestEnds(t *testing.T) {
	var d *db.DB
	t.Run("inner", func(t *testing.T) { d = OpenDB(t) })
	if err := d.W.PingContext(t.Context()); err == nil {
		t.Error("the database was still open after its test ended")
	}
}

func TestExecAndCountFailOnBadSQL(t *testing.T) {
	d := OpenDB(t)
	mustFail(t, "no_such_table", func(tb testing.TB) { Exec(tb, d, `INSERT INTO no_such_table VALUES (1)`) })
	mustFail(t, "no_such_table", func(tb testing.TB) { Count(tb, d, `SELECT COUNT(*) FROM no_such_table`) })
	mustFail(t, "no_such_table", func(tb testing.TB) { Insert(tb, d, `INSERT INTO no_such_table VALUES (1)`) })
}

func TestLoggerDiscards(t *testing.T) {
	if Logger(t).Enabled(t.Context(), slog.LevelError) {
		t.Error("the logger would write output")
	}
}

// echo reports what the handler received.
var echo = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	u, signedIn := httpx.CurrentUser(r.Context())
	_, _ = fmt.Fprintf(w, "%s %s ct=%q body=%q user=%v:%d:%s", r.Method, r.URL.RequestURI(), r.Header.Get("Content-Type"), b, signedIn, u.ID, u.Role)
})

func TestDoJSONEncodesTheBody(t *testing.T) {
	rec := DoJSON(t, echo, http.MethodPost, "/x?y=1", map[string]any{"a": 1})
	want := `POST /x?y=1 ct="application/json" body="{\"a\":1}" user=false:0:`
	if got := rec.Body.String(); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDoJSONWithoutABodySendsNone(t *testing.T) {
	rec := DoJSON(t, echo, http.MethodGet, "/x", nil)
	if got, want := rec.Body.String(), `GET /x ct="" body="" user=false:0:`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDoRawSendsTheBodyAsIs(t *testing.T) {
	rec := DoRaw(t, echo, http.MethodPut, "/x", "text/plain", "hello")
	if got, want := rec.Body.String(), `PUT /x ct="text/plain" body="hello" user=false:0:`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDoJSONRejectsAnUnencodableBody(t *testing.T) {
	mustFail(t, "unsupported type", func(tb testing.TB) { DoJSON(tb, echo, http.MethodPost, "/x", make(chan int)) })
}

func TestAsUserSignsTheRequestIn(t *testing.T) {
	rec := DoJSON(t, echo, http.MethodGet, "/x", nil, AsUser(7, httpx.RoleAdmin))
	if got, want := rec.Body.String(), `GET /x ct="" body="" user=true:7:admin`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestWithHeaderSetsAndReplacesAHeader(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%q", r.Header.Values("X-Probe"))
	})
	rec := DoRaw(t, h, http.MethodGet, "/x", "", "", WithHeader("X-Probe", "first"), WithHeader("X-Probe", "second"))
	if got, want := rec.Body.String(), `["second"]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDecode(t *testing.T) {
	type item struct{ N int }
	rec := httptest.NewRecorder()
	_, _ = rec.WriteString(`{"N":3}`)
	if got := Decode[item](t, rec); got.N != 3 {
		t.Errorf("decoded %+v, want N=3", got)
	}
	var into item
	DecodeInto(t, rec, &into)
	if into.N != 3 {
		t.Errorf("decoded %+v, want N=3", into)
	}
	bad := httptest.NewRecorder()
	_, _ = bad.WriteString(`not json`)
	mustFail(t, "not json", func(tb testing.TB) { Decode[item](tb, bad) })
}

func TestExpectStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusTeapot)
	mustPass(t, func(tb testing.TB) { ExpectStatus(tb, rec, http.StatusTeapot) })
	mustFail(t, "status = 418, want 200", func(tb testing.TB) { ExpectStatus(tb, rec, http.StatusOK) })
}

func errorResponse(status int, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rec.WriteHeader(status)
	_, _ = rec.WriteString(body)
	return rec
}

func TestExpectError(t *testing.T) {
	rec := errorResponse(http.StatusUnprocessableEntity, `{"error":{"code":"validation","message":"bad","fields":{"name":"required","note":"too long"}}}`)

	mustPass(t, func(tb testing.TB) { ExpectError(tb, rec, http.StatusUnprocessableEntity, "validation") })
	mustPass(t, func(tb testing.TB) {
		ExpectError(tb, rec, http.StatusUnprocessableEntity, "validation", "note", "name")
	})
	if b := ExpectError(t, rec, http.StatusUnprocessableEntity, "validation"); b.Error.Message != "bad" || b.Error.Fields["name"] != "required" {
		t.Errorf("returned %+v", b)
	}

	mustFail(t, "status=422", func(tb testing.TB) { ExpectError(tb, rec, http.StatusConflict, "validation") })
	mustFail(t, `code="validation"`, func(tb testing.TB) { ExpectError(tb, rec, http.StatusUnprocessableEntity, "conflict") })
	mustFail(t, "want exactly", func(tb testing.TB) { ExpectError(tb, rec, http.StatusUnprocessableEntity, "validation", "name") })
	mustFail(t, "want exactly", func(tb testing.TB) {
		ExpectError(tb, rec, http.StatusUnprocessableEntity, "validation", "name", "note", "extra")
	})
	mustFail(t, `no message for field "other"`, func(tb testing.TB) {
		ExpectError(tb, rec, http.StatusUnprocessableEntity, "validation", "name", "other")
	})
	empty := errorResponse(http.StatusUnprocessableEntity, `{"error":{"code":"validation","fields":{"name":""}}}`)
	mustFail(t, `no message for field "name"`, func(tb testing.TB) { ExpectError(tb, empty, http.StatusUnprocessableEntity, "validation", "name") })
	mustFail(t, "code=\"\"", func(tb testing.TB) {
		ExpectError(tb, errorResponse(http.StatusNotFound, `<html>`), http.StatusNotFound, "not_found")
	})
}
