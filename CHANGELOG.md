# Changelog

## Unreleased

- Added canonical audit privacy policy and a publication gate rejecting new
  absolute host paths in SMALL metadata and command proofs. Historical receipts
  remain unchanged; unpublished private captures are excluded from delivery.
  Named-home references are rejected along with absolute paths.

- Corrected architecture to describe the shipped external-check runtime; marked
  the historical SMALL task-list design as planned.
- Reconciled source/publication status, canonical Getting Started and conceptual
  homepage prose; removed obsolete rollout claims and corrected historical releases.

Future recipes and workflow adapters will remain configurable, opt-in integrations.

## v0.4.1 — current source

See [release notes](docs/releases/v0.4.1.md). Publication status is authoritative
on [GitHub Releases](https://github.com/justyn-clark/loopexec/releases).

- Adopted stable SMALL v1.1.2 in checksum-verified CI. Its narrowly scoped
  captured-source validation resolves the retained local QA template capture.
  Preserved historical
  command receipts with individually reviewed portable proof; missing or tampered
  proof fails closed. Local validation is not proof of hosted release readiness.
- Hardened code/docs continuity: canonical prose and changelog co-change CI gate,
  generated CLI/JSON contract tests, source-bound allowlisted documentation in
  every platform archive, and fail-closed deployed-docs checks before publication.
  The docs site consumes the allowlisted bundle; release publication requires exact official-origin readback.
- Added a non-publishing Release workflow rehearsal and a main-line ancestry gate
  before deployed-docs verification or tag publication.
- Clarified general-purpose acceptance policy: original run contracts, candidate
  retention, explicit owner adjustments and desired objectives remain separate.
  No domain-specific score, automatic budget reset or model rerun was introduced.

- Read-only reports expose the parsed event timeline, command phase/duration/cause,
  failure cause, and missing or damaged receipt warnings. Existing report exit
  behavior is unchanged; a readable receipt is not an integrity verification.
- Reports distinguish retained numeric candidates from convergence and signature
  presence from verified attestation. The legacy JSON `attested` field still
  describes file presence; use `attestation_status` and explicit verification.
- Added an evidence-capture guide for creative verification and a release/status
  roadmap. These changes are merged in source. The Releases page establishes whether
matching binary packages have been published.

## v0.4.0

See [release notes](docs/releases/v0.4.0.md). Published September 26, 2026.

Adds shared model-call budgets across run IDs, pinned total/per-phase allowances,
durable pre-call reservations, and an offline adapter/proof. A failed child still
consumes its slot. This counts calls; it does not replace token/dollar metering.

## v0.3.0

See [release notes](docs/releases/v0.3.0.md) for capabilities, compatibility
changes, installation, validation, and platform limits.

Adds bounded unattended workflows, exact metering, numeric progress, candidate
restoration, an offline creative adapter, SMALL v1.1.0/v2-session compatibility
proofs, and binary release packages. Existing convergence exit code 10 is unchanged.

## v0.2.0

Published June 27, 2026 (UTC). Introduced the real bounded loop, computed halts,
typed receipts, determinism probing, regression/integrity guards, replay,
attestation, context slicing and isolation orchestration. The deterministic
demo and unattended workflow extensions arrived in v0.3.0. See
[the historical release](https://github.com/justyn-clark/loopexec/releases/tag/v0.2.0).

## v0.1.0

Published February 22, 2026. Established the standalone execution harness,
single-object JSON contract, stable exit classes, contract tests and
format/vet/test CI. See
[the historical release](https://github.com/justyn-clark/loopexec/releases/tag/v0.1.0).
