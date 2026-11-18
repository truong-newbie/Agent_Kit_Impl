package guard

import (
	"fmt"
	"os/exec"
	"strings"
)

// TamperFinding describes a suspicious weakening of test suites detected in a working tree diff.
type TamperFinding struct {
	Path   string `json:"path"`
	Rule   string `json:"rule"` // "added_skip" | "removed_test_case" | "reduced_assertions"
	Detail string `json:"detail"`
}

var skipTokens = []string{
	"t.Skip(", "t.Skipf(", "t.SkipNow(",
	".skip(", "pytest.skip(", "@pytest.mark.skip", "unittest.skip",
	"@Disabled", "@Ignore", "GTEST_SKIP(", "#[ignore]", "[Ignore",
}

var testDeclTokens = []string{
	"func Test", "def test_", "@Test", "it(", "test(", "TEST(", "TEST_F(",
}

var assertionTokens = []string{
	"t.Fatal", "t.Fatalf", "t.Error", "t.Errorf",
	"assert", "require.", "expect(", "ASSERT_", "EXPECT_", "assertEquals", "assertThat",
}

// DetectTestTampering inspects git diff HEAD for modified test files and flags skips, removed tests, or dropped assertions.
func DetectTestTampering(repoRoot string) ([]TamperFinding, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil
	}
	chk := exec.Command("git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree")
	if out, err := chk.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil, nil
	}
	// If HEAD does not exist (brand-new repo with zero commits), no baseline diff exists.
	if err := exec.Command("git", "-C", repoRoot, "rev-parse", "--verify", "HEAD").Run(); err != nil {
		return nil, nil
	}

	diffCmd := exec.Command("git", "-C", repoRoot, "diff", "--no-color", "-U0", "HEAD")
	out, err := diffCmd.Output()
	if err != nil {
		return nil, nil
	}
	return AnalyzeUnifiedDiff(string(out)), nil
}

// AnalyzeUnifiedDiff scans a git unified diff across files and reports test-weakening findings.
func AnalyzeUnifiedDiff(unifiedDiff string) []TamperFinding {
	var findings []TamperFinding
	var curFile string
	var fileLines []string

	flush := func() {
		if curFile != "" && IsTestFile(curFile) && len(fileLines) > 0 {
			findings = append(findings, AnalyzeTestDiff(curFile, strings.Join(fileLines, "\n"))...)
		}
		curFile = ""
		fileLines = nil
	}

	for _, line := range strings.Split(strings.ReplaceAll(unifiedDiff, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			continue
		}
		if strings.HasPrefix(line, "+++ b/") {
			curFile = strings.TrimSpace(strings.TrimPrefix(line, "+++ b/"))
			continue
		}
		if strings.HasPrefix(line, "+++ ") && strings.TrimSpace(strings.TrimPrefix(line, "+++ ")) == "/dev/null" {
			// Entire test file deleted
			if curFile == "" {
				continue
			}
		}
		if curFile != "" {
			fileLines = append(fileLines, line)
		}
	}
	flush()
	return findings
}

// AnalyzeTestDiff checks a single test file's diff lines for tampering heuristics.
func AnalyzeTestDiff(relPath, diffBody string) []TamperFinding {
	var findings []TamperFinding
	var addedSkip int
	var addedTests, removedTests int
	var addedAsserts, removedAsserts int

	for _, rawLine := range strings.Split(strings.ReplaceAll(diffBody, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(rawLine, "+++") || strings.HasPrefix(rawLine, "---") {
			continue
		}
		isAdd := strings.HasPrefix(rawLine, "+")
		isDel := strings.HasPrefix(rawLine, "-")
		if !isAdd && !isDel {
			continue
		}
		body := rawLine[1:]

		if isAdd {
			for _, tok := range skipTokens {
				if strings.Contains(body, tok) {
					addedSkip++
					break
				}
			}
			addedTests += countAny(body, testDeclTokens)
			addedAsserts += countAny(body, assertionTokens)
		} else {
			removedTests += countAny(body, testDeclTokens)
			removedAsserts += countAny(body, assertionTokens)
		}
	}

	if addedSkip > 0 {
		findings = append(findings, TamperFinding{
			Path:   relPath,
			Rule:   "added_skip",
			Detail: fmt.Sprintf("added %d test skip directive(s) in %s", addedSkip, relPath),
		})
	}
	if removedTests > addedTests {
		findings = append(findings, TamperFinding{
			Path:   relPath,
			Rule:   "removed_test_case",
			Detail: fmt.Sprintf("net removal of %d test case(s) in %s", removedTests-addedTests, relPath),
		})
	}
	if removedAsserts > addedAsserts {
		findings = append(findings, TamperFinding{
			Path:   relPath,
			Rule:   "reduced_assertions",
			Detail: fmt.Sprintf("net reduction of %d assertion(s) in %s", removedAsserts-addedAsserts, relPath),
		})
	}
	return findings
}

func countAny(line string, tokens []string) int {
	for _, tok := range tokens {
		if strings.Contains(line, tok) {
			return 1
		}
	}
	return 0
}
