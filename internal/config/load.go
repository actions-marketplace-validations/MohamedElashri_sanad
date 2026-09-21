package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	semver "github.com/Masterminds/semver/v3"
	"github.com/MohamedElashri/sanad/internal/actions"
)

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && path == DefaultPath {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("load config %q: %w", path, err)
	}
	if info.IsDir() {
		return Config{}, fmt.Errorf("load config %q: path is a directory", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config %q: %w", path, err)
	}

	if root, meta, err := decodeConfigData(data); err != nil {
		return Config{}, fmt.Errorf("load config %q: %w", path, err)
	} else if meta.IsDefined("organization", "policy_files") {
		for _, policyFile := range root.Organization.PolicyFiles {
			resolvedPath := policyFile
			if !filepath.IsAbs(resolvedPath) {
				resolvedPath = filepath.Join(filepath.Dir(path), resolvedPath)
			}
			policyData, err := os.ReadFile(resolvedPath)
			if err != nil {
				return Config{}, fmt.Errorf("load organization policy %q: %w", policyFile, err)
			}
			cfg, err = applyConfigData(cfg, resolvedPath, string(policyData))
			if err != nil {
				return Config{}, err
			}
			cfg.PolicySources = append(cfg.PolicySources, resolvedPath)
		}
	}

	cfg, err = applyConfigData(cfg, path, string(data))
	if err != nil {
		return Config{}, err
	}

	cfg.Source = path
	return cfg, nil
}

