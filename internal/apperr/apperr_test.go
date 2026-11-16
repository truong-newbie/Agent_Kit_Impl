package apperr

import (
	"context"
	"errors"
	"testing"
)

func TestExitCodes(t *testing.T) {
	if got := ExitCodeOf(nil); got != ExitOK {
		t.Fatalf("nil = %d", got)
	}
	if got := ExitCodeOf(errors.New("plain")); got != ExitInternal {
		t.Fatalf("plain = %d", got)
	}
	if got := ExitCodeOf(Usage("bad", "bad")); got != ExitUsage {
		t.Fatalf("usage = %d", got)
	}
	if got := ExitCodeOf(Conflict("drift", "drift")); got != ExitConflict {
		t.Fatalf("conflict = %d", got)
	}
	if got := ExitCodeOf(Runtime("missing", "missing")); got != ExitRuntime {
		t.Fatalf("runtime = %d", got)
	}
	if got := ExitCodeOf(Cancelled("stop")); got != ExitCancelled {
		t.Fatalf("cancelled = %d", got)
	}
	if got := ExitCodeOf(context.Canceled); got != ExitCancelled {
		t.Fatalf("context.Canceled = %d", got)
	}
}
