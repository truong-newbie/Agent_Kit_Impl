package paths

import "testing"

func FuzzValidateRelativePath(f *testing.F) {
	for _, seed := range []string{"", ".", "..", "../a", "CON.txt", "a\\b", "a:b", "skills/plan/SKILL.md", "a/b/c/d/e/f/g/h/i/j/k/l/m"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, p string) {
		ok := ValidateRelativePath(p)
		if ok && (p == "." || p == "" || p[0] == '/') {
			t.Fatalf("accepted unsafe path %q", p)
		}
	})
}
