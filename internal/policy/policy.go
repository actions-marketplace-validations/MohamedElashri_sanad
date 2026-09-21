package policy

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/MohamedElashri/sanad/internal/actions"
	"github.com/MohamedElashri/sanad/internal/githubresolver"
)

type TagPolicy string
type BranchPolicy string
type UnpinnedPolicy string
type CooldownSource string

const (
	TagTrack      TagPolicy = "track"
	TagDeny       TagPolicy = "deny"
	TagPinCurrent TagPolicy = "pin-current"

	BranchDeny       BranchPolicy = "deny"
	BranchPinCurrent BranchPolicy = "pin-current"
	BranchTrack      BranchPolicy = "track"

	UnpinnedDeny          UnpinnedPolicy = "deny"
	UnpinnedDefaultBranch UnpinnedPolicy = "default-branch"
	UnpinnedLatestRelease UnpinnedPolicy = "latest-release"

	CooldownSourceUpstream  CooldownSource = "source"
	CooldownSourceFirstSeen CooldownSource = "first-seen"
)

type Options struct {
	Tags              TagPolicy
	Branches          BranchPolicy
	Unpinned          UnpinnedPolicy
	ReusableWorkflows bool
	IgnoreActions     []string
	IgnoreFiles       []string
	Cooldown          time.Duration
	CooldownSource    CooldownSource
	Now               time.Time
}

type Entry struct {
	File            string
	Action          actions.ParsedAction
	Candidate       *githubresolver.ResolvedRef
	ResolveErr      error
	LogicalRef      string
	CurrentSHA      string
	CandidateSeenAt time.Time
	Now             time.Time
}

func DefaultOptions() Options {
	return Options{
		Tags:              TagTrack,
		Branches:          BranchTrack,
		Unpinned:          UnpinnedLatestRelease,
		ReusableWorkflows: true,
		IgnoreActions:     []string{"./*", "docker://*"},
		Cooldown:          7 * 24 * time.Hour,
		CooldownSource:    CooldownSourceUpstream,
	}
}

