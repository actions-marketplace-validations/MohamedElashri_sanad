package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MohamedElashri/sanad/internal/config"
	"github.com/MohamedElashri/sanad/internal/githubresolver"
)

func TestPlanTableShowsDecisionsWithoutModifyingWorkflows(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	old := now.Add(-15 * 24 * time.Hour)
	recent := now.Add(-2 * 24 * time.Hour)
	checkoutSHA := strings.Repeat("1", 40)
	setupGoSHA := strings.Repeat("2", 40)
	branchSHA := strings.Repeat("3", 40)

	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        checkoutSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: old,
		},
		"actions/setup-go@v5": {
			Owner:      "actions",
			Repo:       "setup-go",
			Ref:        "v5",
			SHA:        setupGoSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: recent,
		},
		"owner/repo@main": {
			Owner:      "owner",
			Repo:       "repo",
			Ref:        "main",
			SHA:        branchSHA,
			Kind:       githubresolver.KindBranch,
			CommitTime: old,
		},
	}, now)

	workflows := filepath.Join(t.TempDir(), ".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	original := strings.Join([]string{
		"jobs:",
		"  test:",
		"    steps:",
		"      - uses: actions/checkout@v4",
		"      - uses: actions/setup-go@v5",
		"      - uses: owner/repo@main",
		"      - uses: owner/unpinned",
		"      - uses: docker://alpine:3.20",
		"      - uses: ./.github/actions/local",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"plan", "--workflows", workflows})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	gotWorkflow, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotWorkflow) != original {
		t.Fatalf("workflow file was modified:\n%s", string(gotWorkflow))
	}

	text := out.String()
	for _, want := range []string{
		"Summary:",
		"6 actions found",
		"2 updates available",
		"1 pending cooldown",
		"1 policy violations",
		"2 skipped",
		"FILE",
		"ACTION",
		"CURRENT",
		"CANDIDATE",
		"DECISION",
		"REASON CODE",
		"actions/checkout",
		"111111111111",
		"update",
		"update-available",
		"actions/setup-go",
		"pending-cooldown",
		"cooldown-active",
		"owner/repo",
		"update",
		"update-available",
		"owner/unpinned",
		"error-unresolved",
		"alpine:3.20",
		"skip-docker-action",
		"./.github/actions/local",
		"skip-local-action",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("plan table missing %q:\n%s", want, text)
		}
	}
}

func TestPlanJSONOutputAndOutFile(t *testing.T) {
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

	workflows := filepath.Join(t.TempDir(), ".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(t.TempDir(), "sanad-plan.json")
	var stdout bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--format", "json", "plan", "--workflows", workflows, "--out", outPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	fileBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read plan output: %v", err)
	}
	if stdout.String() != string(fileBytes) {
		t.Fatalf("stdout JSON and --out JSON differ\nstdout:\n%s\nfile:\n%s", stdout.String(), string(fileBytes))
	}

	var report planReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("plan output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if report.Version != 1 {
		t.Fatalf("Version = %d, want 1", report.Version)
	}
	if report.Summary.Actions != 1 || report.Summary.UpdatesAvailable != 1 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}
	if len(report.Files) != 1 || len(report.Files[0].Actions) != 1 {
		t.Fatalf("unexpected report shape: %#v", report)
	}
	action := report.Files[0].Actions[0]
	if action.Raw != "actions/checkout@v4" {
		t.Fatalf("Raw = %q, want actions/checkout@v4", action.Raw)
	}
	if action.CandidateSHA != sha {
		t.Fatalf("CandidateSHA = %q, want %q", action.CandidateSHA, sha)
	}
	if action.CandidateRefKind != string(githubresolver.KindTag) {
		t.Fatalf("CandidateRefKind = %q, want tag", action.CandidateRefKind)
	}
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update", action.Decision)
	}
	if action.ReasonCode != "update-available" {
		t.Fatalf("ReasonCode = %q, want update-available", action.ReasonCode)
	}
	if action.AgeSeconds != int64((15*24*time.Hour)/time.Second) {
		t.Fatalf("AgeSeconds = %d, want %d", action.AgeSeconds, int64((15*24*time.Hour)/time.Second))
	}
}

func TestPlanDiscoversDefaultBranchForUnpinnedActions(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("7", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@default-branch": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "main",
			SHA:        sha,
			Kind:       githubresolver.KindBranch,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)

	withTempWorkingDir(t)
	workflows := filepath.Join(".github", "workflows")
	if err := os.WriteFile(".sanad.toml", []byte("[updates]\nunpinned = \"default-branch\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflows, "ci.yml"), []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update (%s)", action.Decision, action.Reason)
	}
	if action.LogicalRef != "main" {
		t.Fatalf("LogicalRef = %q, want main", action.LogicalRef)
	}
	if action.CandidateSHA != sha || action.CandidateRefKind != string(githubresolver.KindBranch) {
		t.Fatalf("unexpected candidate: sha=%q kind=%q", action.CandidateSHA, action.CandidateRefKind)
	}
}

