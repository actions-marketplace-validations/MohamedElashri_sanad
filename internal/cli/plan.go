package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/MohamedElashri/sanad/internal/actions"
	"github.com/MohamedElashri/sanad/internal/config"
	"github.com/MohamedElashri/sanad/internal/githubresolver"
	"github.com/MohamedElashri/sanad/internal/metadata"
	"github.com/MohamedElashri/sanad/internal/policy"
	"github.com/MohamedElashri/sanad/internal/workflow"
	"github.com/spf13/cobra"
)

type planResolver interface {
	Resolve(context.Context, githubresolver.ActionSelector) (githubresolver.ResolvedRef, error)
}

type defaultBranchResolver interface {
	ResolveDefaultBranch(context.Context, string, string) (githubresolver.ResolvedRef, error)
}

type latestReleaseResolver interface {
	ResolveLatestRelease(context.Context, string, string) (githubresolver.ResolvedRef, error)
}

type releaseResolver interface {
	ListReleases(context.Context, string, string) ([]githubresolver.Release, error)
}

var (
	defaultPlanResolver        planResolver
	defaultPlanResolverFactory = newDefaultPlanResolver
	planNow                    = func() time.Time { return time.Now().UTC() }
)

type planOptions struct {
	out           string
	prBodyOut     string
	workflowPaths []string
}

type planReport struct {
	Version int         `json:"version"`
	Summary planSummary `json:"summary"`
	Files   []planFile  `json:"files"`
}

type planSummary struct {
	Actions          int `json:"actions"`
	AlreadyPinned    int `json:"already_pinned"`
	UpdatesAvailable int `json:"updates_available"`
	PendingCooldown  int `json:"pending_cooldown"`
	PolicyViolations int `json:"policy_violations"`
	Skipped          int `json:"skipped"`
}

type planFile struct {
	Path    string       `json:"path"`
	Actions []planAction `json:"actions"`
}

type planAction struct {
	NodePath         string              `json:"node_path"`
	Line             int                 `json:"line"`
	Column           int                 `json:"column"`
	Raw              string              `json:"raw"`
	InlineComment    string              `json:"inline_comment,omitempty"`
	Owner            string              `json:"owner,omitempty"`
	Repo             string              `json:"repo,omitempty"`
	Path             string              `json:"path,omitempty"`
	Kind             actions.ActionKind  `json:"kind"`
	LogicalRef       string              `json:"logical_ref,omitempty"`
	MetadataSource   string              `json:"metadata_source,omitempty"`
	CurrentSHA       string              `json:"current_sha,omitempty"`
	CandidateSHA     string              `json:"candidate_sha,omitempty"`
	CandidateRefKind string              `json:"candidate_ref_kind,omitempty"`
	Decision         policy.DecisionKind `json:"decision"`
	ReasonCode       string              `json:"reason_code"`
	Reason           string              `json:"reason,omitempty"`
	Age              string              `json:"age,omitempty"`
	AgeSeconds       int64               `json:"age_seconds,omitempty"`
}

type evaluatedUse struct {
	Use              workflow.UseNode
	Parsed           actions.ParsedAction
	Recovered        metadata.Metadata
	HasMetadata      bool
	MetadataErr      error
	Candidate        *githubresolver.ResolvedRef
	CandidateSeenAt  time.Time
	CandidateHistory []metadata.CandidateHistoryEntry
	ResolveErr       error
	Decision         policy.Decision
	Action           planAction
}

func newPlanCommand(opts *rootOptions) *cobra.Command {
	planOpts := &planOptions{}

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Resolve workflow dependencies and show proposed pin changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withRepositoryRoot(opts, func() error { return runPlan(cmd, opts, planOpts, defaultPlanResolver) })
		},
	}
	cmd.Flags().StringVar(&planOpts.out, "out", "", "write JSON plan to a file")
	cmd.Flags().StringVar(&planOpts.prBodyOut, "pr-body-out", "", "write a Markdown pull request body to a file")
	cmd.Flags().StringSliceVar(&planOpts.workflowPaths, "workflows", nil, "workflow file or directory paths to scan")
	return cmd
}

