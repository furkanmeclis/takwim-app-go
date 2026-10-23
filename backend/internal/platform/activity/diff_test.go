package activity

import (
	"reflect"
	"testing"
	"time"
)

func TestDiff(t *testing.T) {
	t.Parallel()
	type rules struct {
		Mode  string `json:"mode"`
		Limit *int   `json:"limit"`
	}
	type doc struct {
		Name      string    `json:"name"`
		Tags      []string  `json:"tags"`
		Rules     rules     `json:"rules"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	ten := 10
	before := doc{Name: "Merkez", Tags: []string{"a"}, Rules: rules{Mode: "auto"}, UpdatedAt: time.Unix(1, 0)}
	after := doc{Name: "Kadıköy", Tags: []string{"a", "b"}, Rules: rules{Mode: "auto", Limit: &ten}, UpdatedAt: time.Unix(2, 0)}
	got := Diff(before, after, "updated_at")
	want := map[string]Change{
		"name":        {Before: "Merkez", After: "Kadıköy"},
		"tags":        {Before: []any{"a"}, After: []any{"a", "b"}},
		"rules.limit": {Before: nil, After: float64(10)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff = %#v, want %#v", got, want)
	}
	if Diff(before, before) != nil {
		t.Fatal("equal values must give no changes")
	}
	if Diff("x", after) != nil {
		t.Fatal("non-object values must give no changes")
	}
}