func TestPlanDiscoversLatestReleaseForUnpinnedActions(t *testing.T) {
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
	workflows := filepath.Join(".github", "workflows")
	if err := os.WriteFile(".sanad.toml", []byte("[updates]\nunpinned = \"latest-release\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflows, "ci.yml"), []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update (%s)", action.Decision, action.Reason)
	}
	if action.LogicalRef != "v5" {
		t.Fatalf("LogicalRef = %q, want v5", action.LogicalRef)
	}
	if action.CandidateSHA != sha || action.CandidateRefKind != string(githubresolver.KindTag) {
		t.Fatalf("unexpected candidate: sha=%q kind=%q", action.CandidateSHA, action.CandidateRefKind)
	}
}

func TestPlanWritesPullRequestBody(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("1", 40)
	nextSHA := strings.Repeat("2", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        nextSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)

	workflows := filepath.Join(t.TempDir(), ".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + currentSHA + " # sanad: ref=v4\n      - uses: owner/unpinned\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	bodyPath := filepath.Join(t.TempDir(), "sanad-pr-body.md")
	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"plan", "--workflows", workflows, "--pr-body-out", bodyPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	body, err := os.ReadFile(bodyPath)
	if err != nil {
		t.Fatalf("read PR body: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"# Update pinned GitHub Actions",
		"- Actions found: 2",
		"- Updates available: 1",
		"- Policy violations: 1",
		"## Updates",
		"actions/checkout",
		"111111111111",
		"222222222222",
		"## Policy Violations",
		"owner/unpinned",
		"error-unresolved",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("PR body missing %q:\n%s", want, text)
		}
	}
}

func TestPlanRecoversLogicalRefFromInlineComment(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("1", 40)
	nextSHA := strings.Repeat("2", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        nextSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)

	workflows := filepath.Join(t.TempDir(), ".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + currentSHA + " # sanad: ref=v4\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.CurrentSHA != currentSHA {
		t.Fatalf("CurrentSHA = %q, want %q", action.CurrentSHA, currentSHA)
	}
	if action.CandidateSHA != nextSHA {
		t.Fatalf("CandidateSHA = %q, want %q", action.CandidateSHA, nextSHA)
	}
	if action.LogicalRef != "v4" {
		t.Fatalf("LogicalRef = %q, want v4", action.LogicalRef)
	}
	if action.MetadataSource != "comment" {
		t.Fatalf("MetadataSource = %q, want comment", action.MetadataSource)
	}
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update", action.Decision)
	}
}

func TestPlanFollowsUpdatedLogicalRefCommentForRenovateInterop(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("1", 40)
	nextSHA := strings.Repeat("5", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v5": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v5",
			SHA:        nextSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)

	workflows := filepath.Join(t.TempDir(), ".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + currentSHA + " # sanad: ref=v5\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.LogicalRef != "v5" {
		t.Fatalf("LogicalRef = %q, want v5", action.LogicalRef)
	}
	if action.CandidateSHA != nextSHA {
		t.Fatalf("CandidateSHA = %q, want %q", action.CandidateSHA, nextSHA)
	}
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update", action.Decision)
	}
}

func TestPlanPinnedSHAWithoutMetadataIsUnmanaged(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("1", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@latest-release": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v5.0.0",
			SHA:        strings.Repeat("5", 40),
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)

	workflows := filepath.Join(t.TempDir(), ".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + currentSHA + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update", action.Decision)
	}
	if action.CurrentSHA != currentSHA {
		t.Fatalf("CurrentSHA = %q, want %q", action.CurrentSHA, currentSHA)
	}
	if action.LogicalRef != "v5.0.0" {
		t.Fatalf("LogicalRef = %q, want v5.0.0", action.LogicalRef)
	}
}

