package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MohamedElashri/sanad/internal/config"
	"github.com/MohamedElashri/sanad/internal/githubresolver"
	"github.com/MohamedElashri/sanad/internal/metadata"
)

func TestApplyDryRunPrintsDiffWithoutWritingFiles(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("a", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n")
	original := readFileString(t, path)

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--dry-run", "--diff"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("workflow changed during dry-run\nwant:\n%s\ngot:\n%s", original, got)
	}
	if _, err := os.Stat(metadata.DefaultLockfilePath); !os.IsNotExist(err) {
		t.Fatalf("lockfile was written during dry-run, stat err = %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"--- .github/workflows/ci.yml",
		"@@ -1,4 +1,4 @@",
		"-      - uses: actions/checkout@v4",
		"+      - uses: actions/checkout@" + sha + " # sanad: ref=v4",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, text)
		}
	}
}

func TestApplyDryRunHidesDiffByDefault(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("0", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n")

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(out.String(), "--- .github/workflows/ci.yml") || strings.Contains(out.String(), "@@ -") {
		t.Fatalf("default dry-run unexpectedly printed a diff:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Add --diff to show the patch") {
		t.Fatalf("default dry-run did not advertise --diff:\n%s", out.String())
	}
}

func TestApplyLockfileWouldWriteDetectsNoOp(t *testing.T) {
	withTempWorkingDir(t)
	entry := metadata.LockfileEntry{
		File:       ".github/workflows/ci.yml",
		Node:       lockTestNode,
		Owner:      "actions",
		Repo:       "checkout",
		Kind:       "github-action",
		LogicalRef: "v4",
		PinnedSHA:  strings.Repeat("a", 40),
	}
	writeTestLockfile(t, entry)

	wouldWrite, err := applyLockfileWouldWrite([]metadata.LockfileEntry{entry})
	if err != nil {
		t.Fatalf("applyLockfileWouldWrite returned error: %v", err)
	}
	if wouldWrite {
		t.Fatal("applyLockfileWouldWrite = true for an unchanged lockfile")
	}
}

func TestApplyPreviewJSONRemainsValidWhenNothingChanges(t *testing.T) {
	withTempWorkingDir(t)
	writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: ./.github/actions/local\n")

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("apply preview returned error: %v", err)
	}
	var report planReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("apply output is not valid JSON: %v\n%s", err, out.String())
	}
}

func TestApplyWriteJSONRemainsValidAndWritesPRBody(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("7", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner: "actions", Repo: "checkout", Ref: "v4", SHA: sha,
			Kind: githubresolver.KindTag, CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n")
	bodyPath := filepath.Join(t.TempDir(), "sanad-pr-body.md")

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--format", "json", "--write", "--yes", "--pr-body-out", bodyPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("apply write returned error: %v", err)
	}

	var report planReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("apply write output is not valid JSON: %v\n%s", err, out.String())
	}
	if report.Version != planReportVersion || report.Summary.UpdatesAvailable != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if !strings.Contains(readFileString(t, path), "actions/checkout@"+sha) {
		t.Fatal("apply did not rewrite the workflow")
	}
	body := readFileString(t, bodyPath)
	if !strings.Contains(body, "actions/checkout") {
		t.Fatalf("PR body does not describe the update:\n%s", body)
	}
}

