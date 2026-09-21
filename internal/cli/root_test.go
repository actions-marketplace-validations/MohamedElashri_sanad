package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootHelpShowsCanonicalTopLevelCommands(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"\n  start",
		"\n  scan",
		"\n  plan",
		"\n  check",
		"\n  apply",
		"\n  upgrade",
		"\n  lock",
		"\n  doctor",
		"\n  config",
		"\n  completion",
		"\n  version",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	for _, removed := range []string{
		"\n  audit",
		"\n  update",
	} {
		if strings.Contains(help, removed) {
			t.Fatalf("root help includes removed namespace %q:\n%s", removed, help)
		}
	}
}

func TestScanExecutes(t *testing.T) {
	workflows := filepath.Join(t.TempDir(), ".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--format", "json", "scan", "--workflows", workflows})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	var got []scanEntry
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("audit scan output is not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].Raw != "actions/checkout@v4" {
		t.Fatalf("unexpected audit scan output: %#v", got)
	}
}

func TestApplyExecutes(t *testing.T) {
	withTempWorkingDir(t)
	writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: ./.github/actions/local\n")

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out.String(), "No workflow updates to apply.") {
		t.Fatalf("unexpected update apply output:\n%s", out.String())
	}
}

func TestBareCommandStartsWithoutConfig(t *testing.T) {
	withTempWorkingDir(t)
	writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: ./.github/actions/local\n")
	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bare sanad returned error: %v", err)
	}
	if !strings.Contains(out.String(), "No workflow updates to apply.") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRuntimeCompletionIncludesFlatCommands(t *testing.T) {
	rootCompletion := completeCommand(t, "")
	for _, want := range []string{"start\t", "scan\t", "plan\t", "check\t", "apply\t", "upgrade\t", "lock\t", "doctor\t", "config\t", "completion\t", "version\t"} {
		if !strings.Contains(rootCompletion, want) {
			t.Fatalf("root completion missing %q:\n%s", want, rootCompletion)
		}
	}
	for _, removed := range []string{"audit\t", "update\t"} {
		if strings.Contains(rootCompletion, removed) {
			t.Fatalf("root completion includes removed namespace %q:\n%s", removed, rootCompletion)
		}
	}
}

func completeCommand(t *testing.T, args ...string) string {
	t.Helper()

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"__complete"}, args...))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	return out.String()
}
