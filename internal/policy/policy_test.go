package policy

import (
	"errors"
	"testing"
	"time"

	"github.com/MohamedElashri/sanad/internal/actions"
	"github.com/MohamedElashri/sanad/internal/githubresolver"
)

func TestEvaluatePolicyDefaults(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	old := now.Add(-15 * 24 * time.Hour)
	recent := now.Add(-2 * 24 * time.Hour)
	sha := "11bd71901bbe5b1630ceea73d27597364c9af683"
	newSHA := "692973e3d937129bcbf40652eb9f2f61becf3332"

	tests := []struct {
		name  string
		entry Entry
		want  DecisionKind
	}{
		{
			name: "tag tracks by default",
			entry: Entry{
				Action: actions.Parse("actions/checkout@v4"),
				Candidate: &githubresolver.ResolvedRef{
					Owner:      "actions",
					Repo:       "checkout",
					Ref:        "v4",
					SHA:        sha,
					Kind:       githubresolver.KindTag,
					CommitTime: old,
				},
				Now: now,
			},
			want: DecisionUpdate,
		},
		{
			name: "tag waits for cooldown",
			entry: Entry{
				Action: actions.Parse("actions/checkout@v4"),
				Candidate: &githubresolver.ResolvedRef{
					Owner:      "actions",
					Repo:       "checkout",
					Ref:        "v4",
					SHA:        sha,
					Kind:       githubresolver.KindTag,
					CommitTime: recent,
				},
				Now: now,
			},
			want: DecisionPending,
		},
		{
			name: "branch tracked by default",
			entry: Entry{
				Action: actions.Parse("owner/repo@main"),
				Candidate: &githubresolver.ResolvedRef{
					Owner:      "owner",
					Repo:       "repo",
					Ref:        "main",
					SHA:        sha,
					Kind:       githubresolver.KindBranch,
					CommitTime: old,
				},
				Now: now,
			},
			want: DecisionUpdate,
		},
		{
			name:  "unpinned action denied by default",
			entry: Entry{Action: actions.Parse("owner/repo"), Now: now},
			want:  DecisionErrorUnresolved,
		},
		{
			name:  "docker action skipped",
			entry: Entry{Action: actions.Parse("docker://alpine:3.20"), Now: now},
			want:  DecisionSkipDockerAction,
		},
		{
			name:  "local action skipped",
			entry: Entry{Action: actions.Parse("./.github/actions/local"), Now: now},
			want:  DecisionSkipLocalAction,
		},
		{
			name:  "full SHA accepted",
			entry: Entry{Action: actions.Parse("actions/checkout@" + sha), Now: now},
			want:  DecisionUnchanged,
		},
		{
			name:  "short SHA rejected",
			entry: Entry{Action: actions.Parse("actions/checkout@11bd719"), Now: now},
			want:  DecisionErrorShortSHA,
		},
		{
			name: "same candidate is unchanged",
			entry: Entry{
				Action: actions.Parse("actions/checkout@" + sha),
				Candidate: &githubresolver.ResolvedRef{
					Owner:      "actions",
					Repo:       "checkout",
					Ref:        sha,
					SHA:        sha,
					Kind:       githubresolver.KindSHA,
					CommitTime: old,
				},
				Now: now,
			},
			want: DecisionUnchanged,
		},
		{
			name: "managed pin updates when candidate changes",
			entry: Entry{
				Action:     actions.Parse("actions/checkout@" + sha),
				LogicalRef: "v4",
				Candidate: &githubresolver.ResolvedRef{
					Owner:      "actions",
					Repo:       "checkout",
					Ref:        "v4",
					SHA:        newSHA,
					Kind:       githubresolver.KindTag,
					CommitTime: old,
				},
				Now: now,
			},
			want: DecisionUpdate,
		},
		{
			name: "resolver errors become unresolved decisions",
			entry: Entry{
				Action:     actions.Parse("actions/checkout@v4"),
				ResolveErr: errors.New("boom"),
				Now:        now,
			},
			want: DecisionErrorUnresolved,
		},
	}

	opts := DefaultOptions()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.entry, opts)
			if got.Kind != tt.want {
				t.Fatalf("Kind = %q, want %q (%s)", got.Kind, tt.want, got.Reason)
			}
		})
	}
}

