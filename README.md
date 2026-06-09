# changelogger

`changelogger` is a small, dependency-light Go CLI for explicit release intent.

Use the GitHub Action to install the released binary and run changelogger in workflows:

```yaml
- uses: adrianmross/changelogger@v0.8.0
  with:
    command: check
    args: --base origin/main --pr
```

Initialize a repository:

```sh
changelogger init
```

This writes `.changelogs/config.json`, so the component is project-local after
initialization. By default the component is read from `package.json` at
`$.name` when that file exists. Otherwise it is inferred from the git remote
repository name, then the current folder name. Use `--component <name>` only
when the Release Please component should differ from the project metadata.

Single package config can either store a literal component:

```json
{
  "component": "example-service"
}
```

or point at the source of truth:

```json
{
  "component": {
    "source": "package.json",
    "jsonPath": "$.name"
  }
}
```

Monorepos can define package entries and select them with `--package`:

```json
{
  "packages": {
    "example-service": {
      "path": "services/example-service",
      "component": {
        "source": "package.json",
        "jsonPath": "$.name"
      }
    }
  }
}
```

The source path is relative to the package path. Literal components remain
supported for projects whose release component is intentionally different from
their metadata.

Developers add explicit changelog fragments. Fragment files use three-word
random slugs, for example `.changelogs/amber-matrix-river.md`.

```sh
changelogger new
```

CI validates the fragment and the PR title that Release Please will consume:

```sh
changelogger check --base origin/main --pr \
  --pr-title "$PR_TITLE" \
  --pr-body "$PR_BODY"
```

Release workflows use the same binary to remove consumed fragments from the
Release Please PR and to decide whether a merged release PR should be tagged for
GoReleaser:

```sh
changelogger consume
changelogger release-tag \
  --version-file package.json \
  --manifest-file .release-please-manifest.json
```

The action installs the released binary and, when `command` is set, runs that
command in the same step. Omit `command` when a workflow only needs
`changelogger` added to `PATH`.
