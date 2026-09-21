package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaultWhenMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(DefaultPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Source != "defaults" {
		t.Fatalf("Source = %q, want defaults", cfg.Source)
	}
	wantWorkflowPaths := []string{".github/workflows", "**/.github/workflows"}
	if len(cfg.WorkflowPaths) != len(wantWorkflowPaths) || cfg.WorkflowPaths[0] != wantWorkflowPaths[0] || cfg.WorkflowPaths[1] != wantWorkflowPaths[1] {
		t.Fatalf("WorkflowPaths = %#v", cfg.WorkflowPaths)
	}
	if cfg.Cooldown != 7*24*time.Hour {
		t.Fatalf("Cooldown = %s, want 168h", cfg.Cooldown)
	}
	if cfg.CooldownSource != DefaultCooldownSource {
		t.Fatalf("CooldownSource = %q, want %q", cfg.CooldownSource, DefaultCooldownSource)
	}
	if cfg.Updates.Tags != "track" || cfg.Updates.Branches != "track" || cfg.Updates.Unpinned != "latest-release" || !cfg.Updates.ReusableWorkflows {
		t.Fatalf("Updates = %#v", cfg.Updates)
	}
	wantIgnore := []string{"./*", "docker://*"}
	if len(cfg.Ignore.Actions) != len(wantIgnore) {
		t.Fatalf("Ignore.Actions = %#v, want %#v", cfg.Ignore.Actions, wantIgnore)
	}
	for i := range wantIgnore {
		if cfg.Ignore.Actions[i] != wantIgnore[i] {
			t.Fatalf("Ignore.Actions = %#v, want %#v", cfg.Ignore.Actions, wantIgnore)
		}
	}
}

func TestLoadExplicitMissingErrors(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err == nil {
		t.Fatal("Load returned nil error for explicit missing config")
	}
}

func TestLoadExistingConfigMarksSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte("# sanad config\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Source != path {
		t.Fatalf("Source = %q, want %q", cfg.Source, path)
	}
}

func TestPersistBranchTrackingCreatesDefaultConfig(t *testing.T) {
	root := t.TempDir()
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

	if err := PersistBranchTracking(DefaultPath); err != nil {
		t.Fatalf("PersistBranchTracking returned error: %v", err)
	}
	cfg, err := Load(DefaultPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Updates.Branches != "track" {
		t.Fatalf("Updates.Branches = %q, want track", cfg.Updates.Branches)
	}
}

func TestPersistBranchTrackingUpdatesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := "# keep\nworkflow_paths = [\".github/workflows\"]\n\n[updates]\ntags = \"track\"\nbranches = 'deny' # keep comment\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := PersistBranchTracking(path); err != nil {
		t.Fatalf("PersistBranchTracking returned error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantLine := `branches = "track" # keep comment`
	if !strings.Contains(string(got), wantLine) {
		t.Fatalf("config missing %q:\n%s", wantLine, string(got))
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Updates.Branches != "track" || cfg.WorkflowPaths[0] != ".github/workflows" {
		t.Fatalf("unexpected config after persistence: %#v", cfg)
	}
}

func TestPersistBranchTrackingAddsUpdatesWithoutChangingOtherBranches(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := "[other]\nbranches = \"keep\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := PersistBranchTracking(path); err != nil {
		t.Fatalf("PersistBranchTracking returned error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "[other]\nbranches = \"keep\"") {
		t.Fatalf("other table was changed:\n%s", text)
	}
	if !strings.Contains(text, "[updates]\nbranches = \"track\"") {
		t.Fatalf("updates section was not appended:\n%s", text)
	}
}

func TestLoadWorkflowPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`workflow_paths = ["ci/workflows", ".github/workflows"]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := []string{"ci/workflows", ".github/workflows"}
	if len(cfg.WorkflowPaths) != len(want) {
		t.Fatalf("WorkflowPaths = %#v, want %#v", cfg.WorkflowPaths, want)
	}
	for i := range want {
		if cfg.WorkflowPaths[i] != want[i] {
			t.Fatalf("WorkflowPaths = %#v, want %#v", cfg.WorkflowPaths, want)
		}
	}
}

func TestLoadWorkflowPathsMultiline(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`workflow_paths = [
  ".github/workflows", # default
  'ci/workflows',
]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := []string{".github/workflows", "ci/workflows"}
	if len(cfg.WorkflowPaths) != len(want) {
		t.Fatalf("WorkflowPaths = %#v, want %#v", cfg.WorkflowPaths, want)
	}
	for i := range want {
		if cfg.WorkflowPaths[i] != want[i] {
			t.Fatalf("WorkflowPaths = %#v, want %#v", cfg.WorkflowPaths, want)
		}
	}
}

func TestLoadRejectsWorkflowPathsOutsideRepository(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "absolute", path: filepath.Join(t.TempDir(), ".github", "workflows")},
		{name: "parent", path: "../workflows"},
		{name: "empty", path: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), DefaultPath)
			content := `workflow_paths = [` + quoteTOML(tt.path) + "]\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load returned nil error")
			}
		})
	}
}

