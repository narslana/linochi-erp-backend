package audit

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

type rec struct {
	ID      uuid.UUID  `json:"id"`
	Name    string     `json:"name"`
	Email   string     `json:"email"`
	Color   *string    `json:"color"`
	Seen    *time.Time `json:"seen"`
	Version int32      `json:"version"`
	Updated time.Time  `json:"updated_at"`
}

func ptr[T any](v T) *T { return &v }

func TestINS05_ToMapTurnsStructsAndMapsIntoJSONObjects(t *testing.T) {
	id := uuid.MustParse("0194fa5e-0000-7000-8000-000000000001")
	got, err := toMap(rec{ID: id, Name: "A", Color: ptr("red")})
	if err != nil {
		t.Fatal(err)
	}
	if got["id"] != id.String() || got["name"] != "A" || got["color"] != "red" {
		t.Errorf("got %v", got)
	}
	if v, ok := got["seen"]; !ok || v != nil {
		t.Errorf("a nil pointer must be an explicit null, got %v (present: %v)", v, ok)
	}

	if m, err := toMap(map[string]any{"n": 1}); err != nil || m["n"] != float64(1) {
		t.Errorf("map: %v, %v", m, err)
	}
	if m, err := toMap(nil); err != nil || m != nil {
		t.Errorf("nil: %v, %v", m, err)
	}
	for _, bad := range []any{"text", 42, []string{"a"}} {
		if _, err := toMap(bad); err == nil {
			t.Errorf("%v is not an object and must be rejected", bad)
		}
	}
}

func TestINS05_ChangedFieldsHoldsOnlyWhatChanged(t *testing.T) {
	ignore := map[string]struct{}{"version": {}, "updated_at": {}}
	tests := []struct {
		name          string
		before, after map[string]any
		wantBefore    map[string]any
		wantAfter     map[string]any
		wantNames     []string
	}{
		{"nothing changed", map[string]any{"a": "x"}, map[string]any{"a": "x"}, nil, nil, nil},
		{"one field", map[string]any{"a": "x", "b": "y"}, map[string]any{"a": "x", "b": "z"},
			map[string]any{"b": "y"}, map[string]any{"b": "z"}, []string{"b"}},
		{"names are sorted", map[string]any{"b": 1.0, "a": 1.0}, map[string]any{"b": 2.0, "a": 2.0},
			map[string]any{"a": 1.0, "b": 1.0}, map[string]any{"a": 2.0, "b": 2.0}, []string{"a", "b"}},
		{"a field that appears", map[string]any{"a": "x"}, map[string]any{"a": "x", "n": "new"},
			map[string]any{"n": nil}, map[string]any{"n": "new"}, []string{"n"}},
		{"a field that disappears", map[string]any{"a": "x", "gone": "g"}, map[string]any{"a": "x"},
			map[string]any{"gone": "g"}, map[string]any{"gone": nil}, []string{"gone"}},
		{"null to value", map[string]any{"c": nil}, map[string]any{"c": "red"},
			map[string]any{"c": nil}, map[string]any{"c": "red"}, []string{"c"}},
		{"nested objects are compared deeply", map[string]any{"o": map[string]any{"k": 1.0}}, map[string]any{"o": map[string]any{"k": 2.0}},
			map[string]any{"o": map[string]any{"k": 1.0}}, map[string]any{"o": map[string]any{"k": 2.0}}, []string{"o"}},
		{"ignored fields alone are not a change", map[string]any{"a": "x", "version": 1.0, "updated_at": "t1"}, map[string]any{"a": "x", "version": 2.0, "updated_at": "t2"}, nil, nil, nil},
		{"ignored fields are left out when something else changed", map[string]any{"a": "x", "version": 1.0}, map[string]any{"a": "y", "version": 2.0},
			map[string]any{"a": "x"}, map[string]any{"a": "y"}, []string{"a"}},
		{"no before: everything in after counts", nil, map[string]any{"fields": []any{"email"}, "rows": 2.0},
			nil, map[string]any{"fields": []any{"email"}, "rows": 2.0}, []string{"fields", "rows"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, a, names := changedFields(tt.before, tt.after, ignore)
			if !reflect.DeepEqual(b, tt.wantBefore) || !reflect.DeepEqual(a, tt.wantAfter) || !reflect.DeepEqual(names, tt.wantNames) {
				t.Errorf("before=%v after=%v names=%v\nwant   before=%v after=%v names=%v", b, a, names, tt.wantBefore, tt.wantAfter, tt.wantNames)
			}
		})
	}
}

func TestINS05_StructDiffThroughToMap(t *testing.T) {
	t1 := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	before, _ := toMap(rec{Name: "Anna", Email: "a@example.se", Version: 1, Updated: t1})
	after, _ := toMap(rec{Name: "Anna", Email: "anna@example.se", Version: 2, Updated: t1.Add(time.Hour)})
	b, a, names := changedFields(before, after, ignoredFields)
	if !reflect.DeepEqual(names, []string{"email"}) || b["email"] != "a@example.se" || a["email"] != "anna@example.se" {
		t.Errorf("before=%v after=%v names=%v, want only email", b, a, names)
	}
}
