package httpx

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// typedFieldError is the shape other packages use to report a field without importing httpx.
type typedFieldError struct{ field, code string }

func (e typedFieldError) Error() string                    { return e.field + " " + e.code }
func (e typedFieldError) FieldError() (field, code string) { return e.field, e.code }

func TestFieldErrorsErrIsNilWhenEmpty(t *testing.T) {
	var f FieldErrors
	if err := f.Err(); err != nil {
		t.Errorf("Err() of an empty collector = %v, want nil", err)
	}
	f.Add("name", CodeRequired)
	if f.Err() == nil {
		t.Error("Err() of a collector with a field = nil")
	}
}

func TestFieldErrorsFirstReasonWins(t *testing.T) {
	var f FieldErrors
	f.Add("name", CodeRequired)
	f.Add("name", CodeTooLong)
	if got := Validation(f).Fields["name"]; got != "required" {
		t.Errorf("name = %q, want the first code", got)
	}
}

func TestValidationMessage(t *testing.T) {
	var f FieldErrors
	f.Add("a", CodeInvalid)
	if got := Validation(f).Message; got != "invalid request" {
		t.Errorf("default message = %q", got)
	}
	f.Message = "invalid tank"
	if got := Validation(f).Message; got != "invalid tank" {
		t.Errorf("message = %q", got)
	}
}

func TestAddFromCollectsEveryKindOfFieldError(t *testing.T) {
	inner := FieldErrors{Message: "inner"}
	inner.Add("note", CodeTooLong)
	cases := map[string]struct {
		err  error
		want map[string]string
	}{
		"typed":         {typedFieldError{"timezone", "invalid"}, map[string]string{"timezone": "invalid"}},
		"wrapped typed": {fmt.Errorf("saving: %w", typedFieldError{"timezone", "invalid"}), map[string]string{"timezone": "invalid"}},
		"collector":     {inner.Err(), map[string]string{"note": "too_long"}},
		"joined": {errors.Join(typedFieldError{"a", "required"}, nil, typedFieldError{"b", "locked"}),
			map[string]string{"a": "required", "b": "locked"}},
		"first field wins": {errors.Join(typedFieldError{"a", "required"}, typedFieldError{"a", "taken"}), map[string]string{"a": "required"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var f FieldErrors
			if !f.AddFrom(tc.err) {
				t.Fatal("AddFrom found no field error")
			}
			got := Validation(f).Fields
			if len(got) != len(tc.want) {
				t.Fatalf("fields = %v, want %v", got, tc.want)
			}
			for field, code := range tc.want {
				if got[field] != code {
					t.Errorf("fields = %v, want %v", got, tc.want)
				}
			}
		})
	}
	var f FieldErrors
	if f.AddFrom(errors.New("disk on fire")) || f.AddFrom(nil) {
		t.Error("AddFrom reported field errors in an error that has none")
	}
}

func TestAnUnlistedCodeBecomesInvalid(t *testing.T) {
	var f FieldErrors
	f.AddFrom(typedFieldError{"a", "out_of_rangee"})
	f.AddFrom(typedFieldError{"b", "too_long"})
	if got := Validation(f).Fields; got["a"] != "invalid" || got["b"] != "too_long" {
		t.Errorf("fields = %v", got)
	}
}

func TestRejectCarriesMessageFieldAndCode(t *testing.T) {
	err := Reject("incorrect password", "current_password", CodeIncorrect)
	var f FieldErrors
	if !f.AddFrom(err) || f.Message != "incorrect password" {
		t.Fatalf("message = %q", f.Message)
	}
	if got := Validation(f).Fields; len(got) != 1 || got["current_password"] != "incorrect" {
		t.Errorf("fields = %v", got)
	}
}

func TestHandleMapsFieldErrorsNotFoundAndSentinels(t *testing.T) {
	errGone := errors.New("gone")
	mapErr := func(err error) error {
		if errors.Is(err, errGone) {
			return NotFound("no such thing")
		}
		return err
	}
	cases := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{"field error", Reject("invalid tank", "name", CodeRequired), http.StatusUnprocessableEntity, "validation", "invalid tank"},
		{"typed field error", typedFieldError{"timezone", "invalid"}, http.StatusUnprocessableEntity, "validation", "invalid request"},
		{"no rows", fmt.Errorf("read: %w", sql.ErrNoRows), http.StatusNotFound, "not_found", "not found"},
		{"mapped sentinel", fmt.Errorf("ctx: %w", errGone), http.StatusNotFound, "not_found", "no such thing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var logs strings.Builder
			h := Handle(slog.New(slog.NewTextHandler(&logs, nil)), func(http.ResponseWriter, *http.Request) error { return c.err }, mapErr)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			body := rec.Body.String()
			if rec.Code != c.status || !strings.Contains(body, `"code":"`+c.code+`"`) || !strings.Contains(body, `"message":"`+c.message+`"`) {
				t.Errorf("status=%d body=%s, want %d %s %q", rec.Code, body, c.status, c.code, c.message)
			}
			if logs.Len() != 0 {
				t.Error("an error with an API form was logged as unexpected")
			}
		})
	}
}