func TestApplyYesWriteRewritesWorkflowAndUpdatesLockfile(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("b", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n")

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	got := readFileString(t, path)
	if !strings.Contains(got, "actions/checkout@"+sha+" # sanad: ref=v4") {
		t.Fatalf("workflow was not rewritten as expected:\n%s", got)
	}

	var lockfile metadata.Lockfile
	lockBytes, err := os.ReadFile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}
	if err := json.Unmarshal(lockBytes, &lockfile); err != nil {
		t.Fatalf("lockfile is invalid JSON: %v\n%s", err, string(lockBytes))
	}
	if len(lockfile.Entries) != 1 {
		t.Fatalf("lockfile entries = %d, want 1: %#v", len(lockfile.Entries), lockfile.Entries)
	}
	entry := lockfile.Entries[0]
	if entry.File != ".github/workflows/ci.yml" || entry.Node != "jobs.test.steps[0].uses" {
		t.Fatalf("unexpected lockfile target: %#v", entry)
	}
	if entry.Owner != "actions" || entry.Repo != "checkout" || entry.LogicalRef != "v4" || entry.PinnedSHA != sha {
		t.Fatalf("unexpected lockfile entry: %#v", entry)
	}
	if !strings.Contains(out.String(), "Applied 1 workflow update(s) across 1 file(s).") {
		t.Fatalf("missing apply summary:\n%s", out.String())
	}
}

func TestApplyYesWritePinsUnpinnedLatestRelease(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("8", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@latest-release": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v5",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	if err := os.WriteFile(".sanad.toml", []byte("[updates]\nunpinned = \"latest-release\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout\n")

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	got := readFileString(t, path)
	if !strings.Contains(got, "actions/checkout@"+sha+" # sanad: ref=v5") {
		t.Fatalf("workflow was not rewritten as expected:\n%s", got)
	}
	lockfile, ok, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("LoadLockfile returned error: %v", err)
	}
	if !ok || len(lockfile.Entries) != 1 {
		t.Fatalf("unexpected lockfile state ok=%v entries=%#v", ok, lockfile.Entries)
	}
	if lockfile.Entries[0].LogicalRef != "v5" || lockfile.Entries[0].PinnedSHA != sha {
		t.Fatalf("unexpected lockfile entry: %#v", lockfile.Entries[0])
	}
}

func TestApplyCommentsWriteFalseUsesLockfileWithoutInlineMetadata(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("9", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	if err := os.WriteFile(".sanad.toml", []byte("[comments]\nwrite = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n")

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	got := readFileString(t, path)
	if strings.Contains(got, "sanad: ref=") {
		t.Fatalf("workflow contains inline metadata despite comments.write=false:\n%s", got)
	}
	if !strings.Contains(got, "actions/checkout@"+sha) {
		t.Fatalf("workflow was not rewritten as expected:\n%s", got)
	}

	lockfile, ok, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("LoadLockfile returned error: %v", err)
	}
	if !ok || len(lockfile.Entries) != 1 {
		t.Fatalf("unexpected lockfile state ok=%v entries=%#v", ok, lockfile.Entries)
	}
	if lockfile.Entries[0].LogicalRef != "v4" || lockfile.Entries[0].PinnedSHA != sha {
		t.Fatalf("unexpected lockfile entry: %#v", lockfile.Entries[0])
	}
}

func TestApplyDefaultsToPreviewWithoutWriting(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("c", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n")
	original := readFileString(t, path)

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("preview returned error: %v", err)
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("workflow changed during preview\nwant:\n%s\ngot:\n%s", original, got)
	}
	if _, err := os.Stat(metadata.DefaultLockfilePath); !os.IsNotExist(err) {
		t.Fatalf("lockfile was written during preview, stat err = %v", err)
	}
}

func TestApplyYesWriteUpdatesLockfileWhenWorkflowAlreadyCurrent(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("d", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@"+sha+" # sanad: ref=v4\n")
	original := readFileString(t, path)

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("workflow changed unexpectedly\nwant:\n%s\ngot:\n%s", original, got)
	}

	lockfile, ok, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("LoadLockfile returned error: %v", err)
	}
	if !ok {
		t.Fatal("lockfile was not written")
	}
	if len(lockfile.Entries) != 1 || lockfile.Entries[0].PinnedSHA != sha || lockfile.Entries[0].LogicalRef != "v4" {
		t.Fatalf("unexpected lockfile entries: %#v", lockfile.Entries)
	}
	if !strings.Contains(out.String(), "Updated lockfile; no workflow updates to apply.") {
		t.Fatalf("missing lockfile-only summary:\n%s", out.String())
	}
}

func TestApplyYesWriteRefreshesStaleLockfilePinDrift(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("1", 40)
	lockfileSHA := strings.Repeat("2", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        currentSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@"+currentSHA+" # sanad: ref=v4\n")
	original := readFileString(t, path)
	writeTestLockfile(t, lockTestEntry("actions", "checkout", "v4", lockfileSHA))

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("workflow changed unexpectedly\nwant:\n%s\ngot:\n%s", original, got)
	}

	lockfile, ok, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("LoadLockfile returned error: %v", err)
	}
	if !ok || len(lockfile.Entries) != 1 {
		t.Fatalf("unexpected lockfile state ok=%v entries=%#v", ok, lockfile.Entries)
	}
	if lockfile.Entries[0].PinnedSHA != currentSHA {
		t.Fatalf("PinnedSHA = %q, want %q", lockfile.Entries[0].PinnedSHA, currentSHA)
	}
	if !strings.Contains(out.String(), "Updated lockfile; no workflow updates to apply.") {
		t.Fatalf("missing lockfile-only summary:\n%s", out.String())
	}
}

