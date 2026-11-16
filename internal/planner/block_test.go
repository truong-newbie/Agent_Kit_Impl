package planner

import (
	"bytes"
	"strings"
	"testing"
)

var projectBlock = []byte("<!-- dtkit:begin engineer project -->\ninitial\n<!-- dtkit:end engineer project -->\n")

func TestBlockPreservesOutsideBytes(t *testing.T) {
	for _, original := range [][]byte{[]byte("existing"), []byte("existing\n"), []byte("existing\r\n"), []byte("\xef\xbb\xbfhello\r\n")} {
		installed, state, err := AppendBlock(original, true, projectBlock, "project")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(installed, original) {
			t.Fatalf("prefix changed: %q", installed)
		}
		installed = append(installed, []byte("user content\n")...)
		newBlock := bytes.Replace(projectBlock, []byte("initial"), []byte("updated"), 1)
		updated, state, err := ReplaceBlock(installed, newBlock, "project", state)
		if err != nil {
			t.Fatal(err)
		}
		removed, deleteFile, err := RemoveBlock(updated, "project", state)
		if err != nil {
			t.Fatal(err)
		}
		want := append(append([]byte{}, original...), []byte("user content\n")...)
		if deleteFile || !bytes.Equal(removed, want) {
			t.Fatalf("got %q, want %q", removed, want)
		}
	}
}

func TestBlockConflictAndNewFile(t *testing.T) {
	installed, state, err := AppendBlock(nil, false, projectBlock, "project")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := AppendBlock(installed, true, projectBlock, "project"); err == nil {
		t.Fatal("adopted unowned block")
	}
	modified := bytes.Replace(installed, []byte("initial"), []byte("modified"), 1)
	if _, _, err := RemoveBlock(modified, "project", state); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected conflict, got %v", err)
	}
	removed, deleteFile, err := RemoveBlock(installed, "project", state)
	if err != nil || !deleteFile || len(removed) != 0 {
		t.Fatalf("remove = %q, %v, %v", removed, deleteFile, err)
	}
}

func TestMalformedBlockFailsClosed(t *testing.T) {
	for _, input := range []string{"<!-- dtkit:begin engineer project -->\n", "<!-- dtkit:end engineer project -->\n", "<!-- dtkit:begin engineer global -->\n", "\xff"} {
		if _, _, err := AppendBlock([]byte(input), true, projectBlock, "project"); err == nil {
			t.Fatalf("accepted malformed block %q", input)
		}
	}
}

func TestInvalidBlockStateDoesNotRemoveUserBytes(t *testing.T) {
	installed, state, err := AppendBlock([]byte("user content"), true, projectBlock, "project")
	if err != nil {
		t.Fatal(err)
	}
	state.Separator = "user content\n"
	if _, _, err := RemoveBlock(installed, "project", state); err == nil {
		t.Fatal("accepted unsafe separator")
	}
}

func TestMultipleRuntimeBlocksInSameFile(t *testing.T) {
	ocBlock := []byte("<!-- dtkit:begin engineer opencode project -->\noc-v1\n<!-- dtkit:end engineer opencode project -->\n")
	base := []byte("# Team Rules\n\nKeep code simple.\n")

	doc, codexSt, err := AppendBlockForRuntime("codex", "engineer", base, true, projectBlock, "project")
	if err != nil {
		t.Fatal(err)
	}
	doc, ocSt, err := AppendBlockForRuntime("opencode", "engineer", doc, true, ocBlock, "project")
	if err != nil {
		t.Fatal(err)
	}

	ocUpdated := []byte("<!-- dtkit:begin engineer opencode project -->\noc-v2\n<!-- dtkit:end engineer opencode project -->\n")
	doc, ocSt, err = ReplaceBlockForRuntime("opencode", "engineer", doc, ocUpdated, "project", ocSt)
	if err != nil {
		t.Fatal(err)
	}

	doc, delCodex, err := RemoveBlockForRuntime("codex", "engineer", doc, "project", codexSt)
	if err != nil || delCodex {
		t.Fatalf("unexpected codex removal: del=%v err=%v", delCodex, err)
	}
	if !bytes.Contains(doc, []byte("oc-v2")) || !bytes.Contains(doc, []byte("Keep code simple.")) {
		t.Fatalf("opencode or user content lost after codex removal: %q", doc)
	}

	doc, delOC, err := RemoveBlockForRuntime("opencode", "engineer", doc, "project", ocSt)
	if err != nil || delOC {
		t.Fatalf("unexpected opencode removal: del=%v err=%v", delOC, err)
	}
	if !bytes.Equal(doc, base) {
		t.Fatalf("final file differs from base: got %q want %q", doc, base)
	}
}
