+++
title = "Configuration"
description = "Configure workflow discovery, update policy, cooldown, metadata comments, and strict security behavior."
weight = 20
template = "page"
+++

Sanad loads `.sanad.toml` by default. If that file is missing, built-in defaults are used.

Start with the defaults and add config only for policy choices your repository needs:

```toml
cooldown = "14d"

[upgrade]
level = "minor"
```

Validate and inspect the merged configuration with `sanad config validate` and `sanad config show --origins`. Unknown keys and unsupported policy values are rejected.

## Common settings

`workflow_paths` controls which workflow files or directories are scanned. Configured paths must be relative and stay inside the repository root.

`cooldown` controls how old a resolved candidate must be before Sanad can adopt it. The default is `7d`.

`cooldown_source = "source"` uses upstream release, tag, or commit timestamps and is the default. Set `cooldown_source = "first-seen"` to use the time Sanad first recorded a candidate SHA locally.

`updates.tags = "track"` means refs like `actions/checkout@v4` are resolved, pinned, and tracked through metadata.

`updates.branches = "track"` is the default, ensuring that branches (like `@main`) are seamlessly resolved to commits and continuously tracked for updates.

`updates.unpinned = "latest-release"` is the default, allowing Sanad to automatically discover the best target for completely unpinned `owner/repo` actions without requiring manual configuration.

`comments.write = false` disables inline `sanad: ref=...` comments. The lockfile remains the metadata source.

`upgrade.level` limits automatic release changes to `major`, `minor`, or `patch`. Use `upgrade.constraint` instead for an absolute SemVer range. `upgrade.selection = "latest-eligible"` selects the highest mature release; `latest` waits for the newest matching release. Add `[upgrade.actions."owner/repo"]` tables for action-specific overrides.

## GitHub API

Sanad resolves refs through `github.com` only. Custom GitHub API endpoints are intentionally unsupported so repository config cannot redirect CI credentials to another host.

For all supported keys and validation behavior, see [Config Reference](../../reference/config/).
