package activity

import (
	"bytes"
	"encoding/json"
	"slices"
)

// Change is the before / after value of one changed field.
type Change struct {
	Before any `json:"before"`
	After  any `json:"after"`
}

// maxDiffDepth bounds how deep nested objects are walked; deeper objects
// (and every array) compare as one value.
const maxDiffDepth = 3

// Diff compares the JSON form of before and after and returns the changed
// fields by dot path ("reservation.approval_mode"). Nested objects are
// walked; arrays compare as a whole. Fields listed in skip (top-level JSON
// names, such as "updated_at") are ignored. A value that does not encode to
// a JSON object gives no changes.
func Diff(before, after any, skip ...string) map[string]Change {
	a, okA := toObject(before)
	b, okB := toObject(after)
	if !okA || !okB {
		return nil
	}
	out := map[string]Change{}
	diffObjects("", a, b, skip, 1, out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func toObject(v any) (map[string]any, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, false
	}
	return obj, true
}

func diffObjects(prefix string, a, b map[string]any, skip []string, depth int, out map[string]Change) {
	keys := make([]string, 0, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		if prefix == "" && slices.Contains(skip, k) {
			continue
		}
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		va, vb := a[k], b[k]
		oa, isObjA := va.(map[string]any)
		ob, isObjB := vb.(map[string]any)
		if isObjA && isObjB && depth < maxDiffDepth {
			diffObjects(path, oa, ob, skip, depth+1, out)
			continue
		}
		if !sameJSON(va, vb) {
			out[path] = Change{Before: va, After: vb}
		}
	}
}

func sameJSON(a, b any) bool {
	ra, errA := json.Marshal(a)
	rb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ra, rb)
}
