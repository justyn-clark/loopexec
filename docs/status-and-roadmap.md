# LoopExec status and roadmap

Snapshot verified October 7, 2026. This is a dated audit, not a live release feed.
This audit covers the general-purpose LoopExec governor and published `loopexec`
binary. The home project's quality verifier is one consumer, not its product scope.

## Published and deployed

| Surface | Verified status | Evidence |
| --- | --- | --- |
| Latest published release | v0.4.0, September 26, 2026; public, not a draft or prerelease | [Release](https://github.com/justyn-clark/loopexec/releases/tag/v0.4.0) |
| Release artifacts | Six macOS/Linux/Windows amd64/arm64 archives plus SHA256 checksums | Release assets and [successful release workflow](https://github.com/justyn-clark/loopexec/actions/runs/36276149282) |
| Release commit | `c0e4cc2653b428faa2fceb6485f94a98aa3167d5` | Annotated v0.4.0 tag resolves to this commit |
| Published main | `4c96ad49d9d57199824fc44c7eeee8108d292dbd`; local main matched at audit start | [Main](https://github.com/justyn-clark/loopexec/tree/main) |
| Main CI | Passed on the exact published main commit | [CI run](https://github.com/justyn-clark/loopexec/actions/runs/36288545259) |
| Open GitHub backlog | No open issues or PRs at audit time; not evidence that the product has no remaining work | Repository issues/PR API reads |
| Public documentation | https://loopexec.dev is accessible; its homepage/changelog describe v0.2.0 | Public page read and matching source in the separate `loopexec.dev` repository |

LoopExec ships as a locally installed CLI; the deployed website is documentation,
not a hosted LoopExec runtime. The docs repository is configured for Vercel, but
this audit did not verify the provider deployment ID or control-plane settings.
Linux/macOS CI exercises runtime behavior. Windows is cross-built; native
Windows runtime remains unverified. Asset metadata was checked live; the binary
archives were not downloaded again during this audit.

## Available capabilities

v0.3.0 introduced bounded unattended workflow hooks, exact known/unknown
accounting, numeric progress/patience, candidate snapshots/restoration, and
offline creative and SMALL compatibility proofs. v0.4.0 adds shared model-call
reservations across run IDs. Existing deterministic loops, integrity guards,
replay/attestation, halt explanations and subprocess deadlines remain available.
See [SPEC section 11](../SPEC.md#11-capability-status-normative-source-for-the-docs-matrix)
for the capability-level contract.

The current local patch improves reports, enforces code/docs continuity gates,
packages source-bound docs and adds general-purpose [acceptance guidance](acceptance-policy.md).
Those changes are unreleased and do not alter v0.4.0 packages or existing consumer
installations. No new version, push, tag, deployment or publication was performed.

An optional Jev semantic judge exists in a separate local `codex/jev-judge`
worktree based on the older v0.3.0 development lineage. It is uncommitted and
unreleased; it is absent from published main/v0.4.0. Previous offline validation
does not establish current integration, live-provider readiness or calibrated
quality thresholds. Reconcile it with current main and evaluate labeled fixtures
before considering it for a release.

## Prioritized remaining work

| Priority | Work | Acceptance evidence |
| --- | --- | --- |
| P0 | Adopt canonical docs/build handshake in the site, then sync home/install/CLI/capability/changelog | Source-bound manifest/content readback, rendered-page checks and correct published/unreleased boundaries; publication remains blocked until matching |
| P0 | Commit continuity gates and require CI/account release protections | Exact-commit CI plus live branch/tag/release permission verification; local tests do not establish hosted enforcement |
| P0 | Release these report fixes after review and CI | Exact-commit CI, offline proof receipts, packages/checksums and fresh install verification |
| P1 | Migrate existing adapters from v0.3.0/custom budget guards deliberately | Historic and failed reservations preserved; no restored allowance; parity and concurrency checks before model launch |
| P1 | Add general-purpose read-only objective history across run IDs | Objective/candidate/policy/evidence/decision/budget/promotion history; explicit owner revisions and missing-evidence warnings; no work or model calls |
| P1 | Strengthen oracle feedback | Calibrated references, controlled measurements, criterion-level interventions, resolved contradictions and evidence freshness |
| P1 | Reconcile and calibrate the optional Jev judge | Current-main integration tests, labeled accept/reject/escalate fixtures; explicit provider/usage proof before live claims |
| P2 | Broader runtime and safety coverage | Native Windows checks; documented OS limits; operator-provided containment/egress/provider hooks proven in deployment |
| P2 | Remaining specification sub-parts | Assertion-count/coverage-floor integrity, adversarial determinism/sequential monitor, context dependency tiers, escalation integrations/watchdog and full SMALL task-list topology |

Acceptance criteria are integration contracts, not a universal quality certificate.
Retention, convergence, human approval and promotion must remain distinguishable.
An exhausted review budget or unresolved visual readiness is a stop condition,
not a reason to launch another critic under a new run ID.
