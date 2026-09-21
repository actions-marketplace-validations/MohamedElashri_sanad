+++
title = "CI Usage"
description = "Enforce pinned workflow dependencies and automate update pull requests using the Sanad GitHub Action."
weight = 40
template = "page"
+++

The recommended CI setup uses the bundled GitHub Action. It installs the exact matching Sanad CLI, verifies the release, and translates output into inline annotations and a rich job summary automatically.

For advanced use cases where you need direct CLI access, use `mode: setup` and invoke `sanad` in a subsequent `run:` step.

## Enforce pinning on pull requests

Add as a required status check — no `.sanad.toml` needed:

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

Use `fresh: "true"` to also resolve tracked refs and fail when an eligible update is available. Use `strict: "true"` to additionally fail on cooldown-pending candidates.

## Automatic weekly update PR

The built-in reusable workflow handles checkout, apply, commit, and PR creation — no scripting required:

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

Pass `upgrade: true` to also bump logical refs according to the repository's upgrade policy:

```yaml
jobs:
  update:
    uses: MohamedElashri/sanad/.github/workflows/update-pr.yml@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
    with:
      upgrade: true
    secrets:
      token: ${{ secrets.GITHUB_TOKEN }}
```

See the [GitHub Action guide](../github-action/) for all available inputs (`branch`, `base`, `title`, `commit-message`, `config`).

## SARIF code scanning

Use `mode: setup` to install the CLI, then run it with the SARIF format and upload the results:

```yaml
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # sanad: ref=v7.0.1

      - uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
        with:
          mode: setup
          token: ${{ secrets.GITHUB_TOKEN }}

      - name: Check and emit SARIF
        run: sanad check --format sarif > sanad.sarif

      - uses: github/codeql-action/upload-sarif@v4
        if: always()
        with:
          sarif_file: sanad.sarif
```

## Apply updates in a custom step

When you need full control over the commit and push logic:

```yaml
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # sanad: ref=v7.0.1
        with:
          fetch-depth: 0

      - name: Apply pin updates
        id: sanad
        uses: MohamedElashri/sanad@58cdb34ef4470b656c2e7bfe91d7fd5ff56cb9ec
        with:
          write: "true"
          token: ${{ secrets.GITHUB_TOKEN }}

      - name: Commit and push
        if: steps.sanad.outputs.changed == 'true'
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          git add .github/
          git commit -m "ci: update pinned actions"
          git push
```

Setting `write: "true"` without an explicit `mode` automatically defaults to `mode: apply`.
