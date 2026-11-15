package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNullableDistinguishesAbsentNullAndValue(t *testing.T) {
	var body struct {
		A Nullable[string] `json:"a"`
		B Nullable[string] `json:"b"`
		C Nullable[int64]  `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"b":null,"c":5}`), &body); err != nil {
		t.Fatal(err)
	}
	if body.A.Set || body.A.Value != nil {
		t.Errorf("absent field = %+v", body.A)
	}
	if !body.B.Set || body.B.Value != nil {
		t.Errorf("null field = %+v", body.B)
	}
	if !body.C.Set || body.C.Value == nil || *body.C.Value != 5 {
		t.Errorf("value field = %+v", body.C)
	}
}

func TestNullableRejectsAWrongType(t *testing.T) {
	var body struct {
		C Nullable[int64] `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"c":"five"}`), &body); err == nil {
		t.Error("a string for an integer field was accepted")
	}
}

func TestDecodeJSONRejectsAWrongTypedNullableNestedInAList(t *testing.T) {
	var body struct {
		List []Nullable[int] `json:"list"`
	}
	r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"list":[1,"x"]}`))
	r.Header.Set("Content-Type", "application/json")
	var apiErr *APIError
	if err := DecodeJSON(r, &body); !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity {
		t.Errorf("error = %v, want a 422", err)
	}
}

func TestDecodeJSONReportsAWrongTypedNullableByFieldName(t *testing.T) {
	var body struct {
		Name Nullable[string]  `json:"name"`
		Low  Nullable[float64] `json:"default_low"`
		High Nullable[float64] `json:"default_high,omitempty"`
		Days Nullable[int]     `json:"days"`
	}
	r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"name":"ok","default_low":"x","default_high":true,"days":1.5}`))
	r.Header.Set("Content-Type", "application/json")
	err := DecodeJSON(r, &body)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("error = %v, want a 422", err)
	}
	want := map[string]string{"default_low": "invalid", "default_high": "invalid", "days": "invalid"}
	if len(apiErr.Fields) != len(want) {
		t.Fatalf("fields = %v, want %v", apiErr.Fields, want)
	}
	for field, code := range want {
		if apiErr.Fields[field] != code {
			t.Errorf("fields = %v, want %v", apiErr.Fields, want)
		}
	}
}

func TestNullableKeepsFalsyValues(t *testing.T) {
	var body struct {
		Zero  Nullable[float64] `json:"zero"`
		Empty Nullable[string]  `json:"empty"`
	}
	if err := json.Unmarshal([]byte(`{"zero":0,"empty":""}`), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Zero.Set || body.Zero.Value == nil || *body.Zero.Value != 0 {
		t.Errorf("zero = %+v", body.Zero)
	}
	if !body.Empty.Set || body.Empty.Value == nil || *body.Empty.Value != "" {
		t.Errorf("empty = %+v", body.Empty)
	}
}
