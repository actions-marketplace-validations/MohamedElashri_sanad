package config

import "time"

const DefaultPath = ".sanad.toml"
const DefaultCommentFormat = "sanad: ref={{ref}}"
const DefaultUpgradeLatestRelease = "github-release"
const DefaultUpgradeLevel = "major"
const DefaultUpgradeSelection = "latest-eligible"
const DefaultCooldownSource = "source"

type Config struct {
	Source         string
	PolicySources  []string
	WorkflowPaths  []string
	Cooldown       time.Duration
	CooldownSource string
	Updates        UpdatesConfig
	Ignore         IgnoreConfig
	Organization   OrganizationConfig
	Comments       CommentsConfig
	Security       SecurityConfig
	Upgrade        UpgradeConfig
}

type UpdatesConfig struct {
	Tags              string
	Branches          string
	Unpinned          string
	ReusableWorkflows bool
}

type IgnoreConfig struct {
	Actions []string
	Files   []string
}

type OrganizationConfig struct {
	PolicyFiles []string
}

type CommentsConfig struct {
	Write  bool
	Format string
}

type SecurityConfig struct {
	RequireFullSHA            bool
	RequireCommitInSourceRepo bool
	AllowPrivate              bool
	DenyForks                 bool
}

type UpgradeConfig struct {
	LatestRelease string
	Level         string
	Constraint    string
	Selection     string
	Actions       map[string]UpgradePolicy
}

type UpgradePolicy struct {
	Level      string `json:"level"`
	Constraint string `json:"constraint"`
	Selection  string `json:"selection"`
}

func Default() Config {
	return Config{
		Source: "defaults",
		// GitHub Action repositories commonly keep workflow fixtures below the
		// action package (for example action/test/integration/.github/workflows).
		// The nested-workflow sentinel is intentionally narrower than scanning
		// the whole repository, which could include dependency metadata such as
		// node_modules/.travis.yml.
		WorkflowPaths:  []string{".github/workflows", "**/.github/workflows"},
		Cooldown:       7 * 24 * time.Hour,
		CooldownSource: DefaultCooldownSource,
		Updates: UpdatesConfig{
			Tags:              "track",
			Branches:          "track",
			Unpinned:          "latest-release",
			ReusableWorkflows: true,
		},
		Ignore: IgnoreConfig{
			Actions: []string{"./*", "docker://*"},
		},
		Comments: CommentsConfig{
			Write:  true,
			Format: DefaultCommentFormat,
		},
		Security: SecurityConfig{
			RequireFullSHA:            true,
			RequireCommitInSourceRepo: true,
			AllowPrivate:              true,
			DenyForks:                 false,
		},
		Upgrade: UpgradeConfig{
			LatestRelease: DefaultUpgradeLatestRelease,
			Level:         DefaultUpgradeLevel,
			Selection:     DefaultUpgradeSelection,
			Actions:       make(map[string]UpgradePolicy),
		},
	}
}
