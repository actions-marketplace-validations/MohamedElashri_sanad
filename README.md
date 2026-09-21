<div align="center">
  <img src="docs/static/logo.svg" alt="Sanad logo" width="128" height="128">
  <h1>Sanad</h1>
  <p><strong>Pin GitHub Actions to immutable SHAs, then keep the refs you trust moving.</strong></p>
  <p>
    <a href="https://github.com/MohamedElashri/sanad/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/MohamedElashri/sanad/actions/workflows/ci.yml/badge.svg"></a>
    <a href="https://github.com/MohamedElashri/sanad/actions/workflows/pages.yml"><img alt="Docs" src="https://github.com/MohamedElashri/sanad/actions/workflows/pages.yml/badge.svg?label=docs"></a>
    <a href="https://github.com/MohamedElashri/sanad/actions/workflows/release.yml"><img alt="Release workflow" src="https://github.com/MohamedElashri/sanad/actions/workflows/release.yml/badge.svg"></a>
    <a href="https://github.com/MohamedElashri/sanad/releases"><img alt="Latest release" src="https://img.shields.io/github/v/release/MohamedElashri/sanad?sort=semver&display_name=tag&logo=github"></a>
    <a href="https://pkg.go.dev/github.com/MohamedElashri/sanad"><img alt="Go reference" src="https://pkg.go.dev/badge/github.com/MohamedElashri/sanad.svg"></a>
    <a href="https://goreportcard.com/report/github.com/MohamedElashri/sanad"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/MohamedElashri/sanad"></a>
    <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/MohamedElashri/sanad"></a>
  </p>
  <p>
    <a href="https://melashri.net/sanad/">Documentation</a>
    |
    <a href="#github-action">GitHub Action</a>
    |
    <a href="#installation">CLI Installation</a>
    |
    <a href="#quickstart">Quickstart</a>
    |
    <a href="docs/content/reference/cli.md">CLI reference</a>
    |
    <a href="docs/content/advanced/security-model.md">Security model</a>
  </p>
</div>

**sanad** pins and updates GitHub Actions dependencies to immutable commit SHAs while preserving the logical refs you want to track. Branches, tags, and completely unpinned actions are all handled automatically — no configuration file required.

## Example

Before:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
```

After `sanad` runs:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # sanad: ref=v4
      - uses: actions/setup-go@93397bea11091df50f3d7e59dc26a7711a8bcfbe # sanad: ref=v5
```

The workflow executes immutable SHAs. The comments and lockfile tell `sanad` which logical refs to resolve on future runs — so the next time a new `v4` patch is tagged, sanad updates the SHA automatically.

---

## GitHub Action

The bundled GitHub Action is the recommended way to use sanad in CI. It installs the exact matching sanad binary, verifies it, and translates sanad's output into annotations, job summaries, and step outputs.