func runPlan(cmd *cobra.Command, opts *rootOptions, planOpts *planOptions, resolver planResolver) error {
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	resolver, err = configuredPlanResolver(cfg, resolver)
	if err != nil {
		return err
	}

	report, err := buildPlanReport(cmd.Context(), cfg, planOpts.workflowPaths, resolver, planNow())
	if err != nil {
		return err
	}

	if planOpts.out != "" {
		if err := writePlanJSON(planOpts.out, report); err != nil {
			return err
		}
	}
	if planOpts.prBodyOut != "" {
		if err := writePlanPRBody(planOpts.prBodyOut, report); err != nil {
			return err
		}
	}

	switch opts.format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "table":
		return printPlanTable(cmd, report)
	default:
		return fmt.Errorf("unsupported format %q: expected table or json", opts.format)
	}
}

func configuredPlanResolver(cfg config.Config, resolver planResolver) (planResolver, error) {
	if resolver != nil {
		return resolver, nil
	}
	return defaultPlanResolverFactory(cfg)
}

func newDefaultPlanResolver(_ config.Config) (planResolver, error) {
	return githubresolver.NewClientFromEnv()
}

func buildPlanReport(ctx context.Context, cfg config.Config, workflowPaths []string, resolver planResolver, now time.Time) (planReport, error) {
	files, evaluated, err := evaluateWorkflowUses(ctx, cfg, workflowPaths, resolver, now)
	if err != nil {
		return planReport{}, err
	}

	actionsByFile := make(map[string][]planAction)
	for _, item := range evaluated {
		actionsByFile[item.Use.File] = append(actionsByFile[item.Use.File], item.Action)
	}

	report := planReport{Version: planReportVersion, Files: []planFile{}}
	for _, file := range files {
		if len(actionsByFile[file]) == 0 {
			continue
		}
		report.Files = append(report.Files, planFile{
			Path:    file,
			Actions: actionsByFile[file],
		})
	}
	sort.Slice(report.Files, func(i, j int) bool {
		return report.Files[i].Path < report.Files[j].Path
	})
	report.Summary = summarizePlan(report)
	return report, nil
}

