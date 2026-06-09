# changelogger

`changelogger` is a small, dependency-light Go CLI for chaincode release intent.

Use the GitHub Action to install the released binary in workflows:

```yaml
- uses: red-wiz/changelogger@v0.6.0
  with:
    token: ${{ secrets.PRIV_GOMOD_INSTLR_PAT }}

- run: changelogger check --base origin/main --pr
```

Initialize a repository:

```sh
changelogger init
```

This writes `.changelogs/config.json`, so the component is project-local after
initialization. By default the component is read from `.ochain.json` at
`$.name` when that file exists. Otherwise it is inferred from the git remote
repository name, then the current folder name. Use `--component <name>` only
when the Release Please component should differ from the project metadata.

Single package config can either store a literal component:

```json
{
  "component": "trqp_vdr_go"
}
```

or point at the source of truth:

```json
{
  "component": {
    "source": ".ochain.json",
    "jsonPath": "$.name"
  }
}
```

Monorepos can define package entries and select them with `--package`:

```json
{
  "packages": {
    "trqp_vdr_go": {
      "path": "chaincodes/trqp_vdr_go",
      "component": {
        "source": ".ochain.json",
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
  --version-file .ochain.json \
  --manifest-file .release-please-manifest.json
```

The action is only a setup wrapper: it resolves a tagged changelogger release,
downloads the matching binary asset, and puts the binary on `PATH`. The CLI
release remains the published artifact that owns validation, fragment creation,
release PR cleanup, and tag intent.