func quoteTOML(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}

func TestLoadExampleConfig(t *testing.T) {
	path := filepath.Join("..", "..", ".sanad.toml.example")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error for example config: %v", err)
	}

	if cfg.Source != path {
		t.Fatalf("Source = %q, want %q", cfg.Source, path)
	}
	wantWorkflowPaths := []string{".github/workflows", "**/.github/workflows"}
	if len(cfg.WorkflowPaths) != len(wantWorkflowPaths) || cfg.WorkflowPaths[0] != wantWorkflowPaths[0] || cfg.WorkflowPaths[1] != wantWorkflowPaths[1] {
		t.Fatalf("WorkflowPaths = %#v", cfg.WorkflowPaths)
	}
	if cfg.Cooldown != 7*24*time.Hour {
		t.Fatalf("Cooldown = %s, want 168h", cfg.Cooldown)
	}
	if cfg.CooldownSource != DefaultCooldownSource {
		t.Fatalf("CooldownSource = %q, want %q", cfg.CooldownSource, DefaultCooldownSource)
	}
	if len(cfg.Ignore.Files) != 0 {
		t.Fatalf("Ignore.Files = %#v, want empty", cfg.Ignore.Files)
	}
	if !cfg.Comments.Write || cfg.Comments.Format != DefaultCommentFormat {
		t.Fatalf("Comments = %#v", cfg.Comments)
	}
	if !cfg.Security.RequireFullSHA || !cfg.Security.RequireCommitInSourceRepo || !cfg.Security.AllowPrivate || cfg.Security.DenyForks {
		t.Fatalf("Security = %#v", cfg.Security)
	}
	if cfg.Upgrade.LatestRelease != DefaultUpgradeLatestRelease {
		t.Fatalf("Upgrade.LatestRelease = %q, want %q", cfg.Upgrade.LatestRelease, DefaultUpgradeLatestRelease)
	}
	if cfg.Upgrade.Level != DefaultUpgradeLevel || cfg.Upgrade.Selection != DefaultUpgradeSelection || cfg.Upgrade.Constraint != "" {
		t.Fatalf("unexpected default upgrade policy: %#v", cfg.Upgrade)
	}
}

func TestLoadDottedNestedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`updates.tags = "deny"
updates.branches = "track"
ignore.actions = []
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Updates.Tags != "deny" || cfg.Updates.Branches != "track" {
		t.Fatalf("Updates = %#v", cfg.Updates)
	}
	if len(cfg.Ignore.Actions) != 0 {
		t.Fatalf("Ignore.Actions = %#v, want explicit empty override", cfg.Ignore.Actions)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{value: "7d", want: 7 * 24 * time.Hour},
		{value: "48h", want: 48 * time.Hour},
		{value: "30m", want: 30 * time.Minute},
		{value: "0s", want: 0},
	}

	for _, tt := range tests {
		got, err := ParseDuration(tt.value)
		if err != nil {
			t.Fatalf("ParseDuration(%q) returned error: %v", tt.value, err)
		}
		if got != tt.want {
			t.Fatalf("ParseDuration(%q) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

func TestParseDurationRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "d", "1w", "1.5d", "-1h"} {
		if _, err := ParseDuration(value); err == nil {
			t.Fatalf("ParseDuration(%q) returned nil error", value)
		}
	}
}

func TestLoadCooldown(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`workflow_paths = [".github/workflows"]
cooldown = "48h"

[updates]
tags = "track"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Cooldown != 48*time.Hour {
		t.Fatalf("Cooldown = %s, want 48h", cfg.Cooldown)
	}
}

func TestLoadCooldownDayDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte(`cooldown = "7d"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Cooldown != 7*24*time.Hour {
		t.Fatalf("Cooldown = %s, want 336h", cfg.Cooldown)
	}
}

func TestLoadCooldownInvalidDurationErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte(`cooldown = "1w"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for invalid cooldown")
	}
}

func TestLoadCooldownSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte(`cooldown_source = "first-seen"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.CooldownSource != "first-seen" {
		t.Fatalf("CooldownSource = %q, want first-seen", cfg.CooldownSource)
	}
}

func TestLoadCooldownSourceRejectsUnsupportedValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte(`cooldown_source = "lockfile"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for unsupported cooldown_source")
	}
}

func TestLoadUpdatesPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`[updates]
tags = "deny"
branches = "track"
unpinned = "latest-release"
reusable_workflows = false
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Updates.Tags != "deny" {
		t.Fatalf("Updates.Tags = %q, want deny", cfg.Updates.Tags)
	}
	if cfg.Updates.Branches != "track" {
		t.Fatalf("Updates.Branches = %q, want track", cfg.Updates.Branches)
	}
	if cfg.Updates.Unpinned != "latest-release" {
		t.Fatalf("Updates.Unpinned = %q, want latest-release", cfg.Updates.Unpinned)
	}
	if cfg.Updates.ReusableWorkflows {
		t.Fatal("Updates.ReusableWorkflows = true, want false")
	}
}

func TestLoadIgnoreActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`[ignore]
actions = [
  "owner/repo",
  "actions/*",
]
files = [
  ".github/workflows/legacy.yml",
]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := []string{"owner/repo", "actions/*"}
	if len(cfg.Ignore.Actions) != len(want) {
		t.Fatalf("Ignore.Actions = %#v, want %#v", cfg.Ignore.Actions, want)
	}
	for i := range want {
		if cfg.Ignore.Actions[i] != want[i] {
			t.Fatalf("Ignore.Actions = %#v, want %#v", cfg.Ignore.Actions, want)
		}
	}
	wantFiles := []string{".github/workflows/legacy.yml"}
	if len(cfg.Ignore.Files) != len(wantFiles) {
		t.Fatalf("Ignore.Files = %#v, want %#v", cfg.Ignore.Files, wantFiles)
	}
	for i := range wantFiles {
		if cfg.Ignore.Files[i] != wantFiles[i] {
			t.Fatalf("Ignore.Files = %#v, want %#v", cfg.Ignore.Files, wantFiles)
		}
	}
}

func TestLoadRejectsGitHubConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`[github]
api_url = "https://github.example.com/api/v3"
send_token_to_custom_api_url = true
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for unsupported GitHub config")
	}
}

func TestLoadRejectsDottedGitHubConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte(`github.api_url = "https://github.example.com/api/v3"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for unsupported dotted GitHub config")
	}
}

func TestLoadUpgradeLatestRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`[upgrade]
latest_release = "release"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Upgrade.LatestRelease != DefaultUpgradeLatestRelease {
		t.Fatalf("Upgrade.LatestRelease = %q, want %q", cfg.Upgrade.LatestRelease, DefaultUpgradeLatestRelease)
	}
}

func TestLoadUpgradeLatestReleaseRejectsUnsupportedMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte("[upgrade]\nlatest_release = \"tag\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for unsupported upgrade latest_release mode")
	}
}

func TestLoadUpgradePoliciesAndPerActionOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`[upgrade]
constraint = ">= 4, < 7"
selection = "latest"

[upgrade.actions."actions/checkout"]
level = "minor"
selection = "latest-eligible"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Upgrade.Level != "" || cfg.Upgrade.Constraint != ">= 4, < 7" || cfg.Upgrade.Selection != "latest" {
		t.Fatalf("unexpected global upgrade policy: %#v", cfg.Upgrade)
	}
	override := cfg.Upgrade.Actions["actions/checkout"]
	if override.Level != "minor" || override.Constraint != "" || override.Selection != "latest-eligible" {
		t.Fatalf("unexpected action override: %#v", override)
	}
}

