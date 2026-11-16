package state

import "testing"

func FuzzParseAndValidateState(f *testing.F) {
	for _, seed := range []string{"{}", `{"schemaVersion":1,"schemaVersion":1}`, "null", "[]", `{"schemaVersion":1,"managedBy":"dtkit"}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		inst, err := ParseAndValidate(data, "")
		if err == nil {
			if err := inst.ValidateSelf(); err != nil {
				t.Fatalf("ParseAndValidate returned invalid state: %v", err)
			}
		}
	})
}
