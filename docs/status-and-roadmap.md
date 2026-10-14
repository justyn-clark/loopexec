# LoopExec status and roadmap

Historical audit verified October 13, 2026, before v0.4.1 publication. Release and deployment status can change;
[GitHub Releases](https://github.com/justyn-clark/loopexec/releases) remains authoritative.
LoopExec is a general-purpose bounded loop governor. The documentation site
presents its contract; it does not host the runtime.

## Published and deployed

| Surface | Verified status | Evidence |
| --- | --- | --- |
| Source identity | v0.4.1, merged on main | [Source](https://github.com/justyn-clark/loopexec/tree/main), [notes](releases/v0.4.1.md) |
| Public release at audit | v0.4.0, September 26, 2026 | [Release](https://github.com/justyn-clark/loopexec/releases/tag/v0.4.0) |
| Release assets | Six macOS/Linux/Windows amd64/arm64 archives and SHA256 checksums | v0.4.0 release assets |
| Canonical documentation flow | Source-bound bundle imported by the site; official-origin readback required for each release source | [Continuity policy](documentation-continuity.md) |
| Hosted core gates | CI and non-publishing Release rehearsal passed on merged source `6b027d0` | [CI](https://github.com/justyn-clark/loopexec/actions/runs/37960688842), [rehearsal](https://github.com/justyn-clark/loopexec/actions/runs/37961116072) |
| Hosted site gate | Canonical Docs passed on docs source `595e4b0` | [CI](https://github.com/justyn-clark/loopexec.dev/actions/runs/38295817979) |
| Account enforcement | Required Linux/macOS CI checks enabled on core main, including administrators; force pushes and deletion disabled | GitHub branch-protection API, October 13, 2026 |

The live documentation manifest already describes source v0.4.1. This is a
documentation source identity, not evidence of a public binary release.
Every subsequent documentation change needs a new bundle and deployment
readback. Linux/macOS CI covers runtime behavior; Windows is cross-built
and native Windows runtime remains unverified.

## Available capabilities

v0.3.0 introduced bounded unattended workflow hooks, exact known/unknown
accounting, numeric progress/patience, candidate snapshots/restoration, and
offline creative and SMALL compatibility proofs. v0.4.0 adds shared model-call
reservations across run IDs. Existing deterministic loops, integrity guards,
replay/attestation, halt explanations and subprocess deadlines remain available.
See [SPEC section 11](../SPEC.md#11-capability-status-normative-source-for-the-docs-matrix)
for the capability-level contract.

The v0.4.1 source improves reports, enforces code/docs continuity gates,
packages source-bound docs and adds general-purpose [acceptance guidance](acceptance-policy.md).
Those source changes do not alter existing v0.4.0 packages or consumer installations. The source and site integration are merged; v0.4.1 has not been published at this audit.

An optional Jev semantic judge exists in a separate local `codex/jev-judge`
worktree based on the older v0.3.0 development lineage. It is uncommitted and
unreleased; it is absent from published main/v0.4.0. Previous offline validation
does not establish current integration, live-provider readiness or calibrated
quality thresholds. Reconcile it with current main and evaluate labeled fixtures
before considering it for a release.

## Prioritized remaining work

| Priority | Work | Acceptance evidence |
| --- | --- | --- |
| P0 | Keep each new canonical bundle and rendered site synchronized | Source-bound manifest/content readback, rendered-page checks and correct published/unreleased boundaries; publication remains blocked until matching |
| P0 | Require committed CI/account release protections | Exact-commit CI plus live branch/tag/release permission verification; local tests do not establish hosted enforcement |
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
