+++
title = "GitHub Action"
description = "Enforce pinning, apply updates, and open automatic pull requests using the Sanad GitHub Action."
weight = 35
template = "page"
+++

The Sanad action installs the exact CLI release associated with the action tag, verifies its immutable GitHub release and SHA-256 asset digest, runs Sanad, and translates its JSON report into inline annotations, a rich job summary, and step outputs.

No `.sanad.toml` is required. Branches, tags, and completely unpinned actions are handled automatically by the built-in defaults.

## Pattern 1 — Enforce pinning on every PR

Add as a required status check to block merges when any action is unpinned or stale:

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
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # sanad: ref=v7.0.1
      - uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
```

`mode: check` is the default when `write` is not set and can be omitted entirely.

Use `fresh: "true"` to resolve tracked refs and fail on eligible updates. Use `strict: "true"` to also fail on candidates still inside cooldown:

```yaml
      - uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
        with:
          fresh: "true"
          token: ${{ secrets.GITHUB_TOKEN }}
```

## Pattern 2 — Automatic weekly update PR

Uses the built-in reusable workflow — no scripting, no manual commit logic:

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

The reusable workflow:
1. Checks out the caller repository.
2. Runs `sanad apply --write` to refresh all tracked-ref SHAs.
3. Optionally runs `sanad upgrade` when `upgrade: true` is passed.
4. Commits changes to a dedicated branch (`sanad/update-action-pins` by default).
5. Creates or updates a pull request.

Available inputs:

| Input | Default | Description |
| --- | --- | --- |
| `branch` | `sanad/update-action-pins` | Branch for the update PR |
| `base` | repository default branch | PR base branch |
| `title` | `ci: update pinned GitHub Actions` | PR title |
| `commit-message` | `ci: update pinned GitHub Actions` | Commit message |
| `upgrade` | `false` | Also run `sanad upgrade` to move logical refs |
| `config` | `.sanad.toml` | Sanad config path |

Pass a narrowly scoped GitHub App token or PAT if the default token cannot trigger downstream workflows:

```yaml
    secrets:
      token: ${{ secrets.SANAD_UPDATE_TOKEN }}
```

## Pattern 3 — Apply updates inside a custom workflow step

For full control over what happens before or after the update:

```yaml
- name: Apply sanad pin updates
  id: sanad
  uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
  with:
    mode: apply
    write: "true"
    token: ${{ secrets.GITHUB_TOKEN }}

- name: Commit if anything changed
  if: steps.sanad.outputs.changed == 'true'
  run: |
    git config user.name "github-actions[bot]"
    git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
    git add .github/
    git commit -m "ci: update pinned actions"
    git push
```

> **Tip:** Setting `write: "true"` without an explicit `mode` automatically defaults `mode` to `apply` — so the above is equivalent to just setting `write: "true"`.

## Pattern 4 — Install sanad for use in later steps

```yaml
- uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
  with:
    mode: setup
    token: ${{ secrets.GITHUB_TOKEN }}

- run: sanad plan --format json | jq '.files[].actions[]'
- run: sanad check --format sarif > sanad.sarif

- uses: github/codeql-action/upload-sarif@v4
  if: always()
  with:
    sarif_file: sanad.sarif
```

## Modes

| Mode | Writes files | Description |
| --- | --- | --- |
| `check` | No | Validate policy. Default when `write` is not set. |
| `plan` | No | Resolve refs and produce JSON and Markdown plan files. |
| `apply` | Opt-in (`write: "true"`) | Refresh tracked-ref SHAs. Default when `write: "true"` is set. |
| `upgrade` | Opt-in (`write: "true"`) | Move managed logical refs according to `[upgrade]` policy. |
| `setup` | No | Install Sanad and add it to `PATH` for later `run:` steps. |

## Inputs

| Input | Default | Description |
| --- | --- | --- |
| `mode` | `check` (or `apply` when `write: "true"`) | `check`, `plan`, `apply`, `upgrade`, or `setup` |
| `config` | `.sanad.toml` | Config path relative to `working-directory`. May be absent — built-in defaults apply. |
| `working-directory` | `.` | Repository directory relative to `GITHUB_WORKSPACE` |
| `fresh` | `false` | Resolve tracked refs during `check` and fail on eligible updates |
| `strict` | `false` | Also fail cooldown-pending candidates during `check`; implies `fresh` |
| `write` | `false` | Permit `apply` or `upgrade` to modify managed files |
| `token` | `github.token` | Token for release download and GitHub ref resolution |

## Outputs

| Output | Description |
| --- | --- |
| `passed` | `"true"` when the command succeeded and, for `check`, policy passed |
| `changed` | `"true"` when a write changed a managed file |
| `changed-files` | JSON array of managed paths changed by the action |
| `updates` | Number of available or applied updates |
| `violations` | Policy violation count (or blocked count for `upgrade`) |
| `pending-cooldown` | Candidates still inside cooldown |
| `report-path` | Temporary path to the complete JSON report |
| `pr-body-path` | Temporary Markdown PR body path (for `plan` and `apply`) |
| `sanad-version` | Exact CLI version validated and executed |

Outputs receive safe defaults even when setup, input validation, installation, or report parsing fails.

## Job summary

The action writes a rich job summary after each run:

- **check**: ✅/❌ status line, metrics table, and a table of all violations (file, line, action, decision, reason).
- **plan / apply**: ✅/❌ status, metrics, a table of every update (action, ref, old SHA → new SHA), and any violations.
- **upgrade**: ✅/❌ status, a table of upgraded refs, and any blocked entries.

## Boundaries

- GitHub.com is supported. GitHub Enterprise Server is not currently supported.
- Supported runners: Linux, macOS, and Windows on amd64 and arm64.
- The core action never checks out, commits, pushes, creates branches, or opens pull requests. Use the reusable `update-pr.yml` for the full PR workflow.
- `write: "true"` only modifies the checked-out workspace.
- Fork pull requests should use `check` mode — never a writing mode.
- Cross-repository private actions require a token that can read their source repositories.
- Arbitrary CLI arguments are not accepted. Use `mode: setup` and a subsequent `run:` step for advanced commands.

The action's [standalone README](https://github.com/MohamedElashri/sanad/blob/main/action/README.md) contains the full local development and test procedure.
