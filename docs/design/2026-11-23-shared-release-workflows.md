# Shared release workflows for the Terraform providers — design

Status: approved · 2026-11-23

## Intent

`terraform-provider-spaceship`, `terraform-provider-namecheap` and
`terraform-provider-jenkins` each keep their own copy of three workflows:
`release.yml` (GoReleaser + GPG signing), `versioning.yml` (release-please) and
`pr-title.yml` (Conventional Commit title check). The copies drift: the
release-job hardening that landed in spaceship (persist-credentials off, no Go
cache in the signing build, pinned GoReleaser binary, `go mod tidy -diff` /
`go mod verify`) is missing from jenkins and only partly present in namecheap.
Every future change has to be made three times and is easy to miss in one.

This repo, `namecheap/terraform-provider-workflows`, holds those three
workflows once, as reusable workflows. Each provider keeps a thin caller.

## Goals

- One definition of the release, versioning and PR-title workflows.
- Every provider gets the full release-job hardening.
- A change to the signing job never reaches a provider without review in the
  shared repo and a reviewed bump in the provider.
- Changes to the shared release workflow are exercised in CI before they are
  tagged.

## Non-goals

- CI and test workflows (`ci.yml`, `test.yml`, stale, EC2 runner
  cleanup) stay in each provider. Their testing strategies differ on purpose.
  Security scanning is the exception (phase 2 below): it is the same job
  everywhere, so it moves here and each `ci.yml` calls it as one job.
- `.goreleaser.yml` stays per provider (build matrix, `draft: true` in
  namecheap, registry manifest handling are provider decisions).
- No SBOM for spaceship and jenkins (the code lives here; only namecheap
  turns it on).

## Anti-goals

- A mutable ref (`@main`, `@v1`) in any caller. That would let a push to this
  repo change what runs next to three GPG signing keys without review.
- Shared behaviour that silently differs per repo beyond the documented
  inputs.
- Forcing an empty release just to test the migration.

## Constraints

- This repo must be **public**: public repos can only call reusable workflows
  from public repos.
- Triggers (`on:`) cannot be shared; each provider keeps them in its caller.
- `GITHUB_TOKEN` permissions in a called workflow are capped by what the
  caller grants. Callers must grant `contents: write` for release, plus
  `id-token: write` and `attestations: write` when attestation is on.
- Jobs moved into a reusable workflow report as `<caller job name> / <callee
  job name>`. Required status checks that point at the old names must be
  renamed at cutover.
- App credentials for release-please have different names per repo
  (`SPS_RELEASE_CLIENT_ID`/`SPS_RELEASE_PRIVATE_KEY` in spaceship,
  `APP_CLIENT_ID`/`APP_PRIVATE_KEY` in namecheap and jenkins). Default
  branches differ (`master`, `master`, `main`).

## Approaches considered

**Sharing mechanism**

| Option | Trade-off |
|---|---|
| **Reusable workflows (`on: workflow_call`)** — chosen | Owns the whole job: runner, permissions, steps, toggles. Caller is ~15 lines. Check names change. |
| Composite actions | Share steps only; every repo still defines the job, permissions and secret wiring — the parts that drifted. |
| Keep copies, sync by script | No new repo, but drift is the problem being solved. |

**How callers reference this repo**

| Option | Trade-off |
|---|---|
| **Commit SHA pin + own tagged releases** — chosen | Unreviewed code never reaches the signing job. One shared change = one PR here + one Dependabot bump PR per provider. |
| Moving major tag `@v1` | Instant propagation; mutable ref in the signing job. |
| `@main` | Simplest; same risk, plus untested changes hit the next release of all three. |

Chosen over `@main` because these are open-source repos with a GPG signing
key in the release job.

**Testing the release workflow**

| Option | Trade-off |
|---|---|
| **Dry-run in this repo's CI against a fixture provider** — chosen | Catches breakage before tagging; needs a fixture and a `dry-run` input. |
| First real release is the test | Nothing to build; a failure lands on a public release. |

Provider migrations are not gated on a real release: no releases are planned,
and the first real release of each provider is watched and fixed iteratively.