Pin it to a full commit SHA (find the latest on the [releases page](https://github.com/MohamedElashri/sanad/releases)):

```yaml
- uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
```

### Pattern 1 — Enforce pinning on every PR

Fail the build if any action is unpinned or stale. Add this as a required status check.

```yaml
name: Check pinned actions

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  sanad:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
      - uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
        with:
          mode: check
          token: ${{ secrets.GITHUB_TOKEN }}
```

`check` is the default when `write` is not set, so `mode: check` can be omitted entirely.

### Pattern 2 — Automatic weekly update PR

Run on a schedule, apply pin updates, and open a pull request when anything changed. Uses the built-in reusable workflow — no extra scripting required.

```yaml
name: Update pinned actions

on:
  schedule:
    - cron: "0 3 * * 1"   # every Monday at 03:00 UTC
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  update:
    uses: MohamedElashri/sanad/.github/workflows/update-pr.yml@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
    secrets:
      token: ${{ secrets.GITHUB_TOKEN }}
```

The reusable workflow checks out your repository, runs `sanad apply --write`, optionally runs `sanad upgrade`, commits to a dedicated branch, and opens or updates a pull request. See [`update-pr.yml`](.github/workflows/update-pr.yml) for all available inputs (`branch`, `base`, `title`, `commit-message`, `upgrade`).

### Pattern 3 — Apply updates inside a custom workflow step

Use the action directly when you need full control over what happens before or after pinning.

```yaml
- name: Apply sanad pin updates
  id: sanad
  uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
  with:
    mode: apply
    write: "true"
    token: ${{ secrets.GITHUB_TOKEN }}

- name: Show changed files
  if: steps.sanad.outputs.changed == 'true'
  run: echo "${{ steps.sanad.outputs.changed-files }}"
```

`mode: apply` with `write: "true"` is also the smart default when you supply `write: "true"` and omit `mode`, so the above is equivalent to just setting `write: "true"`.

### Pattern 4 — Install sanad for use in later steps

```yaml
- uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
  with:
    mode: setup
    token: ${{ secrets.GITHUB_TOKEN }}

- run: sanad plan --format json | jq '.files[].actions[]'
```

`setup` adds the `sanad` binary to `PATH` so you can invoke it freely in subsequent `run:` steps.

### Action inputs

| Input | Default | Description |
| --- | --- | --- |
| `mode` | `check` (or `apply` when `write: "true"`) | `check`, `plan`, `apply`, `upgrade`, or `setup` |
| `config` | `.sanad.toml` | Config path relative to `working-directory`. May be absent — built-in defaults handle branches and unpinned actions automatically. |
| `working-directory` | `.` | Repository directory relative to `GITHUB_WORKSPACE` |
| `fresh` | `false` | Resolve tracked refs during `check` and fail on eligible updates |
| `strict` | `false` | Also fail cooldown-pending candidates; implies `fresh` |
| `write` | `false` | Permit `apply` or `upgrade` to modify managed files |
| `token` | `github.token` | Token for release download and GitHub ref resolution |

### Action outputs

| Output | Description |
| --- | --- |
| `passed` | `"true"` when the command succeeded and policy passed |
| `changed` | `"true"` when a write changed a managed file |
| `changed-files` | JSON array of changed managed paths |
| `updates` | Number of available or applied updates |
| `violations` | Policy violation count |
| `pending-cooldown` | Candidates still inside cooldown |
| `report-path` | Temporary path to the complete JSON report |
| `pr-body-path` | Temporary Markdown PR body path (plan and apply) |
| `sanad-version` | Exact CLI version validated and executed |

See the [GitHub Action README](action/README.md) for the full permissions, boundaries, and local development reference.

---

## Installation

### Homebrew

On macOS or Linux with Homebrew:

```bash
brew tap MohamedElashri/sanad && brew install sanad
```

Verify:

```bash
sanad version
```

Homebrew installs shell completions for bash, zsh, fish, and PowerShell automatically.

### Nix

Run the packaged release directly:

```bash
nix run github:MohamedElashri/sanad -- version
```

Or install it into your profile:

```bash
nix profile install github:MohamedElashri/sanad
```

The flake installs the published release archive for your platform and verifies it with the release checksum. The Nix package installs bash, zsh, and fish completions automatically.

### Go

Install the latest tagged release with Go:

```bash
go install github.com/MohamedElashri/sanad/cmd/sanad@latest
```

### Prebuilt archives

Tagged releases publish Linux, macOS, and Windows archives on GitHub Releases. Download the archive for your platform, place the `sanad` binary on your `PATH`, and verify it against the published `sanad_<version>_checksums.txt` file.

For manual archive or `go install` usage, install completions for your current shell with:

```bash
sanad completion install
```

---

## Quickstart

The easiest way to initialize sanad and pin your workflows is to run:

```bash
sanad start
```

This is equivalent to running `sanad` with no arguments — both launch the interactive wizard that scans your workflows, resolves all action references, and applies immutable SHAs.

For a non-interactive initial write:

```bash
sanad start --write --yes
```

### Common local commands

```bash
# Preview what would change
GITHUB_TOKEN=$(gh auth token) sanad plan

# Preview with a file diff
GITHUB_TOKEN=$(gh auth token) sanad apply --diff

# Apply locally
GITHUB_TOKEN=$(gh auth token) sanad apply --write --yes

# Validate current state (local, no API calls)
sanad check

# Diagnose and repair stale lockfile entries
sanad doctor
sanad doctor --write --yes
```

`sanad plan`, `sanad apply`, `sanad upgrade`, and `sanad check --fresh` contact GitHub. `sanad check`, `sanad scan`, `sanad doctor`, and the `lock` commands are local-only.

---

## Scope

Sanad scans workflow files under `.github/workflows` and nested conventional `.github/workflows` directories by default, classifies `uses:` references, resolves GitHub tags and branches through the GitHub API, rewrites mutable action refs to full SHAs, adds `# sanad: ref=...` metadata, maintains `.github/sanad.lock.json`, applies cooldown rules, and emits table, JSON, SARIF, and Markdown helper output.

It is not a general dependency updater, vulnerability scanner, workflow formatter, YAML linter, Docker image updater, or local action rewriter.

In Arabic scholarly culture, a sanad is a chain of transmission back to a source. This tool keeps that chain explicit for workflow dependencies: the workflow runs an immutable commit, while metadata records the tag or branch that commit came from.

## Configuration

Create `.sanad.toml` only when the built-in defaults are not enough:

```toml
cooldown = "14d"

[upgrade]
level = "minor"
```

By default branches are tracked (`[updates].branches = "track"`) and completely unpinned actions are resolved to their latest release (`[updates].unpinned = "latest-release"`). Set `[comments].write = false` to rely on `.github/sanad.lock.json` without inline `sanad` comments. See the [config reference](docs/content/reference/config.md) for all supported keys.

## Commands

```bash
sanad scan
sanad plan
sanad check
sanad apply
sanad upgrade
sanad doctor
sanad lock status
sanad lock refresh
sanad lock repair
sanad lock prune
sanad config validate
sanad config show --origins
sanad completion
sanad version
```

All commands accept `--config`, `--format`, and `--root`. `sanad check --format sarif` emits SARIF for code scanning. `sanad upgrade --action actions/checkout --to v5` moves one managed pin to a specific logical ref.

Command-specific usage is covered in the [CLI reference](docs/content/reference/cli.md).

## GitHub Authentication

`sanad` reads tokens from environment variables:

1. `GITHUB_TOKEN`
2. `GH_TOKEN`

If neither is set, sanad reuses `gh auth token` when the GitHub CLI is installed and authenticated. Tokens are used for GitHub API requests and are never printed. Public repositories can work without a token, but authenticated requests are strongly recommended for CI and private repositories.

## Security Model

Workflow dependencies run immutable full-length SHAs. Mutable tags and branches are automatically resolved to commits and tracked; short SHAs are rejected; local and Docker actions are skipped by default. Unpinned actions are automatically resolved to their latest release. Strict policies (denying branch tracking, requiring manual approval for unpinned actions) are available via `.sanad.toml` for advanced use cases.

See the [security model](docs/content/advanced/security-model.md) for the full model.

## Cooldown

The default cooldown is `7d`. Automatic upgrades select the highest matching release that has satisfied this window. Set `cooldown_source = "first-seen"` for the stricter mode: sanad records candidate histories in the lockfile and waits for the local observation window before adopting them.

## Development

```bash
make build
make test
make lint
make docs-build
```

The documentation site is built with Nida from `docs/`. User docs live under `docs/content/guide`, exact lookup pages under `docs/content/reference`, and contributor/internal docs under `docs/content/advanced`.

## LICENCE

This project is released under the [MIT LICENCE](./LICENSE)