func evaluateWorkflowUses(ctx context.Context, cfg config.Config, workflowPaths []string, resolver planResolver, now time.Time) ([]string, []evaluatedUse, error) {
	paths := cfg.WorkflowPaths
	if len(workflowPaths) > 0 {
		paths = workflowPaths
	}

	files, err := workflow.DiscoverWorkflowFiles(paths)
	if err != nil {
		return nil, nil, err
	}

	uses, err := workflow.ExtractUsesFromFiles(files)
	if err != nil {
		return nil, nil, err
	}
	reconciliation, err := reconcileWorkflowUses(uses)
	if err != nil {
		return nil, nil, err
	}

	evaluated := make([]evaluatedUse, 0, len(uses))
	for _, use := range uses {
		parsed := actions.Parse(use.Raw)
		reconciled, _ := reconciliation.Use(use.File, use.NodePath)
		recovered := reconciled.Metadata
		hasMetadata := reconciled.HasMetadata
		metadataErr := reconciled.Error
		var candidate *githubresolver.ResolvedRef
		var candidateSeenAt time.Time
		var resolveErr error
		var decision policy.Decision
		if metadataErr != nil {
			decision = policy.Decision{
				Kind:       policy.DecisionErrorInvalid,
				Reason:     metadataErr.Error(),
				CurrentSHA: currentPinnedSHA(parsed),
			}
		} else {
			logicalRef := ""
			metadataSource := ""
			if hasMetadata {
				logicalRef = recovered.LogicalRef
				metadataSource = string(recovered.Source)
			}
			var ignore policy.IgnoreMatch
			var ignoreErr error
			if shouldCheckIgnoreBeforeResolve(parsed) {
				ignore, ignoreErr = policy.MatchIgnore(parsed, use.File, policyOptionsFromConfig(cfg, now))
			}
			if ignoreErr != nil {
				decision = policy.Decision{
					Kind:       policy.DecisionErrorUnsupported,
					Reason:     ignoreErr.Error(),
					CurrentSHA: currentPinnedSHA(parsed),
					LogicalRef: logicalRefForDecision(parsed, logicalRef),
				}
			} else if ignore.Ignored {
				decision = ignoredDecision(parsed, logicalRef, ignore)
			} else {
				candidate, resolveErr = resolvePlanCandidate(ctx, resolver, parsed, logicalRef, policy.UnpinnedPolicy(cfg.Updates.Unpinned))
				candidateSeenAt = candidateObservationTime(
					reconciled.Entry,
					reconciled.HasEntry && reconciled.CandidateHistoryPreservable,
					hasMetadata,
					candidate,
					now,
					cfg.CooldownSource,
				)
				decision = policy.Evaluate(policy.Entry{
					File:            use.File,
					Action:          parsed,
					Candidate:       candidate,
					ResolveErr:      resolveErr,
					LogicalRef:      logicalRef,
					CandidateSeenAt: candidateSeenAt,
					Now:             now,
				}, policyOptionsFromConfig(cfg, now))
			}
			if metadataSource != "" && decision.LogicalRef == "" {
				decision.LogicalRef = logicalRef
			}
		}

		action := planActionFromDecision(use, parsed, candidate, decision)
		if hasMetadata && metadataErr == nil {
			action.MetadataSource = string(recovered.Source)
		}
		var candidateHistory []metadata.CandidateHistoryEntry
		if reconciled.CandidateHistoryPreservable {
			candidateHistory = append(candidateHistory, reconciled.Entry.Candidates...)
		}
		evaluated = append(evaluated, evaluatedUse{
			Use:              use,
			Parsed:           parsed,
			Recovered:        recovered,
			HasMetadata:      hasMetadata,
			MetadataErr:      metadataErr,
			Candidate:        candidate,
			CandidateSeenAt:  candidateSeenAt,
			CandidateHistory: candidateHistory,
			ResolveErr:       resolveErr,
			Decision:         decision,
			Action:           action,
		})
	}
	return files, evaluated, nil
}

func candidateObservationTime(entry metadata.LockfileEntry, hasLockfile bool, hasMetadata bool, candidate *githubresolver.ResolvedRef, now time.Time, cooldownSource string) time.Time {
	if candidate == nil {
		return time.Time{}
	}
	if hasLockfile {
		for _, observed := range entry.Candidates {
			if observed.LogicalRef == candidate.Ref && observed.SHA == candidate.SHA {
				seenAt, err := time.Parse(time.RFC3339, observed.SeenAt)
				if err == nil {
					return seenAt
				}
			}
		}
	}
	if hasLockfile || hasMetadata || cooldownSource == string(policy.CooldownSourceFirstSeen) {
		return now
	}
	return time.Time{}
}

func reconcileWorkflowUses(uses []workflow.UseNode) (metadata.Reconciliation, error) {
	lockfile, hasLockfile, err := metadata.LoadLockfile(metadata.DefaultLockfilePath)
	if err != nil {
		return metadata.Reconciliation{}, err
	}

	reconcileUses := make([]metadata.ReconcileUse, 0, len(uses))
	for _, use := range uses {
		reconcileUses = append(reconcileUses, metadata.ReconcileUse{
			File:          use.File,
			Node:          use.NodePath,
			InlineComment: use.InlineComment,
			Action:        actions.Parse(use.Raw),
		})
	}
	return metadata.ReconcileLockfile(lockfile, hasLockfile, reconcileUses), nil
}