func applyConfigData(cfg Config, path string, data string) (Config, error) {
	raw, meta, err := decodeConfigData([]byte(data))
	if err != nil {
		return Config{}, fmt.Errorf("load config %q: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return Config{}, fmt.Errorf("load config %q: unsupported key(s): %s", path, strings.Join(keys, ", "))
	}

	if meta.IsDefined("workflow_paths") {
		if err := validateWorkflowPaths(raw.WorkflowPaths); err != nil {
			return Config{}, fmt.Errorf("load config %q: workflow_paths: %w", path, err)
		}
		cfg.WorkflowPaths = raw.WorkflowPaths
	}

	if meta.IsDefined("cooldown") {
		cooldown, err := ParseDuration(raw.Cooldown)
		if err != nil {
			return Config{}, fmt.Errorf("load config %q: cooldown: %w", path, err)
		}
		cfg.Cooldown = cooldown
	}
	if meta.IsDefined("cooldown_source") {
		cooldownSource, err := normalizeCooldownSource(raw.CooldownSource)
		if err != nil {
			return Config{}, fmt.Errorf("load config %q: %w", path, err)
		}
		cfg.CooldownSource = cooldownSource
	}

	if meta.IsDefined("updates", "tags") {
		cfg.Updates.Tags = raw.Updates.Tags
	}
	if meta.IsDefined("updates", "branches") {
		cfg.Updates.Branches = raw.Updates.Branches
	}
	if meta.IsDefined("updates", "unpinned") {
		cfg.Updates.Unpinned = raw.Updates.Unpinned
	}
	if meta.IsDefined("updates", "reusable_workflows") {
		cfg.Updates.ReusableWorkflows = raw.Updates.ReusableWorkflows
	}
	if err := validateUpdatesConfig(cfg.Updates); err != nil {
		return Config{}, fmt.Errorf("load config %q: %w", path, err)
	}

	if meta.IsDefined("ignore", "actions") {
		cfg.Ignore.Actions = raw.Ignore.Actions
	}
	if meta.IsDefined("ignore", "files") {
		cfg.Ignore.Files = raw.Ignore.Files
	}

	if meta.IsDefined("github") {
		return Config{}, fmt.Errorf("load config %q: [github] is not supported; Sanad resolves refs only through github.com", path)
	}

	if meta.IsDefined("organization", "policy_files") {
		cfg.Organization.PolicyFiles = raw.Organization.PolicyFiles
	}

	if meta.IsDefined("comments", "write") {
		cfg.Comments.Write = raw.Comments.Write
	}
	if meta.IsDefined("comments", "format") {
		if err := validateCommentFormat(raw.Comments.Format); err != nil {
			return Config{}, fmt.Errorf("load config %q: %w", path, err)
		}
		cfg.Comments.Format = raw.Comments.Format
	}

	if meta.IsDefined("security", "require_full_sha") {
		cfg.Security.RequireFullSHA = raw.Security.RequireFullSHA
	}
	if meta.IsDefined("security", "require_commit_in_source_repo") {
		cfg.Security.RequireCommitInSourceRepo = raw.Security.RequireCommitInSourceRepo
	}
	if meta.IsDefined("security", "allow_private") {
		cfg.Security.AllowPrivate = raw.Security.AllowPrivate
	}
	if meta.IsDefined("security", "deny_forks") {
		cfg.Security.DenyForks = raw.Security.DenyForks
	}
	if err := validateSecurityConfig(cfg.Security); err != nil {
		return Config{}, fmt.Errorf("load config %q: %w", path, err)
	}

	if meta.IsDefined("upgrade", "latest_release") {
		latestRelease, err := normalizeUpgradeLatestRelease(raw.Upgrade.LatestRelease)
		if err != nil {
			return Config{}, fmt.Errorf("load config %q: %w", path, err)
		}
		cfg.Upgrade.LatestRelease = latestRelease
	}
	if err := applyUpgradePolicy(&cfg.Upgrade.Level, &cfg.Upgrade.Constraint, &cfg.Upgrade.Selection, raw.Upgrade.Level, raw.Upgrade.Constraint, raw.Upgrade.Selection, meta, "upgrade"); err != nil {
		return Config{}, fmt.Errorf("load config %q: %w", path, err)
	}
	if cfg.Upgrade.Actions == nil {
		cfg.Upgrade.Actions = make(map[string]UpgradePolicy)
	}
	for selector, rawPolicy := range raw.Upgrade.Actions {
		if err := validateUpgradeActionSelector(selector); err != nil {
			return Config{}, fmt.Errorf("load config %q: upgrade.actions.%q: %w", path, selector, err)
		}
		policy := cfg.Upgrade.Actions[selector]
		if err := applyUpgradePolicy(&policy.Level, &policy.Constraint, &policy.Selection, rawPolicy.Level, rawPolicy.Constraint, rawPolicy.Selection, meta, "upgrade", "actions", selector); err != nil {
			return Config{}, fmt.Errorf("load config %q: upgrade.actions.%q: %w", path, selector, err)
		}
		cfg.Upgrade.Actions[selector] = policy
	}

	return cfg, nil
}

func validateUpdatesConfig(cfg UpdatesConfig) error {
	switch cfg.Tags {
	case "track", "pin-current", "deny":
	default:
		return fmt.Errorf("updates.tags %q is not supported; expected track, pin-current, or deny", cfg.Tags)
	}
	switch cfg.Branches {
	case "deny", "pin-current", "track":
	default:
		return fmt.Errorf("updates.branches %q is not supported; expected deny, pin-current, or track", cfg.Branches)
	}
	switch cfg.Unpinned {
	case "deny", "default-branch", "latest-release":
	default:
		return fmt.Errorf("updates.unpinned %q is not supported; expected deny, default-branch, or latest-release", cfg.Unpinned)
	}
	return nil
}

type fileConfig struct {
	WorkflowPaths  []string               `toml:"workflow_paths"`
	Cooldown       string                 `toml:"cooldown"`
	CooldownSource string                 `toml:"cooldown_source"`
	Updates        updatesFileConfig      `toml:"updates"`
	Ignore         ignoreFileConfig       `toml:"ignore"`
	Organization   organizationFileConfig `toml:"organization"`
	Comments       commentsFileConfig     `toml:"comments"`
	Security       securityFileConfig     `toml:"security"`
	Upgrade        upgradeFileConfig      `toml:"upgrade"`
}

type updatesFileConfig struct {
	Tags              string `toml:"tags"`
	Branches          string `toml:"branches"`
	Unpinned          string `toml:"unpinned"`
	ReusableWorkflows bool   `toml:"reusable_workflows"`
}

type ignoreFileConfig struct {
	Actions []string `toml:"actions"`
	Files   []string `toml:"files"`
}

type organizationFileConfig struct {
	PolicyFiles []string `toml:"policy_files"`
}

type commentsFileConfig struct {
	Write  bool   `toml:"write"`
	Format string `toml:"format"`
}

type securityFileConfig struct {
	RequireFullSHA            bool `toml:"require_full_sha"`
	RequireCommitInSourceRepo bool `toml:"require_commit_in_source_repo"`
	AllowPrivate              bool `toml:"allow_private"`
	DenyForks                 bool `toml:"deny_forks"`
}

type upgradeFileConfig struct {
	LatestRelease string                       `toml:"latest_release"`
	Level         string                       `toml:"level"`
	Constraint    string                       `toml:"constraint"`
	Selection     string                       `toml:"selection"`
	Actions       map[string]upgradePolicyFile `toml:"actions"`
}

type upgradePolicyFile struct {
	Level      string `toml:"level"`
	Constraint string `toml:"constraint"`
	Selection  string `toml:"selection"`
}

func decodeConfigData(data []byte) (fileConfig, toml.MetaData, error) {
	var raw fileConfig
	meta, err := toml.Decode(string(data), &raw)
	if err != nil {
		return fileConfig{}, toml.MetaData{}, fmt.Errorf("parse TOML: %w", err)
	}
	return raw, meta, nil
}

func ParseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("duration must not be empty")
	}

	var duration time.Duration
	var err error
	if strings.HasSuffix(value, "d") {
		days := strings.TrimSuffix(value, "d")
		if days == "" || strings.HasPrefix(days, "+") || strings.HasPrefix(days, "-") {
			return 0, fmt.Errorf("invalid duration %q", value)
		}
		n, parseErr := strconv.ParseInt(days, 10, 64)
		if parseErr != nil {
			return 0, fmt.Errorf("invalid duration %q", value)
		}
		duration = time.Duration(n) * 24 * time.Hour
	} else {
		duration, err = time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", value)
		}
	}

	if duration < 0 {
		return 0, fmt.Errorf("duration must not be negative")
	}
	return duration, nil
}