func TestLoadUpgradePolicyRejectsInvalidCombinations(t *testing.T) {
	for _, content := range []string{
		"[upgrade]\nlevel = \"minor\"\nconstraint = \"< 5\"\n",
		"[upgrade]\nlevel = \"breaking\"\n",
		"[upgrade]\nconstraint = \"not a constraint\"\n",
		"[upgrade]\nselection = \"oldest\"\n",
	} {
		path := filepath.Join(t.TempDir(), DefaultPath)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("Load returned nil error for %q", content)
		}
	}
}

func TestLoadCommentsConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`[comments]
write = false
format = "sanad: ref={{ref}}"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Comments.Write {
		t.Fatal("Comments.Write = true, want false")
	}
	if cfg.Comments.Format != DefaultCommentFormat {
		t.Fatalf("Comments.Format = %q, want %q", cfg.Comments.Format, DefaultCommentFormat)
	}
}

func TestLoadRejectsUnsupportedCommentFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte("[comments]\nformat = \"ref={{ref}}\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for unsupported comment format")
	}
}

func TestLoadSecurityConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	content := []byte(`[security]
require_full_sha = true
require_commit_in_source_repo = true
allow_private = true
deny_forks = false
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !cfg.Security.RequireFullSHA || !cfg.Security.RequireCommitInSourceRepo || !cfg.Security.AllowPrivate || cfg.Security.DenyForks {
		t.Fatalf("Security = %#v", cfg.Security)
	}
}

func TestLoadSecurityConfigFailsClosedForUnsupportedCombinations(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "disable full sha",
			content: "[security]\nrequire_full_sha = false\n",
		},
		{
			name:    "disable source repo check",
			content: "[security]\nrequire_commit_in_source_repo = false\n",
		},
		{
			name:    "deny private",
			content: "[security]\nallow_private = false\n",
		},
		{
			name:    "deny forks",
			content: "[security]\ndeny_forks = true\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), DefaultPath)
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load returned nil error")
			}
		})
	}
}

func TestLoadOrganizationPolicyFilesApplyBeforeLocalConfig(t *testing.T) {
	root := t.TempDir()
	policyPath := filepath.Join(root, "org-policy.toml")
	if err := os.WriteFile(policyPath, []byte(`[updates]
tags = "deny"
branches = "track"

[ignore]
actions = ["org/internal-action"]

[upgrade]
constraint = "< 7"

[upgrade.actions."actions/checkout"]
constraint = "< 6"
selection = "latest"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, DefaultPath)
	content := []byte(`[organization]
policy_files = ["org-policy.toml"]

[updates]
tags = "track"

[upgrade]
level = "minor"

[upgrade.actions."actions/checkout"]
selection = "latest-eligible"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Updates.Tags != "track" {
		t.Fatalf("Updates.Tags = %q, want local override track", cfg.Updates.Tags)
	}
	if cfg.Updates.Branches != "track" {
		t.Fatalf("Updates.Branches = %q, want organization policy track", cfg.Updates.Branches)
	}
	if len(cfg.Ignore.Actions) != 1 || cfg.Ignore.Actions[0] != "org/internal-action" {
		t.Fatalf("Ignore.Actions = %#v, want organization policy action", cfg.Ignore.Actions)
	}
	if len(cfg.Organization.PolicyFiles) != 1 || cfg.Organization.PolicyFiles[0] != "org-policy.toml" {
		t.Fatalf("Organization.PolicyFiles = %#v", cfg.Organization.PolicyFiles)
	}
	if cfg.Upgrade.Level != "minor" || cfg.Upgrade.Constraint != "" {
		t.Fatalf("local global upgrade policy did not override organization policy: %#v", cfg.Upgrade)
	}
	upgradeOverride := cfg.Upgrade.Actions["actions/checkout"]
	if upgradeOverride.Constraint != "< 6" || upgradeOverride.Selection != "latest-eligible" {
		t.Fatalf("per-action upgrade policy was not merged field by field: %#v", upgradeOverride)
	}
}

func TestLoadInvalidNestedPolicyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte("[updates]\nreusable_workflows = maybe\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error for invalid nested policy")
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte("[updates]\nbranch = \"track\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported key(s): updates.branch") {
		t.Fatalf("Load error = %v, want unknown-key diagnostic", err)
	}
}

func TestLoadRejectsInvalidUpdateEnums(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPath)
	if err := os.WriteFile(path, []byte("[updates]\nbranches = \"sometimes\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "updates.branches") {
		t.Fatalf("Load error = %v, want invalid branches diagnostic", err)
	}
}
