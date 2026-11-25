# terraform-provider-workflows

Reusable GitHub Actions workflows shared by Namecheap's Terraform providers:

- [terraform-provider-spaceship](https://github.com/namecheap/terraform-provider-spaceship)
- [terraform-provider-namecheap](https://github.com/namecheap/terraform-provider-namecheap)
- [terraform-provider-jenkins](https://github.com/namecheap/terraform-provider-jenkins)

| Workflow | Purpose |
|---|---|
| [`release.yml`](.github/workflows/release.yml) | Builds, GPG-signs and publishes a provider release with GoReleaser; attests build provenance; optionally attaches a CycloneDX SBOM |
| [`versioning.yml`](.github/workflows/versioning.yml) | Runs release-please with a GitHub App token to open the Release PR and cut the tag |
| [`pr-title.yml`](.github/workflows/pr-title.yml) | Requires Conventional Commit PR titles |
| [`security-scan.yml`](.github/workflows/security-scan.yml) | Trivy and govulncheck gates; optionally a summary as a sticky PR comment |

Each provider keeps a short caller workflow with its own triggers and secrets.
Tests stay in each provider; `security-scan.yml` runs as one job of the
provider's CI.

## Pinning

Callers pin a release of this repository by full commit SHA, with the version
in a comment:

```yaml
uses: namecheap/terraform-provider-workflows/.github/workflows/release.yml@<sha> # vX.Y.Z
```

Dependabot (`github-actions` ecosystem) in each provider updates the SHA and
the comment when a new release is published. Never reference `@main` or a
moving tag: the release workflow runs next to the provider's GPG signing key.

## `release.yml`

```yaml
name: Release
on:
  push:
    tags: [v*]
permissions: {}
jobs:
  release:
    name: Release
    permissions:
      contents: write
      id-token: write
      attestations: write
    uses: namecheap/terraform-provider-workflows/.github/workflows/release.yml@<sha> # vX.Y.Z
    secrets:
      gpg-private-key: ${{ secrets.GPG_PRIVATE_KEY }}
      passphrase: ${{ secrets.PASSPHRASE }}
```

| Input | Type | Default | Description |
|---|---|---|---|
| `attest` | boolean | `true` | Build provenance attestation for every file listed in `SHA256SUMS`. Needs `id-token: write` and `attestations: write`. |
| `sbom` | boolean | `false` | Generate a CycloneDX SBOM with Trivy and attach it to the release. |
| `dry-run` | boolean | `false` | Build and sign a snapshot without publishing. |
| `working-directory` | string | `.` | Directory containing `go.mod` and `.goreleaser.yml`. |

| Secret | Required | Description |
|---|---|---|
| `gpg-private-key` | yes | ASCII-armored private key that signs `SHA256SUMS`. |
| `passphrase` | yes | Passphrase for `gpg-private-key`. |

The provider's `.goreleaser.yml` must sign the checksum file with
`--local-user {{ .Env.GPG_FINGERPRINT }}`.

Verify a release built by this workflow:

```bash
gh attestation verify <zip> -R namecheap/<provider> \
  --signer-repo namecheap/terraform-provider-workflows
```

## `versioning.yml`

```yaml
name: Versioning
on:
  workflow_run:
    workflows: ["CI"]
    types: [completed]
    branches: [master]
  workflow_dispatch:
concurrency:
  group: release-please
  cancel-in-progress: false
permissions: {}
jobs:
  versioning:
    name: Versioning
    uses: namecheap/terraform-provider-workflows/.github/workflows/versioning.yml@<sha> # vX.Y.Z
    with:
      app-client-id: ${{ vars.APP_CLIENT_ID }}
    secrets:
      app-private-key: ${{ secrets.APP_PRIVATE_KEY }}
```

| Input / secret | Required | Description |
|---|---|---|
| `app-client-id` (input) | yes | Client ID of the GitHub App that release-please acts as. |
| `app-private-key` (secret) | yes | Private key of that App. |

The App needs `contents: write` and `pull-requests: write`. A GitHub App token
is required because tags and PRs created with `GITHUB_TOKEN` do not trigger
other workflows. The job runs only for a manual dispatch or a successful CI run
from this repository, never from a fork.

## `pr-title.yml`

```yaml
name: PR Title Check
on:
  pull_request:
    types: [opened, edited, synchronize, reopened]
permissions: {}
jobs:
  pr-title:
    name: PR Title
    permissions:
      pull-requests: read
    uses: namecheap/terraform-provider-workflows/.github/workflows/pr-title.yml@<sha> # vX.Y.Z
```

The check reports as `PR Title / Conventional PR Title`. Allowed types:
`feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `ci`, `perf`, `build`,
`revert`.

## `security-scan.yml`

Called as one job of the provider's CI, behind its own change gate:

```yaml
  security:
    name: Security
    needs: changes
    if: needs.changes.outputs.code == 'true'
    permissions:
      contents: read
      pull-requests: write   # only with report: true
      security-events: read  # only with report: true
    uses: namecheap/terraform-provider-workflows/.github/workflows/security-scan.yml@<sha> # vX.Y.Z
    with:
      report: true
```

Checks report as `Security / Security scan (Trivy)`,
`Security / Security scan (govulncheck)` and, with `report`,
`Security / Security scan summary`.

| Input | Type | Default | Description |
|---|---|---|---|
| `trivy` | boolean | `true` | Fail on fixable CRITICAL/HIGH vulnerabilities, misconfigurations, committed secrets and forbidden licenses. |
| `trivy-config` | string | `trivy.yaml` | Trivy config (license denylist), relative to `working-directory`. |
| `govulncheck` | boolean | `true` | Fail on Go vulnerabilities reachable from the module's code, using the Go version in `go.mod`. |
| `report` | boolean | `false` | Non-gating summary: all-severity results, a CycloneDX SBOM artifact and open CodeQL alerts, in the job summary and a sticky comment on same-repository PRs. |
| `working-directory` | string | `.` | Directory containing `go.mod`. |

The Trivy version here and in `release.yml` is the same; CI checks it.

## Contributing

Use Conventional Commits. A change to any reusable workflow, including an
action bump, must be `fix:` or `feat:` so that it is released; `ci:`, `docs:`
and `chore:` do not cut a release.

Every pull request runs the release workflow in dry-run mode against the
fixture in `testdata/provider`, signed with a throwaway key, and checks the
signature.

See [`docs/design`](docs/design) for the design.

## License

[Apache-2.0](LICENSE)