**Name**: `terraform-provider-workflows` over a generic `github-workflows`
(unrelated workflows would ride the same release stream and open irrelevant
bump PRs in the providers) and `namecheap/.github` (reserved for org-wide
community defaults).

## Design

### Repository

- `namecheap/terraform-provider-workflows`, public, default branch `main`.
- Ruleset on `main`: pull request required, this repo's CI required, no force
  push, no deletion. DCO, as in the providers.
- Conventional Commits, release-please, tags `vX.Y.Z` with GitHub Releases.
- Release-please here authenticates with a **dedicated GitHub App**, owned by
  the org and installed only on this repo (`contents: write`,
  `pull-requests: write`). It is not one of the providers' Apps, so a key
  leaked from a provider's CI cannot write to the repo behind every signing
  job. Accepted cost: one more App and private key to manage.
- The App's client ID (variable) and private key (secret) live in an
  environment `release` restricted to `main`, so a workflow on another branch
  cannot read the key. This is the only environment secret in the setup;
  everything else stays a plain repo or org secret.
- Key rotation: create a new key in the App settings → check its fingerprint
  (`openssl rsa -in key.pem -pubout -outform DER | openssl sha256 -binary |
  openssl base64`) → `gh secret set --env release` → dispatch
  `self-versioning.yml` and confirm green → delete the old key and the local
  PEM. On suspected leak, delete the old key first.
- Current App and key: `tf-provider-workflows-release`, key in
  `RELEASE_APP_PRIVATE_KEY` since 2026-11-25, fingerprint
  `SHA256:h4R702cT3y0QTHww3tn//QfCC7dOj+MTUY5A51XC06A=`. Update this line on
  every rotation.

Layout:

```
.github/workflows/
  release.yml          # workflow_call — GoReleaser + GPG (+ attest, + SBOM)
  versioning.yml       # workflow_call — release-please
  pr-title.yml         # workflow_call — Conventional PR title
  ci.yml               # this repo's own CI (lint + dry-run release)
  self-versioning.yml  # standalone release-please for this repo (needs
                       # `environment: release`, which a job calling a
                       # reusable workflow cannot set)
  self-pr-title.yml    # calls ./pr-title.yml for this repo
testdata/provider/     # minimal Go "provider" + .goreleaser.yml for the dry-run
.release-please-config.json
.release-please-manifest.json
.github/dependabot.yml
```

### `release.yml` (reusable)

Inputs:

| Input | Type | Default | Purpose |
|---|---|---|---|
| `attest` | bool | `true` | Build provenance attestation for every file listed in `SHA256SUMS`. On for all three providers. |
| `sbom` | bool | `false` | CycloneDX SBOM via Trivy, uploaded to the release. Namecheap only. |
| `dry-run` | bool | `false` | Build and sign without publishing. Used by this repo's CI only. |
| `working-directory` | string | `.` | Where `go.mod` and `.goreleaser.yml` live. Used by the dry-run fixture. |

Secrets (explicit, never `secrets: inherit`): `gpg-private-key`,
`passphrase`. `GITHUB_TOKEN` comes from the caller.

Steps, one job:

1. Checkout — `fetch-depth: 0`, `persist-credentials: false`.
2. Setup Go — `go-version-file`, `cache: false`.
3. `go mod tidy -diff` and `go mod verify` as workflow steps (so jenkins and
   namecheap get them without touching their `.goreleaser.yml`).
4. Import GPG key.
5. GoReleaser — binary pinned (`v2.19.0`, owned here, bumped by hand).
   `release --clean`; in dry-run `release --clean --snapshot --skip=publish`.
6. If `attest` and not dry-run: locate `dist/*_SHA256SUMS`, run
   `actions/attest-build-provenance`.
7. If `sbom`: generate CycloneDX with Trivy (binary version pinned here); if
   not dry-run, `gh release upload --clobber`.

All third-party actions SHA-pinned with a version comment.

### `versioning.yml` (reusable)

Inputs: `app-client-id` (string, required). Secrets: `app-private-key`.
Callee job permissions `{}`; all writes go through the App token.

