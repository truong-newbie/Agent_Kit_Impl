package hooks

import "testing"

func FuzzHooksJSON(f *testing.F) {
	f.Add([]byte(`{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"echo hi"}]}]}}`))
	f.Add([]byte(`{"hooks":{},"hooks":{}}`))
	f.Add([]byte(`{"hooks":{"SessionStart":["bad"]}}`))
	f.Add([]byte(`{"custom":9007199254740993}`))

	entry := BuildSessionContextEntry("'/usr/local/bin/dtkit' hook run session-context", "", DefaultTimeoutSec)
	f.Fuzz(func(t *testing.T, data []byte) {
		if merged, digest, err := MergeHooksJSON(data, true, entry); err == nil {
			_, _ = InspectHooksJSON(merged, digest)
			_, _, _ = UnmergeHooksJSON(merged, digest, false)
		}
		_, _ = InspectHooksJSON(data, "0000000000000000000000000000000000000000000000000000000000000000")
		_, _, _ = UnmergeHooksJSON(data, "0000000000000000000000000000000000000000000000000000000000000000", false)
	})
}
