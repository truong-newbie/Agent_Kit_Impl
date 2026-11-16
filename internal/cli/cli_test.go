package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestListAndInfo(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"info", "engineer"}} {
		var out, stderr bytes.Buffer
		if err := Run(args, &out, &stderr); err != nil { t.Fatalf("%v: %v", args, err) }
		if !strings.Contains(out.String(), "engineer") { t.Fatalf("missing engineer: %s", out.String()) }
	}
}

func TestUnsupportedInstallDoesNotClaimSuccess(t *testing.T) {
	var out, stderr bytes.Buffer
	if err := Run([]string{"install", "engineer", "--project"}, &out, &stderr); err == nil { t.Fatal("expected unsupported command error") }
}