func resolvePlanCandidate(ctx context.Context, resolver planResolver, parsed actions.ParsedAction, logicalRef string, unpinned policy.UnpinnedPolicy) (*githubresolver.ResolvedRef, error) {
	if resolver == nil {
		return nil, fmt.Errorf("resolver is required")
	}
	if !shouldResolvePlanCandidate(parsed, logicalRef, unpinned) {
		return nil, nil
	}

	if (parsed.Ref == "" || parsed.Pinned) && logicalRef == "" {
		switch unpinned {
		case policy.UnpinnedDefaultBranch:
			defaultBranch, ok := resolver.(defaultBranchResolver)
			if !ok {
				return nil, fmt.Errorf("resolver does not support default branch discovery")
			}
			resolved, err := defaultBranch.ResolveDefaultBranch(ctx, parsed.Owner, parsed.Repo)
			if err != nil {
				return nil, err
			}
			return &resolved, nil
		case policy.UnpinnedLatestRelease:
			latestRelease, ok := resolver.(latestReleaseResolver)
			if !ok {
				return nil, fmt.Errorf("resolver does not support latest release discovery")
			}
			resolved, err := latestRelease.ResolveLatestRelease(ctx, parsed.Owner, parsed.Repo)
			if err != nil {
				return nil, err
			}
			return &resolved, nil
		default:
			return nil, nil
		}
	}

	ref := parsed.Ref
	if logicalRef != "" {
		ref = logicalRef
	}

	resolved, err := resolver.Resolve(ctx, githubresolver.ActionSelector{
		Owner: parsed.Owner,
		Repo:  parsed.Repo,
		Ref:   ref,
	})
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}

func shouldResolvePlanCandidate(parsed actions.ParsedAction, logicalRef string, unpinned policy.UnpinnedPolicy) bool {
	if !parsed.Valid {
		return false
	}
	switch parsed.Kind {
	case actions.KindGitHubAction, actions.KindReusableWorkflow:
	default:
		return false
	}
	if parsed.Ref == "" || (parsed.Pinned && logicalRef == "") {
		return logicalRef != "" || unpinned == policy.UnpinnedDefaultBranch || unpinned == policy.UnpinnedLatestRelease
	}
	if parsed.Pinned {
		return logicalRef != ""
	}
	return true
}

func policyOptionsFromConfig(cfg config.Config, now time.Time) policy.Options {
	return policy.Options{
		Tags:              policy.TagPolicy(cfg.Updates.Tags),
		Branches:          policy.BranchPolicy(cfg.Updates.Branches),
		Unpinned:          policy.UnpinnedPolicy(cfg.Updates.Unpinned),
		ReusableWorkflows: cfg.Updates.ReusableWorkflows,
		IgnoreActions:     cfg.Ignore.Actions,
		IgnoreFiles:       cfg.Ignore.Files,
		Cooldown:          cfg.Cooldown,
		CooldownSource:    policy.CooldownSource(cfg.CooldownSource),
		Now:               now,
	}
}

func shouldCheckIgnoreBeforeResolve(parsed actions.ParsedAction) bool {
	return parsed.Kind != actions.KindLocalAction && parsed.Kind != actions.KindDockerAction
}

func ignoredDecision(parsed actions.ParsedAction, logicalRef string, match policy.IgnoreMatch) policy.Decision {
	return policy.Decision{
		Kind:       policy.DecisionSkipIgnored,
		Reason:     match.Reason(),
		CurrentSHA: currentPinnedSHA(parsed),
		LogicalRef: logicalRefForDecision(parsed, logicalRef),
	}
}

func logicalRefForDecision(parsed actions.ParsedAction, logicalRef string) string {
	if logicalRef != "" {
		return logicalRef
	}
	if !parsed.Pinned {
		return parsed.Ref
	}
	return ""
}