func TestApplyRetainsManagedPendingPinsInLockfile(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	checkoutSHA := strings.Repeat("e", 40)
	currentSetupSHA := strings.Repeat("1", 40)
	nextSetupSHA := strings.Repeat("2", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        checkoutSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
		"actions/setup-go@v5": {
			Owner:      "actions",
			Repo:       "setup-go",
			Ref:        "v5",
			SHA:        nextSetupSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-2 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)

	path := writeApplyWorkflow(t, strings.Join([]string{
		"jobs:",
		"  test:",
		"    steps:",
		"      - uses: actions/checkout@v4",
		"      - uses: actions/setup-go@" + currentSetupSHA + " # sanad: ref=v5",
		"",
	}, "\n"))

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	got := readFileString(t, path)
	if !strings.Contains(got, "actions/checkout@"+checkoutSHA+" # sanad: ref=v4") {
		t.Fatalf("checkout was not updated:\n%s", got)
	}
	if !strings.Contains(got, "actions/setup-go@"+currentSetupSHA+" # sanad: ref=v5") {
		t.Fatalf("pending managed pin changed unexpectedly:\n%s", got)
	}

	lockfile, ok, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("LoadLockfile returned error: %v", err)
	}
	if !ok {
		t.Fatal("lockfile was not written")
	}
	if len(lockfile.Entries) != 2 {
		t.Fatalf("lockfile entries = %d, want 2: %#v", len(lockfile.Entries), lockfile.Entries)
	}
	for _, entry := range lockfile.Entries {
		if entry.Repo == "setup-go" && entry.PinnedSHA != currentSetupSHA {
			t.Fatalf("pending setup-go pin = %q, want current %q", entry.PinnedSHA, currentSetupSHA)
		}
	}
}

func TestApplyInteractivePinsUnpinnedActionFromExplicitRef(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("f", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	if err := os.WriteFile(".sanad.toml", []byte("[updates]\nunpinned = \"deny\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout\n")

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("e\nv4\ny\n"))
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--interactive"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	got := readFileString(t, path)
	if !strings.Contains(got, "actions/checkout@"+sha+" # sanad: ref=v4") {
		t.Fatalf("workflow was not rewritten as expected:\n%s", got)
	}
	if !strings.Contains(out.String(), "Found unpinned action") {
		t.Fatalf("interactive prompt missing from output:\n%s", out.String())
	}
}

func TestApplyInteractiveTracksLogicalRefForUnmanagedPinnedSHA(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("1", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	if err := os.WriteFile(".sanad.toml", []byte("[updates]\nunpinned = \"deny\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@"+sha+"\n")
	original := readFileString(t, path)

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("t\nv4\ny\n"))
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--interactive"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("workflow changed unexpectedly\nwant:\n%s\ngot:\n%s", original, got)
	}
	lockfile, ok, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("LoadLockfile returned error: %v", err)
	}
	if !ok || len(lockfile.Entries) != 1 {
		t.Fatalf("unexpected lockfile state ok=%v entries=%#v", ok, lockfile.Entries)
	}
	if lockfile.Entries[0].LogicalRef != "v4" || lockfile.Entries[0].PinnedSHA != sha {
		t.Fatalf("unexpected lockfile entry: %#v", lockfile.Entries[0])
	}
	if !strings.Contains(out.String(), "Found unmanaged pinned action") {
		t.Fatalf("interactive prompt missing from output:\n%s", out.String())
	}
}

