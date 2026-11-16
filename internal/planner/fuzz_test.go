package planner

import (
	"bytes"
	"testing"
)

func FuzzBlockOperations(f *testing.F) {
	seeds := [][]byte{
		[]byte(""),
		[]byte("# Personal\n"),
		[]byte("\xef\xbb\xbf# BOM\r\nNo trailing"),
		[]byte("<!-- dtkit:begin engineer project -->\n"),
		[]byte("<!-- dtkit:end engineer project -->\n"),
		[]byte("<!-- dtkit:begin engineer global -->\nbody\n<!-- dtkit:end engineer global -->\n"),
		[]byte("\xff\xfe\x00"),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, existing []byte) {
		out, st, err := AppendBlock(existing, len(existing) > 0, projectBlock, "project")
		if err != nil {
			// Must never panic; verify BlockDigest fails closed if markers invalid.
			_, _ = BlockDigest(existing, "project")
			return
		}
		if !bytes.HasPrefix(out, existing) {
			t.Fatalf("prefix altered")
		}
		removed, deleteFile, err := RemoveBlock(out, "project", st)
		if err != nil {
			t.Fatalf("remove after append failed: %v", err)
		}
		if !bytes.Equal(removed, existing) {
			t.Fatalf("roundtrip mismatch: got %q want %q", removed, existing)
		}
		if len(existing) == 0 && !deleteFile {
			t.Fatalf("deleteFile expected when existing was empty")
		}
	})
}