func TestPlanUsesInlineCommentWhenLockfileRefDrifts(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("1", 40)
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

	workflows := filepath.Join(".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + currentSHA + " # sanad: ref=v4\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lockfile := `{
  "version": 1,
  "entries": [
    {
      "file": ".github/workflows/ci.yml",
      "node": "jobs.test.steps[0].uses",
      "owner": "actions",
      "repo": "checkout",
      "kind": "github-action",
      "logical_ref": "v5",
      "pinned_sha": "` + currentSHA + `"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(".github", "sanad.lock.json"), []byte(lockfile), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.Decision != "unchanged" {
		t.Fatalf("Decision = %q, want unchanged", action.Decision)
	}
	if action.LogicalRef != "v4" || action.MetadataSource != "comment" {
		t.Fatalf("unexpected reconciled metadata: ref=%q source=%q", action.LogicalRef, action.MetadataSource)
	}
}

func TestPlanTreatsActionMismatchWithoutCommentAsUnmanaged(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("1", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/setup-go@latest-release": {
			Owner:      "actions",
			Repo:       "setup-go",
			Ref:        "v5.0.0",
			SHA:        strings.Repeat("5", 40),
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)

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

	workflows := filepath.Join(".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/setup-go@" + currentSHA + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lockfile := `{
  "version": 1,
  "entries": [
    {
      "file": ".github/workflows/ci.yml",
      "node": "jobs.test.steps[0].uses",
      "owner": "actions",
      "repo": "checkout",
      "kind": "github-action",
      "logical_ref": "v4",
      "pinned_sha": "` + currentSHA + `"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(".github", "sanad.lock.json"), []byte(lockfile), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update", action.Decision)
	}
	if action.LogicalRef != "v5.0.0" || action.MetadataSource != "" {
		t.Fatalf("action mismatch without inline metadata should be unmanaged, got ref=%q source=%q", action.LogicalRef, action.MetadataSource)
	}
}

func TestPlanUsesWorkflowPinWhenLockfilePinDrifts(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	lockfileSHA := strings.Repeat("1", 40)
	workflowSHA := strings.Repeat("2", 40)
	installPlanTestResolver(t, fakePlanResolver{
		"actions/checkout@v4": {
			Owner:      "actions",
			Repo:       "checkout",
			Ref:        "v4",
			SHA:        workflowSHA,
			Kind:       githubresolver.KindTag,
			CommitTime: now.Add(-15 * 24 * time.Hour),
		},
	}, now)

	withTempWorkingDir(t)
	workflows := filepath.Join(".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + workflowSHA + " # sanad: ref=v4\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lockfile := `{
  "version": 1,
  "entries": [
    {
      "file": ".github/workflows/ci.yml",
      "node": "jobs.test.steps[0].uses",
      "owner": "actions",
      "repo": "checkout",
      "kind": "github-action",
      "logical_ref": "v4",
      "pinned_sha": "` + lockfileSHA + `"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(".github", "sanad.lock.json"), []byte(lockfile), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.Decision != "unchanged" {
		t.Fatalf("Decision = %q, want unchanged", action.Decision)
	}
	if action.CurrentSHA != workflowSHA || action.LogicalRef != "v4" {
		t.Fatalf("unexpected action after pin drift reconciliation: %#v", action)
	}
}

func TestPlanUsesLockfileCandidateFirstSeenForCooldown(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	currentSHA := strings.Repeat("1", 40)
	nextSHA := strings.Repeat("2", 40)
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
	if err := os.WriteFile(".sanad.toml", []byte(`cooldown_source = "first-seen"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workflows := filepath.Join(".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + currentSHA + " # sanad: ref=v4\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lockfile := `{
  "version": 1,
  "entries": [
    {
      "file": ".github/workflows/ci.yml",
      "node": "jobs.test.steps[0].uses",
      "owner": "actions",
      "repo": "checkout",
      "kind": "github-action",
      "logical_ref": "v4",
      "pinned_sha": "` + currentSHA + `",
      "candidate_sha": "` + nextSHA + `",
      "candidate_seen_at": "` + now.Add(-15*24*time.Hour).Format(time.RFC3339) + `"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(".github", "sanad.lock.json"), []byte(lockfile), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update (%s)", action.Decision, action.Reason)
	}
	if action.AgeSeconds != int64((15*24*time.Hour)/time.Second) {
		t.Fatalf("AgeSeconds = %d, want first-seen age", action.AgeSeconds)
	}
}

func TestPlanPreservesFirstSeenAcrossLockfilePinDrift(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	lockfileSHA := strings.Repeat("1", 40)
	workflowSHA := strings.Repeat("2", 40)
	nextSHA := strings.Repeat("3", 40)
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
	if err := os.WriteFile(".sanad.toml", []byte(`cooldown_source = "first-seen"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workflows := filepath.Join(".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  test:\n    steps:\n      - uses: actions/checkout@" + workflowSHA + " # sanad: ref=v4\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lockfile := `{
  "version": 1,
  "entries": [
    {
      "file": ".github/workflows/ci.yml",
      "node": "jobs.test.steps[0].uses",
      "owner": "actions",
      "repo": "checkout",
      "kind": "github-action",
      "logical_ref": "v4",
      "pinned_sha": "` + lockfileSHA + `",
      "candidate_sha": "` + nextSHA + `",
      "candidate_seen_at": "` + now.Add(-15*24*time.Hour).Format(time.RFC3339) + `"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(".github", "sanad.lock.json"), []byte(lockfile), 0o600); err != nil {
		t.Fatal(err)
	}

	report := executePlanJSON(t, workflows)
	action := report.Files[0].Actions[0]
	if action.Decision != "update" {
		t.Fatalf("Decision = %q, want update (%s)", action.Decision, action.Reason)
	}
	if action.AgeSeconds != int64((15*24*time.Hour)/time.Second) {
		t.Fatalf("AgeSeconds = %d, want first-seen age preserved across pin drift", action.AgeSeconds)
	}
}

func TestPlanSkipsIgnoredActionsWithoutResolving(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	installPlanTestResolver(t, failingResolver{}, now)

	cfg := config.Default()
	cfg.Ignore.Actions = []string{"actions/checkout@v4"}

	workflows := filepath.Join(t.TempDir(), ".github", "workflows")
	path := filepath.Join(workflows, "ci.yml")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v4\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := buildPlanReport(context.Background(), cfg, []string{workflows}, defaultPlanResolver, now)
	if err != nil {
		t.Fatalf("buildPlanReport returned error: %v", err)
	}
	if report.Summary.Skipped != 1 || report.Summary.PolicyViolations != 0 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}
	action := report.Files[0].Actions[0]
	if action.Decision != "skip-ignored" {
		t.Fatalf("Decision = %q, want skip-ignored", action.Decision)
	}
	if action.ReasonCode != "ignored-action" {
		t.Fatalf("ReasonCode = %q, want ignored-action", action.ReasonCode)
	}
	if !strings.Contains(action.Reason, `actions/checkout@v4`) {
		t.Fatalf("Reason = %q, want ignore rule", action.Reason)
	}
}

type fakePlanResolver map[string]githubresolver.ResolvedRef

func (r fakePlanResolver) Resolve(_ context.Context, selector githubresolver.ActionSelector) (githubresolver.ResolvedRef, error) {
	key := selector.Owner + "/" + selector.Repo + "@" + selector.Ref
	resolved, ok := r[key]
	if !ok {
		if latest, latestOK := r[selector.Owner+"/"+selector.Repo+"@latest-release"]; latestOK && latest.Ref == selector.Ref {
			return latest, nil
		}
	}
	if !ok {
		return githubresolver.ResolvedRef{}, fmt.Errorf("unexpected resolve %s", key)
	}
	return resolved, nil
}

func (r fakePlanResolver) ListReleases(_ context.Context, owner, repo string) ([]githubresolver.Release, error) {
	prefix := owner + "/" + repo + "@"
	seen := make(map[string]struct{})
	var releases []githubresolver.Release
	for key, resolved := range r {
		if !strings.HasPrefix(key, prefix) || resolved.Kind != githubresolver.KindTag {
			continue
		}
		tag := resolved.Ref
		if tag == "" || tag == "latest-release" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		published := resolved.CommitTime
		if resolved.ReleaseTime != nil {
			published = *resolved.ReleaseTime
		}
		releases = append(releases, githubresolver.Release{TagName: tag, PublishedAt: published})
	}
	return releases, nil
}

func (r fakePlanResolver) ResolveDefaultBranch(ctx context.Context, owner, repo string) (githubresolver.ResolvedRef, error) {
	return r.Resolve(ctx, githubresolver.ActionSelector{Owner: owner, Repo: repo, Ref: "default-branch"})
}

func (r fakePlanResolver) ResolveLatestRelease(ctx context.Context, owner, repo string) (githubresolver.ResolvedRef, error) {
	return r.Resolve(ctx, githubresolver.ActionSelector{Owner: owner, Repo: repo, Ref: "latest-release"})
}

type failingResolver struct{}

func (failingResolver) Resolve(_ context.Context, selector githubresolver.ActionSelector) (githubresolver.ResolvedRef, error) {
	return githubresolver.ResolvedRef{}, fmt.Errorf("resolver should not be called for %s/%s@%s", selector.Owner, selector.Repo, selector.Ref)
}

func installPlanTestResolver(t *testing.T, resolver planResolver, now time.Time) {
	t.Helper()

	previousResolver := defaultPlanResolver
	previousNow := planNow
	defaultPlanResolver = resolver
	planNow = func() time.Time { return now }
	t.Cleanup(func() {
		defaultPlanResolver = previousResolver
		planNow = previousNow
	})
}

func executePlanJSON(t *testing.T, workflows string) planReport {
	t.Helper()

	var stdout bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--format", "json", "plan", "--workflows", workflows})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	var report planReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("plan output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(report.Files) != 1 || len(report.Files[0].Actions) != 1 {
		t.Fatalf("unexpected report shape: %#v", report)
	}
	return report
}
