package claude_test

import (
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/claude"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestClaudeRenderProjectAndGlobal(t *testing.T) {
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"project", "global"} {
		arts, err := claude.Render(pkg, src, scope)
		if err != nil {
			t.Fatalf("claude.Render(%s) failed: %v", scope, err)
		}
		var sawCLAUDE, sawSkill, sawRule bool
		for _, a := range arts {
			if a.Path == "CLAUDE.md" && a.Strategy == "managed-block" {
				sawCLAUDE = true
				if !strings.Contains(string(a.Content), "<!-- dtkit:begin engineer claude "+scope+" -->") {
					t.Fatalf("unexpected CLAUDE.md tag:\n%s", string(a.Content))
				}
			}
			if strings.HasSuffix(a.Path, "/SKILL.md") && strings.Contains(a.Path, "dtkit-claude-"+scope+"-") {
				sawSkill = true
			}
			if strings.Contains(a.Path, ".dtkit/runtimes/claude/content/engineer/") {
				sawRule = true
			}
		}
		if !sawCLAUDE || !sawSkill || !sawRule {
			t.Fatalf("%s render missing expected artifacts (claude=%v skill=%v rule=%v)", scope, sawCLAUDE, sawSkill, sawRule)
		}
	}
}