func Evaluate(entry Entry, opts Options) Decision {
	opts = withDefaults(opts)
	action := entry.Action

	switch action.Kind {
	case actions.KindLocalAction:
		return Decision{Kind: DecisionSkipLocalAction, Reason: "local actions are skipped"}
	case actions.KindDockerAction:
		return Decision{Kind: DecisionSkipDockerAction, Reason: "docker actions are skipped"}
	}

	ignore, err := MatchIgnore(action, entry.File, opts)
	if err != nil {
		return Decision{
			Kind:   DecisionErrorUnsupported,
			Reason: err.Error(),
		}
	}
	if ignore.Ignored {
		return Decision{
			Kind:       DecisionSkipIgnored,
			Reason:     ignore.Reason(),
			CurrentSHA: currentSHA(entry),
			LogicalRef: logicalRef(entry),
		}
	}

	switch action.Kind {
	case actions.KindInvalid:
		return invalidActionDecision(action)
	case actions.KindGitHubAction:
	case actions.KindReusableWorkflow:
		if !opts.ReusableWorkflows {
			return Decision{
				Kind:       DecisionErrorReusable,
				Reason:     "reusable workflows are denied by policy",
				LogicalRef: logicalRef(entry),
			}
		}
	default:
		return Decision{
			Kind:   DecisionErrorInvalid,
			Reason: fmt.Sprintf("unsupported action kind %q", action.Kind),
		}
	}

	if !action.Valid {
		return invalidActionDecision(action)
	}

	if action.Ref == "" && entry.LogicalRef == "" {
		switch opts.Unpinned {
		case UnpinnedDeny:
			return Decision{
				Kind:   DecisionErrorUnpinned,
				Reason: "unpinned GitHub action references are denied by policy",
			}
		case UnpinnedDefaultBranch, UnpinnedLatestRelease:
			return candidateDecision(entry, opts, fmt.Sprintf("unpinned action policy %q requires a resolved candidate", opts.Unpinned))
		default:
			return unsupportedPolicy("updates.unpinned", string(opts.Unpinned))
		}
	}

	if action.Pinned && entry.LogicalRef == "" && entry.Candidate == nil && entry.ResolveErr == nil {
		return Decision{
			Kind:       DecisionUnchanged,
			Reason:     "full SHA pin accepted",
			CurrentSHA: currentSHA(entry),
			LogicalRef: logicalRef(entry),
		}
	}

	if entry.ResolveErr != nil {
		return Decision{
			Kind:       DecisionErrorUnresolved,
			Reason:     entry.ResolveErr.Error(),
			CurrentSHA: currentSHA(entry),
			LogicalRef: logicalRef(entry),
		}
	}
	if entry.Candidate == nil {
		return Decision{
			Kind:       DecisionErrorUnresolved,
			Reason:     "resolved candidate is required",
			CurrentSHA: currentSHA(entry),
			LogicalRef: logicalRef(entry),
		}
	}

	switch entry.Candidate.Kind {
	case githubresolver.KindTag:
		switch opts.Tags {
		case TagTrack, TagPinCurrent:
		case TagDeny:
			return Decision{
				Kind:       DecisionErrorTagDenied,
				Reason:     "tag refs are denied by policy",
				CurrentSHA: currentSHA(entry),
				NewSHA:     entry.Candidate.SHA,
				LogicalRef: logicalRef(entry),
			}
		default:
			return unsupportedPolicy("updates.tags", string(opts.Tags))
		}
	case githubresolver.KindBranch:
		switch opts.Branches {
		case BranchTrack, BranchPinCurrent:
		case BranchDeny:
			return Decision{
				Kind:       DecisionErrorBranchDenied,
				Reason:     "branch refs are denied by policy",
				CurrentSHA: currentSHA(entry),
				NewSHA:     entry.Candidate.SHA,
				LogicalRef: logicalRef(entry),
			}
		default:
			return unsupportedPolicy("updates.branches", string(opts.Branches))
		}
	case githubresolver.KindSHA:
	default:
		return Decision{
			Kind:       DecisionErrorUnresolved,
			Reason:     fmt.Sprintf("unsupported resolved ref kind %q", entry.Candidate.Kind),
			CurrentSHA: currentSHA(entry),
			LogicalRef: logicalRef(entry),
		}
	}

	return candidateDecision(entry, opts, "resolved candidate is available")
}

