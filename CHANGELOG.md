# Changelog

All notable changes to Sanad are documented here.

## 0.3.11 - 2026-09-17

### Changed 
- The README.md is updated to focus on the action usage (needed a new release for that)

## 0.3.10 - 2026-09-17

### Fixed
- Fix action behavior to align with the new sanad CLI workflow


## 0.3.9 - 2026-09-17

### Fixed 
- Fix compiled bundle that bakes package.json's version at build time.

## 0.3.8 - 2026-09-17

### Fixed

- Fixed nested workflow discovery (`**/.github/workflows`) scanning into hidden directories such as `.gomodcache`, `.gocache`, `.gopath`, and `.cache` that are kept at the repository root. All hidden directories except `.github` are now skipped, matching the intent of the glob.
- Release CI now validates the `VERSION` file against the git tag in addition to `action/package.json`, ensuring all three sources stay in sync.

## 0.3.7 - 2026-09-17

### Added

- Added branch tracking support, allowing you to track branches in addition to tags and unpinned actions.

### Changed

- Updated default branch policy from `deny` to `track`.
- Updated default unpinned action policy from `latest` to `latest-release`.

## 0.3.6 - 2026-09-15

### Added

- Added support for discovering conventional nested `.github/workflows` directories, allowing action repositories to scan workflow fixtures under directories such as `action/test/integration` without scanning unrelated YAML files.
- Added `make update` to update Go dependencies and tidy the module files.

### Changed

- Expanded the default workflow scope to include `.github/workflows` directories nested in the repository, with matching configuration and documentation updates.
- Updated Go dependencies, including `golang.org/x/oauth2`, `github.com/google/go-querystring`, and `github.com/spf13/pflag`.

### Fixed

- Made nested workflow discovery handle both slash and backslash path separators on Windows.

## 0.3.5 - 2026-08-29

### Fixed

- Use a unique Marketplace display name: `Sanad Dependency Guard`.

## 0.3.4 - 2026-08-29

### Fixed

- Align the action package version with the release tag so release validation cannot publish a mismatched action.
- Verify published release state without requiring GitHub artifact attestations that GoReleaser does not generate.

## 0.3.1 - 2026-08-29

### Fixed

- Kept the Node action's CommonJS dependencies exact and verified them during CI and release builds.
- Added explicit release-state diagnostics so mutable releases fail before the action can consume them.

## 0.3.0 - 2026-08-29

### Added

- Added the bundled Node 24 GitHub Action with check, plan, apply, upgrade, setup, annotations, job summaries, and structured outputs.
- Added versioned JSON Schemas and serialization compatibility tests for automation reports.

### Changed

- Release publishing now stages assets in a draft and verifies that the published release is immutable before continuing.
- `apply --write --format json` now preserves machine-readable output and can write a pull request body with `--pr-body-out`.

## 0.2.5 - 2026-08-29

### Added

- Added local, token-free policy checks by default, with `check --fresh` and `check --strict` for update freshness enforcement.
- Added strict `config validate` and effective `config show --origins` commands.
- Added repository-root discovery, `--root`, GitHub CLI authentication fallback, and the consolidated `doctor` health and repair workflow.

### Changed

- Updated the pinned Staticcheck toolchain to v0.8.1 for Go 1.27 export-data support and made the local install stamp Go-toolchain-specific.
- Made apply, upgrade, and lockfile mutations preview by default and require `--write --yes` for non-interactive writes.
- Made `start` use built-in defaults without generating `.sanad.toml`; terminal initialization remains interactive.
- Made workflow and lockfile updates a coordinated transaction with rollback on commit failure.
- Rejected unknown configuration keys and invalid update-policy values during config loading.

## 0.2.0 - 2026-07-22

### Added

- Introduced the `sanad start` command to streamline initialization. It automatically generates a default `.sanad.toml` configuration and applies default settings to your workflows in a single step.

### Changed

- **BREAKING CHANGE**: Simplified the CLI by flattening the command structure. The grouped namespaces (`audit scan`, `update apply`, etc.) have been removed in favor of top-level commands (`sanad scan`, `sanad apply`, etc.).
- Improved the default behavior: Running `sanad` with no arguments now checks for `.sanad.toml`. If found, it defaults to running `sanad check`. If missing, it provides a helpful message to run `sanad start`.

## 0.1.7 - 2026-07-22

### Changed

- Reduced the default cooldown from 14 days to 7 days in code, configuration examples, and documentation.

### Dependencies

- Updated `github.com/Masterminds/semver/v3` from 3.4.0 to 3.5.0.

### Chores

- Updated managed workflow pins: `actions/checkout` to v6.0.3 and `goreleaser/goreleaser-action` to v7.2.2.

## 0.1.6 - 2026-06-24

### Changed

- Made workflow file diffs opt-in for apply and upgrade commands through `--diff`; default output now stays at the summary and decision-table level.



## 0.1.5 - 2026-06-24

### Added

- Added SemVer upgrade levels, arbitrary constraints, per-action overrides, and `latest` versus `latest-eligible` release selection.
- Added paginated stable GitHub release discovery and audit details for every newer candidate considered by `sanad update upgrade`.

### Changed