func planActionFromDecision(use workflow.UseNode, parsed actions.ParsedAction, candidate *githubresolver.ResolvedRef, decision policy.Decision) planAction {
	action := planAction{
		NodePath:      use.NodePath,
		Line:          use.Line,
		Column:        use.Column,
		Raw:           use.Raw,
		InlineComment: use.InlineComment,
		Owner:         parsed.Owner,
		Repo:          parsed.Repo,
		Path:          parsed.Path,
		Kind:          parsed.Kind,
		LogicalRef:    decision.LogicalRef,
		CurrentSHA:    decision.CurrentSHA,
		Decision:      decision.Kind,
		ReasonCode:    reasonCodeForDecision(decision.Kind),
		Reason:        decision.Reason,
	}
	if candidate != nil {
		action.CandidateSHA = candidate.SHA
		action.CandidateRefKind = string(candidate.Kind)
	}
	if decision.NewSHA != "" {
		action.CandidateSHA = decision.NewSHA
	}
	if decision.Age != 0 {
		action.Age = decision.Age.String()
		action.AgeSeconds = int64(decision.Age / time.Second)
	}
	return action
}

func summarizePlan(report planReport) planSummary {
	var summary planSummary
	for _, file := range report.Files {
		for _, action := range file.Actions {
			summary.Actions++
			switch action.Decision {
			case policy.DecisionUnchanged:
				if action.CurrentSHA != "" {
					summary.AlreadyPinned++
				}
			case policy.DecisionUpdate:
				summary.UpdatesAvailable++
			case policy.DecisionPending:
				summary.PendingCooldown++
			case policy.DecisionSkip, policy.DecisionSkipLocalAction, policy.DecisionSkipDockerAction, policy.DecisionSkipIgnored:
				summary.Skipped++
			}
			if isBlockingDecision(action.Decision) {
				summary.PolicyViolations++
			}
		}
	}
	return summary
}

func reasonCodeForDecision(kind policy.DecisionKind) string {
	switch kind {
	case policy.DecisionUnchanged:
		return "already-current"
	case policy.DecisionUpdate:
		return "update-available"
	case policy.DecisionPending:
		return "cooldown-active"
	case policy.DecisionSkip:
		return "skipped"
	case policy.DecisionSkipLocalAction:
		return "local-action"
	case policy.DecisionSkipDockerAction:
		return "docker-action"
	case policy.DecisionSkipIgnored:
		return "ignored-action"
	case policy.DecisionErrorInvalid:
		return "invalid-reference"
	case policy.DecisionErrorUnpinned:
		return "unpinned-reference"
	case policy.DecisionErrorShortSHA:
		return "short-sha"
	case policy.DecisionErrorTagDenied:
		return "tag-denied"
	case policy.DecisionErrorBranchDenied:
		return "branch-denied"
	case policy.DecisionErrorReusable:
		return "reusable-workflow-denied"
	case policy.DecisionErrorUnresolved:
		return "unresolved-reference"
	case policy.DecisionErrorUnsupported:
		return "unsupported-policy"
	case policy.DecisionError:
		return "error"
	default:
		return string(kind)
	}
}

func currentPinnedSHA(parsed actions.ParsedAction) string {
	if parsed.Pinned {
		return parsed.Ref
	}
	return ""
}

func writePlanJSON(path string, report planReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write plan %q: %w", path, err)
	}
	return nil
}

func printPlanTable(cmd *cobra.Command, report planReport) error {
	style := styleForCommand(cmd)
	printPlanSummary(cmd.OutOrStdout(), report.Summary, style)
	var rows []styledTableRow
	for _, file := range report.Files {
		for _, action := range file.Actions {
			rows = append(rows, planTableRow(file.Path, action))
		}
	}
	return printStyledTable(
		cmd.OutOrStdout(),
		style,
		[]string{"FILE", "LINE", "ACTION", "CURRENT", "CANDIDATE", "REF", "AGE", "DECISION", "REASON CODE", "REASON"},
		rows,
	)
}

