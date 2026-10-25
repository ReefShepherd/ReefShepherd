package httpx

import (
	"encoding/json"
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
