package httpheader

import (
	"reflect"
	"testing"
)

func TestMergeCanonicalizesNamesAndAppliesLayerPrecedence(t *testing.T) {
	base := map[string]string{"x-api-key": "base", "X-Base": "base-only"}
	dominant := map[string]string{"X-API-KEY": "dominant", "x-request": "request-only"}

	got := Merge(base, dominant)
	want := map[string]string{
		"X-Api-Key": "dominant",
		"X-Base":    "base-only",
		"X-Request": "request-only",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Merge() = %#v, want %#v", got, want)
	}
	if base["x-api-key"] != "base" || dominant["X-API-KEY"] != "dominant" {
		t.Fatalf("Merge() mutated its inputs: base=%#v dominant=%#v", base, dominant)
	}
}

func TestMergeReturnsNilWhenEmpty(t *testing.T) {
	if got := Merge(nil, map[string]string{}); got != nil {
		t.Fatalf("Merge() = %#v, want nil", got)
	}
}