- Made bare `sanad update upgrade` choose the highest cooldown-eligible stable release instead of always waiting for GitHub's single latest release.
- Upgraded the lockfile to version 2 with multi-candidate first-seen history and backward migration from version 1.

### Fixed

- Prevented audit commands, scoped update operations, and `lock repair` from dropping unrelated lockfile entries. Entry deletion now requires explicit `lock refresh` or `lock prune --write` intent.

## 0.1.4 - 2026-06-14

### Added

- Added shell completion generation and user-level completion installation for bash, zsh, fish, and PowerShell.
- Added `sanad audit`, `sanad update`, and `sanad lock` command groups. The canonical workflow commands are now `sanad audit scan`, `sanad audit plan`, `sanad audit check`, `sanad update apply`, and `sanad update upgrade`.
- Added `sanad lock status`, `sanad lock refresh`, `sanad lock repair`, and `sanad lock prune` for inspecting and safely repairing stale `.github/sanad.lock.json` entries after Dependabot or manual workflow edits.

### Changed

- Kept `sanad scan`, `sanad plan`, `sanad check`, `sanad apply`, and `sanad upgrade` as hidden compatibility aliases for the migration period. New docs and completion output prefer the nested commands.
- Reconciled safe lockfile drift from current workflow content and inline `sanad` comments instead of requiring users to delete the lockfile when a valid stale entry can be repaired.

## 0.1.3 - 2026-05-19

### Security

- Removed repository-configurable custom GitHub API endpoints. Sanad now resolves refs through `github.com` only, so repository config cannot redirect CI tokens to another host.
- Added lockfile pin-drift detection so a managed workflow SHA that no longer matches `.github/sanad.lock.json` is reported as invalid metadata instead of being treated as a normal managed update.
- Restricted configured `workflow_paths` to relative in-repository paths and rejected workflow YAML symlinks during discovery.
- Made `sanad check` fail by default when a managed pin has an eligible update or cooldown-pending candidate. Use `--allow-pending-cooldown` only when pending managed candidates should be allowed in CI.

### Added

- Added `cooldown_source` with the default `source` mode for upstream release/tag/commit timestamps and an optional stricter `first-seen` mode based on `.github/sanad.lock.json` candidate observation time.
- Added `candidate_sha` and `candidate_seen_at` lockfile fields to record pending managed candidates for auditability and first-seen cooldown mode.
- Added colorized table output with CLI and environment controls for color behavior.

### Changed

- Made bare `sanad upgrade` a safe dry run by default, using all managed pins and latest GitHub releases unless explicit selector or target flags are passed.
- Add colors to cli output to make better UX.

### Fixed

- Improved command error reporting so CLI errors are printed consistently.

## 0.1.2 - 2026-05-19

### Security

- Stopped sending `GITHUB_TOKEN` or `GH_TOKEN` to custom `[github].api_url` endpoints by default. GitHub Enterprise users can opt in explicitly with `[github].send_token_to_custom_api_url = true`.
- Hardened `[github].api_url` validation so custom GitHub API endpoints must use HTTPS and cannot include embedded credentials.

### Fixed

- Detected lockfile entries that no longer match the workflow action at the same YAML node, reporting a metadata conflict instead of applying stale tracking metadata to a different action.
- Made `sanad plan`, `sanad apply`, and `sanad upgrade` handle lockfile and inline-comment metadata recovery consistently.

## 0.1.1 - 2026-05-18

### Added

- Added full TOML parsing through `github.com/BurntSushi/toml`, including richer nested config support and overlay behavior.
- Added configurable metadata comments through `[comments].write` and validated comment format handling.
- Added fail-closed `[security]` config validation for strict full-SHA and source-repository behavior.
- Added unpinned action discovery modes for `updates.unpinned = "default-branch"` and `updates.unpinned = "latest-release"`.
- Added `sanad upgrade` for intentionally moving managed pins from one logical ref to another.
- Added optional interactive persistence for branch tracking choices.
- Added a Nida-powered documentation site and generated release notes from this changelog.
- Added Dependabot configuration and pinned the repository's own workflow actions with Sanad metadata.
- Added Homebrew tap publishing through GoReleaser for the external `homebrew-sanad` tap.
- Added a Nix flake that installs verified Sanad release archives on Linux and macOS.


## v0.1.0 - 2026-05-18

First public release.

### Added

- Added `sanad scan`, `plan`, `check`, `apply`, and `version`.
- Added GitHub Actions `uses:` discovery for workflow files, reusable workflows, local actions, Docker actions, invalid references, short SHAs, and full SHA pins.
- Added GitHub ref resolution for tags, branches, and full SHAs.
- Added cooldown-aware update planning so newly moved refs are not adopted immediately.
- Added byte-preserving workflow rewrites with inline `sanad: ref=...` metadata.
- Added `.github/sanad.lock.json` metadata support for deterministic tracking.
- Added JSON, table, SARIF, unified diff, and pull request body outputs.
- Added interactive apply flows for unpinned actions, unmanaged full-SHA pins, and denied branch refs.
- Added update policies, ignore rules, organization policy files, GitHub API endpoint configuration, and strict default security behavior.
- Added GoReleaser packaging for Linux, macOS, and Windows archives with checksums.

### Notes

- `v0.1.0` is intentionally focused on GitHub Actions workflow dependencies. It is not a general dependency updater or vulnerability scanner.
