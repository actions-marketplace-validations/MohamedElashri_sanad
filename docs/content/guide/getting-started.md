+++
title = "Getting Started"
description = "Install Sanad, pin your first workflow actions, and keep them up to date — with or without the GitHub Action."
weight = 10
template = "page"
+++

Sanad works in two complementary ways: as a **CLI** you run locally or in a `run:` step, and as a **bundled GitHub Action** that installs and invokes the same CLI automatically. Both use the same policy model.

## Install with Homebrew

On macOS or Linux with Homebrew:

```bash
brew tap MohamedElashri/sanad && brew install sanad
```

Check that it is installed:

```bash
sanad version
```

Homebrew installs shell completions for bash, zsh, fish, and PowerShell automatically.

## Install with Nix

Run the packaged release directly:

```bash
nix run github:MohamedElashri/sanad -- version
```

Or install it into your profile:

```bash
nix profile install github:MohamedElashri/sanad
sanad version
```

The flake uses the published release archives for Linux and macOS on `x86_64` and `aarch64`, with fixed hashes derived from the release checksums. Bash, zsh, and fish completions are installed automatically.

## Install from source

Install the latest tagged release with Go:

```bash
go install github.com/MohamedElashri/sanad/cmd/sanad@latest
```

## Manual prebuilt archive install

Tagged releases also publish Linux, macOS, and Windows archives on [GitHub Releases](https://github.com/MohamedElashri/sanad/releases). Download the archive for your platform and verify it against the published checksums file.

For manual archive or `go install` usage, install completions with:

```bash
sanad completion install
```

Sanad detects bash, zsh, fish, and PowerShell from your environment. You can also pass the shell explicitly:

```bash
sanad completion install bash
sanad completion install zsh
sanad completion install fish
sanad completion install powershell
```

Use `sanad completion install --dry-run` to preview what would be written, or `--no-profile` to skip updating shell profile files.

## Pin your first workflows

Run `sanad` with no arguments (or `sanad start`) to launch the interactive wizard:

```bash
sanad start
```

Sanad will:
1. Use zero-config built-in defaults — no `.sanad.toml` needed.
2. Scan `.github/workflows` and any nested `.github/workflows` directories.
3. Automatically resolve **all** action ref types: tags, branches, and completely unpinned `owner/repo` actions.
4. Preview changes and let you confirm them interactively.
5. Apply immutable SHAs and create `.github/sanad.lock.json`.

For a non-interactive initial write, use:

```bash
sanad start --write --yes
```

### Zero-config defaults

Out of the box, without any `.sanad.toml`:

- **Tagged actions** (`@v4`) are resolved to their current SHA and tracked — future `apply` runs keep the SHA current.
- **Branch-tracking actions** (`@main`) are resolved and tracked through updates automatically.
- **Completely unpinned actions** (`owner/repo`) are automatically resolved to their latest stable release and pinned.

You only need a `.sanad.toml` when you want to deviate from these defaults (e.g. stricter cooldown, upgrade constraints).

## Preview and apply updates

Preview what would change:

```bash
GITHUB_TOKEN=$(gh auth token) sanad plan
```

Preview with a file diff:

```bash
GITHUB_TOKEN=$(gh auth token) sanad apply --diff
```

Apply tracked-ref updates:

```bash
GITHUB_TOKEN=$(gh auth token) sanad apply --write --yes
```

## Check policy

Use `check` when a repository should already comply:

```bash
sanad check
```

The default check is local-only — no API calls. Use `sanad check --fresh` to resolve tracked refs and fail on eligible updates. Use `sanad check --strict` to also fail on cooldown-pending candidates.

Exit code `0` means the check passed. Exit code `1` means policy violations were found.

## Use the GitHub Action

For CI, the bundled GitHub Action is simpler than installing the CLI manually. See the [GitHub Action guide](../github-action/) for the full patterns.

Minimal usage — enforce pinning on every PR:

```yaml
- uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # sanad: ref=v7.0.1
- uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
```

Weekly auto-update PR — no scripting required:

```yaml
jobs:
  update:
    uses: MohamedElashri/sanad/.github/workflows/update-pr.yml@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
    secrets:
      token: ${{ secrets.GITHUB_TOKEN }}
```