func TestEvaluatePolicyOverrides(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	old := now.Add(-15 * 24 * time.Hour)
	sha := "11bd71901bbe5b1630ceea73d27597364c9af683"

	t.Run("branch denying can be enabled", func(t *testing.T) {
		opts := DefaultOptions()
		opts.Branches = BranchDeny
		got := Evaluate(Entry{
			Action: actions.Parse("owner/repo@main"),
			Candidate: &githubresolver.ResolvedRef{
				Owner:      "owner",
				Repo:       "repo",
				Ref:        "main",
				SHA:        sha,
				Kind:       githubresolver.KindBranch,
				CommitTime: old,
			},
			Now: now,
		}, opts)
		if got.Kind != DecisionErrorBranchDenied {
			t.Fatalf("Kind = %q, want %q (%s)", got.Kind, DecisionErrorBranchDenied, got.Reason)
		}
	})

	t.Run("unpinned default branch policy accepts discovered branch", func(t *testing.T) {
		opts := DefaultOptions()
		opts.Unpinned = UnpinnedDefaultBranch
		got := Evaluate(Entry{
			Action: actions.Parse("owner/repo"),
			Candidate: &githubresolver.ResolvedRef{
				Owner:      "owner",
				Repo:       "repo",
				Ref:        "main",
				SHA:        sha,
				Kind:       githubresolver.KindBranch,
				CommitTime: old,
			},
			Now: now,
		}, opts)
		if got.Kind != DecisionUpdate {
			t.Fatalf("Kind = %q, want %q (%s)", got.Kind, DecisionUpdate, got.Reason)
		}
		if got.LogicalRef != "main" {
			t.Fatalf("LogicalRef = %q, want main", got.LogicalRef)
		}
	})

	t.Run("unsupported unpinned policy is reported", func(t *testing.T) {
		opts := DefaultOptions()
		opts.Unpinned = "surprise"
		got := Evaluate(Entry{Action: actions.Parse("owner/repo"), Now: now}, opts)
		if got.Kind != DecisionErrorUnsupported {
			t.Fatalf("Kind = %q, want %q", got.Kind, DecisionErrorUnsupported)
		}
	})

	t.Run("reusable workflows can be denied", func(t *testing.T) {
		opts := DefaultOptions()
		opts.ReusableWorkflows = false
		got := Evaluate(Entry{Action: actions.Parse("owner/repo/.github/workflows/reuse.yml@v1")}, opts)
		if got.Kind != DecisionErrorReusable {
			t.Fatalf("Kind = %q, want %q", got.Kind, DecisionErrorReusable)
		}
	})

	t.Run("ignore action patterns skip matching GitHub actions", func(t *testing.T) {
		opts := DefaultOptions()
		opts.IgnoreActions = []string{"actions/*"}
		got := Evaluate(Entry{Action: actions.Parse("actions/checkout@v4")}, opts)
		if got.Kind != DecisionSkipIgnored {
			t.Fatalf("Kind = %q, want %q", got.Kind, DecisionSkipIgnored)
		}
	})

	t.Run("ignore action patterns match raw references", func(t *testing.T) {
		opts := DefaultOptions()
		opts.IgnoreActions = []string{"actions/checkout@v4"}
		got := Evaluate(Entry{Action: actions.Parse("actions/checkout@v4")}, opts)
		if got.Kind != DecisionSkipIgnored {
			t.Fatalf("Kind = %q, want %q", got.Kind, DecisionSkipIgnored)
		}
	})

	t.Run("ignore action patterns match owner and repo", func(t *testing.T) {
		opts := DefaultOptions()
		opts.IgnoreActions = []string{"actions/checkout"}
		got := Evaluate(Entry{Action: actions.Parse("actions/checkout@v4")}, opts)
		if got.Kind != DecisionSkipIgnored {
			t.Fatalf("Kind = %q, want %q", got.Kind, DecisionSkipIgnored)
		}
	})

	t.Run("ignore action prefix patterns cross path separators", func(t *testing.T) {
		opts := DefaultOptions()
		opts.IgnoreActions = []string{"owner/repo/*"}
		got := Evaluate(Entry{Action: actions.Parse("owner/repo/path/to/action@v1")}, opts)
		if got.Kind != DecisionSkipIgnored {
			t.Fatalf("Kind = %q, want %q", got.Kind, DecisionSkipIgnored)
		}
	})

	t.Run("ignore file patterns skip matching workflow entries", func(t *testing.T) {
		opts := DefaultOptions()
		opts.IgnoreActions = nil
		opts.IgnoreFiles = []string{".github/workflows/legacy.yml"}
		got := Evaluate(Entry{
			File:   ".github/workflows/legacy.yml",
			Action: actions.Parse("actions/checkout@v4"),
		}, opts)
		if got.Kind != DecisionSkipIgnored {
			t.Fatalf("Kind = %q, want %q", got.Kind, DecisionSkipIgnored)
		}
		if got.Reason != `workflow file matches ignore pattern ".github/workflows/legacy.yml"` {
			t.Fatalf("Reason = %q", got.Reason)
		}
	})

	t.Run("invalid ignore action patterns are policy errors", func(t *testing.T) {
		opts := DefaultOptions()
		opts.IgnoreActions = []string{"["}
		got := Evaluate(Entry{Action: actions.Parse("actions/checkout@v4")}, opts)
		if got.Kind != DecisionErrorUnsupported {
			t.Fatalf("Kind = %q, want %q", got.Kind, DecisionErrorUnsupported)
		}
	})
}
