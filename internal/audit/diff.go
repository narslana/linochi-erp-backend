package audit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
)

// ignoredFields are bookkeeping columns every table has. They change on every
// write, so they say nothing about what happened and are left out of the diff of
// an update. (A create and an archive store the whole record, version included.)
var ignoredFields = map[string]struct{}{"version": {}, "updated_at": {}}

// toMap turns a struct or a map into the JSON object the audit log stores, so
// the field names in the log are the JSON names (the column names for the sqlc
// models). nil stays nil.
func toMap(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("audit: encode record: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("audit: a record must be a struct or a map, got %T", v)
	}
	return m, nil
}

// changedFields compares two records field by field. It returns the old and the
// new values of the fields that differ, and their names in sorted order. When
// nothing differs (ignored fields aside) all three are nil.
//
// A nil before means there is no old record, as for the entry that describes a
// redaction: everything in after counts, and before stays nil.
func changedFields(before, after map[string]any, ignore map[string]struct{}) (b, a map[string]any, names []string) {
	if before == nil {
		for k, v := range after {
			if _, skip := ignore[k]; skip {
				continue
			}
			if a == nil {
				a = map[string]any{}
			}
			a[k] = v
			names = append(names, k)
		}
		slices.Sort(names)
		return nil, a, names
	}

	keys := map[string]struct{}{}
	for k := range before {
		keys[k] = struct{}{}
	}
	for k := range after {
		keys[k] = struct{}{}
	}
	for k := range keys {
		if _, skip := ignore[k]; skip {
			continue
		}
		bv, bok := before[k]
		av, aok := after[k]
		if bok && aok && reflect.DeepEqual(bv, av) {
			continue
		}
		if b == nil {
			b, a = map[string]any{}, map[string]any{}
		}
		b[k], a[k] = bv, av // a missing side is a JSON null
		names = append(names, k)
	}
	slices.Sort(names)
	return b, a, names
}

// keys returns the sorted field names of a record.
func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