func TestApplyInteractivePinsDeniedBranchHead(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("6", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"owner/repo@main": {
			Owner:      "owner",
			Repo:       "repo",
			Ref:        "main",
			SHA:        sha,
			Kind:       githubresolver.KindBranch,
			CommitTime: now.Add(-1 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	if err := os.WriteFile(".sanad.toml", []byte("[updates]\nbranches = \"deny\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: owner/repo@main\n")

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("p\nn\ny\n"))
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--interactive"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	got := readFileString(t, path)
	if !strings.Contains(got, "owner/repo@"+sha+" # sanad: ref=main") {
		t.Fatalf("workflow was not rewritten as expected:\n%s", got)
	}
	if !strings.Contains(out.String(), "Found branch ref") {
		t.Fatalf("branch prompt missing from output:\n%s", out.String())
	}
}

func TestApplyInteractivePersistsBranchTrackingWhenRequested(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("6", 40)
	nextSHA := strings.Repeat("7", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"owner/repo@main": {
			Owner:      "owner",
			Repo:       "repo",
			Ref:        "main",
			SHA:        sha,
			Kind:       githubresolver.KindBranch,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	if err := os.WriteFile(".sanad.toml", []byte("[updates]\nbranches = \"deny\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: owner/repo@main\n")

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("p\ny\ny\n"))
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--interactive"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	configText := readFileString(t, config.DefaultPath)
	if !strings.Contains(configText, `branches = "track"`) {
		t.Fatalf("config did not persist branch tracking:\n%s", configText)
	}

	installPlanTestResolver(t, fakePlanResolver{
		"owner/repo@main": {
			Owner:      "owner",
			Repo:       "repo",
			Ref:        "main",
			SHA:        nextSHA,
			Kind:       githubresolver.KindBranch,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	cmd = NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("non-interactive apply after persistence returned error: %v", err)
	}
	got := readFileString(t, path)
	if !strings.Contains(got, "owner/repo@"+nextSHA+" # sanad: ref=main") {
		t.Fatalf("workflow was not updated by persisted branch policy:\n%s", got)
	}
}

func TestApplyFirstSeenCooldownRecordsPendingCandidate(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("6", 40)
	nextSHA := strings.Repeat("7", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"owner/repo@main": {
			Owner:      "owner",
			Repo:       "repo",
			Ref:        "main",
			SHA:        nextSHA,
			Kind:       githubresolver.KindBranch,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	if err := os.WriteFile(".sanad.toml", []byte("cooldown_source = \"first-seen\"\n\n[updates]\nbranches = \"track\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: owner/repo@"+currentSHA+" # sanad: ref=main\n")

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}
	got := readFileString(t, path)
	if !strings.Contains(got, "owner/repo@"+currentSHA+" # sanad: ref=main") {
		t.Fatalf("workflow changed before first-seen cooldown elapsed:\n%s", got)
	}
	lockfile, ok, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("LoadLockfile returned error: %v", err)
	}
	if !ok || len(lockfile.Entries) != 1 || len(lockfile.Entries[0].Candidates) != 1 || lockfile.Entries[0].Candidates[0].SHA != nextSHA {
		t.Fatalf("pending candidate was not recorded: ok=%v entries=%#v", ok, lockfile.Entries)
	}
}

func TestApplyPreservesFutureUpgradeCandidateHistory(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("8", 40)
	futureSHA := strings.Repeat("9", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        currentSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-30 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@"+currentSHA+" # sanad: ref=v4\n")
	writeTestLockfile(t, metadata.LockfileEntry{
		File:       ".github/workflows/ci.yml",
		Node:       "jobs.test.steps[0].uses",
		Owner:      "actions",
		Repo:       "checkout",
		Kind:       "github-action",
		LogicalRef: "v4",
		PinnedSHA:  currentSHA,
		Candidates: []metadata.CandidateHistoryEntry{{LogicalRef: "v5.0.0", SHA: futureSHA, SeenAt: now.Format(time.RFC3339)}},
	})

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--yes", "--write"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	lockfile, ok, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil || !ok || len(lockfile.Entries) != 1 {
		t.Fatalf("unexpected lockfile: ok=%v err=%v entries=%#v", ok, err, lockfile.Entries)
	}
	if len(lockfile.Entries[0].Candidates) != 1 || lockfile.Entries[0].Candidates[0].LogicalRef != "v5.0.0" {
		t.Fatalf("future candidate history was not preserved: %#v", lockfile.Entries[0])
	}
}

func TestApplyScopedWorkflowPreservesOutOfScopeLockEntries(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("a", 40)
	nextSHA := strings.Repeat("b", 40)
	otherSHA := strings.Repeat("c", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        nextSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-30 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@"+currentSHA+" # sanad: ref=v4\n")
	otherPath := filepath.Join(".github", "workflows", "other.yml")
	if err := os.WriteFile(otherPath, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/setup-go@"+otherSHA+" # sanad: ref=v5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestLockfile(t,
		metadata.LockfileEntry{File: ".github/workflows/ci.yml", Node: lockTestNode, Owner: "actions", Repo: "checkout", Kind: "github-action", LogicalRef: "v4", PinnedSHA: currentSHA},
		metadata.LockfileEntry{File: filepath.ToSlash(otherPath), Node: lockTestNode, Owner: "actions", Repo: "setup-go", Kind: "github-action", LogicalRef: "v5", PinnedSHA: otherSHA},
	)

	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"apply", "--workflows", ".github/workflows/ci.yml", "--yes", "--write"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	lockfile, _, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		t.Fatalf("LoadLockfile returned error: %v", err)
	}
	if len(lockfile.Entries) != 2 {
		t.Fatalf("scoped apply removed out-of-scope entries: %#v", lockfile.Entries)
	}
	for _, entry := range lockfile.Entries {
		if entry.File == filepath.ToSlash(otherPath) && entry.PinnedSHA != otherSHA {
			t.Fatalf("scoped apply changed out-of-scope entry: %#v", entry)
		}
	}
}

func TestAuditCommandsNeverModifyLockfile(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("d", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        sha,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-30 * 24 * time.Hour),
		},
	}, now)
	withTempWorkingDir(t)
	writeApplyWorkflow(t, "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@"+sha+" # sanad: ref=v4\n")
	writeTestLockfile(t, metadata.LockfileEntry{File: ".github/workflows/ci.yml", Node: lockTestNode, Owner: "actions", Repo: "checkout", Kind: "github-action", LogicalRef: "v4", PinnedSHA: sha})
	original := readFileString(t, metadata.DefaultLockfilePath)

	for _, args := range [][]string{{"scan"}, {"plan"}, {"check"}} {
		cmd := NewRootCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v returned error: %v", args, err)
		}
		if got := readFileString(t, metadata.DefaultLockfilePath); got != original {
			t.Fatalf("%v modified the lockfile", args)
		}
	}
}

func withTempWorkingDir(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousWorkingDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	return root
}

func writeApplyWorkflow(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFileString(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