The guard lives in the callee, so jenkins gains the fork guard it lacks today:

```yaml
if: >-
  github.event_name == 'workflow_dispatch' ||
  (github.event.workflow_run.conclusion == 'success' &&
   github.event.workflow_run.head_repository.full_name == github.repository)
```

Steps: create App token → release-please with `.release-please-config.json`
and `.release-please-manifest.json`.

### `pr-title.yml` (reusable)

No inputs. `amannn/action-semantic-pull-request` SHA-pinned, same type list as
today (`feat fix docs chore refactor test ci perf build revert`). Permission
`pull-requests: read`.

### Caller stubs (shape)

```yaml
# provider/.github/workflows/release.yml
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
      id-token: write       # attestation (Sigstore certificate)
      attestations: write   # attestation (store)
    uses: namecheap/terraform-provider-workflows/.github/workflows/release.yml@<sha> # vX.Y.Z
    # namecheap only:
    # with: { sbom: true }
    secrets:
      gpg-private-key: ${{ secrets.GPG_PRIVATE_KEY }}
      passphrase: ${{ secrets.PASSPHRASE }}
```

```yaml
# provider/.github/workflows/versioning.yml
name: Versioning
on:
  workflow_run:
    workflows: ["CI"]
    types: [completed]
    branches: [master]        # main in jenkins
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
      app-client-id: ${{ vars.SPS_RELEASE_CLIENT_ID }}   # APP_CLIENT_ID elsewhere
    secrets:
      app-private-key: ${{ secrets.SPS_RELEASE_PRIVATE_KEY }}
```

`pr-title.yml` follows the same shape with `pull-requests: read`.

### This repo's CI

On every PR and push to `main`:

- `actionlint` and `zizmor` over `.github/workflows/`.
- Dry-run: call `./.github/workflows/release.yml` with `dry-run: true`,
  `working-directory: testdata/provider`, `sbom: true`, signing with a
  throwaway GPG key stored as this repo's secret (no trust value; never a
  provider key). The key is set in both the Actions and the Dependabot
  secret stores: runs Dependabot triggers read only the Dependabot store, and
  action bumps are the PRs this dry-run matters for most.

### Versioning of this repo

- Conventional Commits; release-please cuts `vX.Y.Z`.
- Anything callers run is a published artifact: a change to a reusable
  workflow, including a Dependabot bump of an action **inside** it, must be
  `fix:` (or `feat:`) so it reaches a release. Dependabot here uses prefix
  `fix` for `github-actions`.
- Providers already run Dependabot for `github-actions` with prefix `ci`; it
  bumps the SHA + version comment on the caller line. Those bumps don't
  release the provider, which is correct: workflows don't ship in the binary.

### Step 0: code scanning baseline (before phase 1)

Independent of this repo; done first so CodeQL's workflow-file scan reviews
the phase 1 migration PRs.

- One org-level security configuration, applied to the three providers only:
  CodeQL **default setup** (Go + GitHub Actions, `default` suite), secret
  scanning, push protection, Dependabot alerts and security updates.
- Spaceship and jenkins: delete `codeql.yml` (advanced setup) in the same
  change window as enabling default setup, so the two don't conflict, and
  delete the orphaned advanced configurations in code scanning.
- Jenkins: replace the required check `CodeQL (go)` with the default-setup
  check name before merging its `codeql.yml` removal.
- Set up by hand in org settings for now. As of v6.13.0, the
  `integrations/github` Terraform provider has no resource for org security
  configurations or CodeQL default setup; open upstream PRs #3284/#3542
  (configurations) and #3315 (default setup) would add them. When the org moves
  to Terraform, the configuration is imported, and Terraform should not also
  manage the same settings per repo through `github_repository.security_and_analysis`.
- Accepted: spaceship and jenkins drop from `security-and-quality` to the
  `default` suite. Over the repos' whole history the Go scan found nothing;
  the Actions scan found the only real alerts (namecheap, 5×
  `missing-workflow-permissions`, fixed 2026-03-29).

### Migration

Order: **spaceship → jenkins → namecheap**, one PR per provider, proceeding
to the next without waiting for a release.

Per provider:

1. Replace `release.yml`, `versioning.yml`, `pr-title.yml` with caller stubs
   pinned to the current release of this repo.
2. Update required status checks to the new `<caller> / <callee>` names in the
   same change window (jenkins branch protection requires
   `Conventional PR Title` today).
3. Update `RELEASE.md` / `CI.md` references to the moved logic.

Provider-specific:

- **spaceship**: same release job as today, plus attestation.
- **jenkins**: gains the hardening, attestation, the fork guard, and
  `permissions: {}` in versioning (today it grants
  `contents`/`pull-requests: write` it doesn't need).
- **namecheap**: `sbom: true`; GoReleaser jumps v2.15.4 →
  v2.19.0 (run `goreleaser check` on its `.goreleaser.yml` first);
  floating-tag actions become SHA-pinned via the shared repo.

Attestation signer identity: after the move, an attestation's
`buildSignerURI` names this repo's `release.yml`; `sourceRepositoryURI` stays
the provider. Nobody verifies namecheap's attestations today, so no
communication step is needed. Each provider's `RELEASE.md` (and namecheap's
`SECURITY_COMPLIANCE.md`) documents the verify command:

```bash
gh attestation verify <zip> -R namecheap/<provider> \
  --signer-repo namecheap/terraform-provider-workflows
```

Namecheap releases up to the cutover verify without `--signer-repo`.

Validation per provider: PR-title check runs on the migration PR; versioning
runs on the next successful master/main CI; release is validated by this
repo's dry-run and confirmed on the provider's next real release, fixing
forward if needed.

Rollback: revert the provider's migration PR.

### Phase 2: `security-scan.yml` (reusable)

Starts after all three providers run the shared release, versioning and
PR-title workflows. Each provider's `ci.yml` calls it as one job, so the
provider keeps its own change-detection and `CI OK` aggregation.

| Input | Type | Default | Purpose |
|---|---|---|---|
| `trivy` | bool | `true` | Trivy filesystem scan: vulnerabilities + license denylist. |
| `trivy-config` | string | `trivy.yaml` | Per-repo config (license denylist stays in the provider). |
| `govulncheck` | bool | `true` | Go vulnerability check. |

- Trivy's action and binary versions are pinned here once and shared with the
  release SBOM step, so the CI scan and the SBOM can't drift.
- On by default everywhere: jenkins gains Trivy and needs its own
  `trivy.yaml`. All three already run govulncheck (spaceship and namecheap
  as `Security scan (govulncheck)` in `ci.yml`, jenkins inside `test.yml`).
- Namecheap's required checks `Security scan (Trivy)` and
  `Security scan (govulncheck)` are renamed at its cutover.
- Same migration order: spaceship → jenkins → namecheap.
- CodeQL is not part of this workflow; it runs as default setup (step 0).
- Optionally upload Trivy results as SARIF so they appear in the Security
  tab next to CodeQL.

## Left to the builder

- Fixture shape under `testdata/provider` (smallest Go module +
  `.goreleaser.yml` mirroring the providers' signing config).
- Whether GoReleaser signs under `--snapshot`; if not, adjust `--skip` so the
  dry-run still exercises GPG signing.
- Whether `concurrency` must stay in the caller or can move into the callee.
- Exact caller job names and the resulting check names.

## Verified facts

- Org Actions policy: all three providers report `allowed_actions: all`.
- Required checks: only jenkins requires a moved check (`Conventional PR
  Title`, classic branch protection). Spaceship's ruleset has no required
  status checks; namecheap's (`Check`, the two security scans, snyk) are not
  in the moved workflows.

## Calls made during write-up (check these)

- **Trivy version is pinned here**, not passed as an input. Between phase 1
  and phase 2, namecheap's `ci.yml` Trivy pin is matched by hand; phase 2
  removes that. Namecheap's `SECURITY_COMPLIANCE.md` is updated to say the
  scanner version lives in this repo.
- **Phase 2 also shares govulncheck**, on by default, alongside Trivy; all
  three providers already run it in some form.
- **Security scanning is phase 2**, not part of the first migration, to keep
  the first cutover small.
