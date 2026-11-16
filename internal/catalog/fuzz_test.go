package catalog

import "testing"

func FuzzCheckJSON(f *testing.F) {
	for _, seed := range []string{`{}`, `{"a":1,"a":2}`, `{} {}`, `{"a":[1,2,3]}`, "\xff"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = checkJSON(data)
	})
}