func validateWorkflowPaths(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("must include at least one path")
	}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			return fmt.Errorf("path must not be empty")
		}
		if filepath.IsAbs(path) {
			return fmt.Errorf("%q must be relative to the repository root", path)
		}
		if path == "**/.github/workflows" {
			continue
		}
		cleaned := filepath.Clean(path)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%q must stay inside the repository root", path)
		}
	}
	return nil
}

func normalizeCooldownSource(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch value {
	case DefaultCooldownSource, "upstream":
		return DefaultCooldownSource, nil
	case "first-seen":
		return "first-seen", nil
	case "":
		return "", fmt.Errorf("cooldown_source must not be empty")
	default:
		return "", fmt.Errorf("cooldown_source %q is not supported; expected %q or %q", value, DefaultCooldownSource, "first-seen")
	}
}

func validateCommentFormat(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("comments.format must not be empty")
	}
	if value != DefaultCommentFormat {
		return fmt.Errorf("comments.format %q is not supported; expected %q because sanad must be able to parse its own metadata safely", value, DefaultCommentFormat)
	}
	return nil
}

func validateSecurityConfig(cfg SecurityConfig) error {
	if !cfg.RequireFullSHA {
		return fmt.Errorf("security.require_full_sha=false is not supported; sanad always requires full 40-character SHAs")
	}
	if !cfg.RequireCommitInSourceRepo {
		return fmt.Errorf("security.require_commit_in_source_repo=false is not supported; sanad resolves and verifies commits in the referenced source repository")
	}
	if !cfg.AllowPrivate {
		return fmt.Errorf("security.allow_private=false is not supported because repository visibility is not resolved by the current CLI")
	}
	if cfg.DenyForks {
		return fmt.Errorf("security.deny_forks=true is not supported because fork lineage is not resolved by the current CLI")
	}
	return nil
}

func normalizeUpgradeLatestRelease(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch value {
	case DefaultUpgradeLatestRelease, "release":
		return DefaultUpgradeLatestRelease, nil
	case "":
		return "", fmt.Errorf("upgrade.latest_release must not be empty")
	default:
		return "", fmt.Errorf("upgrade.latest_release %q is not supported; expected %q", value, DefaultUpgradeLatestRelease)
	}
}

func applyUpgradePolicy(level *string, constraint *string, selection *string, rawLevel string, rawConstraint string, rawSelection string, meta toml.MetaData, path ...string) error {
	hasLevel := meta.IsDefined(append(path, "level")...)
	hasConstraint := meta.IsDefined(append(path, "constraint")...)
	if hasLevel && hasConstraint {
		return fmt.Errorf("%s.level and %s.constraint are mutually exclusive", strings.Join(path, "."), strings.Join(path, "."))
	}
	if hasLevel {
		value, err := normalizeUpgradeLevel(rawLevel)
		if err != nil {
			return err
		}
		*level, *constraint = value, ""
	}
	if hasConstraint {
		value := strings.TrimSpace(rawConstraint)
		if value == "" {
			return fmt.Errorf("%s.constraint must not be empty", strings.Join(path, "."))
		}
		if _, err := semver.NewConstraint(value); err != nil {
			return fmt.Errorf("%s.constraint %q is invalid: %w", strings.Join(path, "."), value, err)
		}
		*constraint, *level = value, ""
	}
	if meta.IsDefined(append(path, "selection")...) {
		value, err := normalizeUpgradeSelection(rawSelection)
		if err != nil {
			return err
		}
		*selection = value
	}
	return nil
}

func normalizeUpgradeLevel(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "major", "minor", "patch":
		return value, nil
	case "":
		return "", fmt.Errorf("upgrade level must not be empty")
	default:
		return "", fmt.Errorf("upgrade level %q is not supported; expected major, minor, or patch", value)
	}
}

func normalizeUpgradeSelection(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "latest-eligible", "latest":
		return value, nil
	case "":
		return "", fmt.Errorf("upgrade selection must not be empty")
	default:
		return "", fmt.Errorf("upgrade selection %q is not supported; expected latest-eligible or latest", value)
	}
}

func validateUpgradeActionSelector(selector string) error {
	selector = strings.TrimSpace(selector)
	if selector == "" || strings.Contains(selector, "@") {
		return fmt.Errorf("selector must be owner/repo[/path] without @ref")
	}
	parsed := actions.Parse(selector + "@v1.0.0")
	if !parsed.Valid || parsed.Owner == "" || parsed.Repo == "" {
		return fmt.Errorf("selector must be a valid owner/repo[/path]")
	}
	return nil
}
