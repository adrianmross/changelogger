# changelogger

`changelogger` is a small, dependency-light Go CLI for chaincode release intent.

Use the GitHub Action to install the released binary in workflows:

```yaml
- uses: red-wiz/changelogger@v0.4.0
  with:
    token: ${{ secrets.PRIV_GOMOD_INSTLR_PAT }}

- run: changelogger check --base origin/main --pr
```

Initialize a repository:

```sh
changelogger init --component trqp_vdr_go
```

This writes `.changelogs/config.json`, so the component is project-local after
initialization.

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