func planTableRow(file string, action planAction) styledTableRow {
	return styledTableRow{
		{Text: file, Role: colorFile},
		{Text: fmt.Sprintf("%d", action.Line), Role: colorLine},
		{Text: planActionName(action)},
		{Text: shortSHAOrDash(action.CurrentSHA), Role: currentSHAColorRole(action.Decision, action.CurrentSHA)},
		{Text: shortSHAOrDash(action.CandidateSHA), Role: candidateSHAColorRole(action.Decision, action.CandidateSHA)},
		{Text: emptyDash(action.LogicalRef), Role: refColorRole(action.LogicalRef)},
		{Text: emptyDash(action.Age), Role: ageColorRole(action.Decision, action.Age)},
		{Text: string(action.Decision), Role: decisionColorRole(action.Decision)},
		{Text: action.ReasonCode, Role: decisionColorRole(action.Decision)},
		{Text: emptyDash(action.Reason), Role: colorReason},
	}
}

func printPlanSummary(out io.Writer, summary planSummary, style terminalStyle) {
	_, _ = fmt.Fprintln(out, style.Wrap(colorHeader, "Summary:"))
	_, _ = fmt.Fprintf(out, "  %d actions found\n", summary.Actions)
	_, _ = fmt.Fprintf(out, "  %s already pinned\n", style.Wrapf(colorSuccess, "%d", summary.AlreadyPinned))
	_, _ = fmt.Fprintf(out, "  %s updates available\n", style.Wrapf(colorWarning, "%d", summary.UpdatesAvailable))
	_, _ = fmt.Fprintf(out, "  %s pending cooldown\n", style.Wrapf(colorWarning, "%d", summary.PendingCooldown))
	_, _ = fmt.Fprintf(out, "  %s policy violations\n", style.Wrapf(colorDanger, "%d", summary.PolicyViolations))
	_, _ = fmt.Fprintf(out, "  %s skipped\n\n", style.Wrapf(colorMuted, "%d", summary.Skipped))
}

func currentSHAColorRole(kind policy.DecisionKind, value string) colorRole {
	if value == "" {
		return colorMuted
	}
	if kind == policy.DecisionUpdate {
		return colorDelete
	}
	if isBlockingDecision(kind) {
		return colorDanger
	}
	return colorSuccess
}

func candidateSHAColorRole(kind policy.DecisionKind, value string) colorRole {
	if value == "" {
		return colorMuted
	}
	switch kind {
	case policy.DecisionUpdate:
		return colorAdd
	case policy.DecisionPending:
		return colorWarning
	default:
		if isBlockingDecision(kind) {
			return colorDanger
		}
		return colorInfo
	}
}

func refColorRole(value string) colorRole {
	if value == "" {
		return colorMuted
	}
	return colorInfo
}

func ageColorRole(kind policy.DecisionKind, value string) colorRole {
	if value == "" {
		return colorMuted
	}
	if kind == policy.DecisionPending {
		return colorWarning
	}
	return colorMuted
}

func decisionColorRole(kind policy.DecisionKind) colorRole {
	switch {
	case isBlockingDecision(kind):
		return colorDanger
	}
	switch kind {
	case policy.DecisionUnchanged:
		return colorSuccess
	case policy.DecisionUpdate:
		return colorWarning
	case policy.DecisionPending:
		return colorWarning
	case policy.DecisionSkip, policy.DecisionSkipLocalAction, policy.DecisionSkipDockerAction, policy.DecisionSkipIgnored:
		return colorMuted
	default:
		return colorNone
	}
}

func planActionName(action planAction) string {
	return actionName(action.Owner, action.Repo, action.Path, action.Raw)
}

func shortSHAOrDash(value string) string {
	if value == "" {
		return "-"
	}
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