func candidateDecision(entry Entry, opts Options, reason string) Decision {
	if entry.ResolveErr != nil {
		return Decision{
			Kind:       DecisionErrorUnresolved,
			Reason:     entry.ResolveErr.Error(),
			CurrentSHA: currentSHA(entry),
			LogicalRef: logicalRef(entry),
		}
	}
	if entry.Candidate == nil {
		return Decision{
			Kind:       DecisionErrorUnresolved,
			Reason:     reason,
			CurrentSHA: currentSHA(entry),
			LogicalRef: logicalRef(entry),
		}
	}

	current := currentSHA(entry)
	logical := logicalRef(entry)
	if current != "" && current == entry.Candidate.SHA {
		return Decision{
			Kind:       DecisionUnchanged,
			Reason:     "current SHA already matches resolved candidate",
			CurrentSHA: current,
			NewSHA:     entry.Candidate.SHA,
			LogicalRef: logical,
		}
	}

	now := entry.Now
	if now.IsZero() {
		now = opts.Now
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	candidateTime := candidateTimestamp(*entry.Candidate)
	if opts.CooldownSource == CooldownSourceFirstSeen && !entry.CandidateSeenAt.IsZero() {
		candidateTime = entry.CandidateSeenAt
	}
	cooldown := EvaluateCooldown(now, candidateTime, opts.Cooldown)
	return Decision{
		Kind:            cooldown.Kind,
		Reason:          cooldown.Reason,
		CurrentSHA:      current,
		NewSHA:          entry.Candidate.SHA,
		LogicalRef:      logical,
		Age:             cooldown.Age,
		CandidateSeenAt: candidateTime,
	}
}

func invalidActionDecision(action actions.ParsedAction) Decision {
	kind := DecisionErrorInvalid
	if actions.IsShortSHA(action.Ref) {
		kind = DecisionErrorShortSHA
	}
	return Decision{
		Kind:       kind,
		Reason:     action.Error,
		CurrentSHA: pinnedSHA(action),
		LogicalRef: action.Ref,
	}
}

func candidateTimestamp(candidate githubresolver.ResolvedRef) time.Time {
	switch candidate.Kind {
	case githubresolver.KindTag:
		if candidate.ReleaseTime != nil && !candidate.ReleaseTime.IsZero() {
			return *candidate.ReleaseTime
		}
		if candidate.TagTime != nil && !candidate.TagTime.IsZero() {
			return *candidate.TagTime
		}
	}
	return candidate.CommitTime
}

func currentSHA(entry Entry) string {
	if entry.CurrentSHA != "" {
		return entry.CurrentSHA
	}
	return pinnedSHA(entry.Action)
}

func pinnedSHA(action actions.ParsedAction) string {
	if action.Pinned {
		return action.Ref
	}
	return ""
}

func logicalRef(entry Entry) string {
	if entry.LogicalRef != "" {
		return entry.LogicalRef
	}
	if (entry.Action.Ref == "" || entry.Action.Pinned) && entry.Candidate != nil {
		return entry.Candidate.Ref
	}
	if !entry.Action.Pinned {
		return entry.Action.Ref
	}
	return ""
}

type IgnoreMatch struct {
	Ignored bool
	Kind    string
	Pattern string
	Value   string
}

func (m IgnoreMatch) Reason() string {
	switch m.Kind {
	case "file":
		return fmt.Sprintf("workflow file matches ignore pattern %q", m.Pattern)
	default:
		return fmt.Sprintf("action matches ignore pattern %q", m.Pattern)
	}
}

func MatchIgnore(action actions.ParsedAction, file string, opts Options) (IgnoreMatch, error) {
	opts = withDefaults(opts)
	match, err := ignoredValue("action", opts.IgnoreActions, actionMatchValues(action))
	if err != nil || match.Ignored {
		return match, err
	}
	if file == "" {
		return IgnoreMatch{}, nil
	}
	return ignoredValue("file", opts.IgnoreFiles, []string{file})
}

func ignoredValue(kind string, patterns []string, values []string) (IgnoreMatch, error) {
	for _, pattern := range patterns {
		for _, value := range values {
			match, err := globMatch(pattern, value)
			if err != nil {
				return IgnoreMatch{}, fmt.Errorf("invalid ignore %s pattern %q: %w", kind, pattern, err)
			}
			if match {
				return IgnoreMatch{
					Ignored: true,
					Kind:    kind,
					Pattern: pattern,
					Value:   value,
				}, nil
			}
		}
	}
	return IgnoreMatch{}, nil
}

func globMatch(pattern string, value string) (bool, error) {
	match, err := path.Match(pattern, value)
	if err != nil || match {
		return match, err
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*")), nil
	}
	return false, nil
}

func actionMatchValues(action actions.ParsedAction) []string {
	values := []string{action.Raw}
	switch {
	case action.Owner != "" && action.Repo != "" && action.Path != "":
		values = append(values, action.Owner+"/"+action.Repo+"/"+action.Path)
	case action.Owner != "" && action.Repo != "":
		values = append(values, action.Owner+"/"+action.Repo)
	case action.Path != "":
		values = append(values, action.Path)
	}
	return values
}

func unsupportedPolicy(key string, value string) Decision {
	return Decision{
		Kind:   DecisionErrorUnsupported,
		Reason: fmt.Sprintf("%s has unsupported value %q", key, value),
	}
}

func withDefaults(opts Options) Options {
	defaults := DefaultOptions()
	if opts.Tags == "" {
		opts.Tags = defaults.Tags
	}
	if opts.Branches == "" {
		opts.Branches = defaults.Branches
	}
	if opts.Unpinned == "" {
		opts.Unpinned = defaults.Unpinned
	}
	if opts.IgnoreActions == nil {
		opts.IgnoreActions = defaults.IgnoreActions
	}
	if opts.CooldownSource == "" {
		opts.CooldownSource = defaults.CooldownSource
	}
	return opts
}
