# Changelog

## Unreleased

Future recipes and workflow adapters will remain configurable, opt-in integrations.

## v0.4.1

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
  The separate docs site must adopt the build handshake before the next release.
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
  roadmap. These changes are local until a subsequent release is published.

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

Previous published release. Later core command and deterministic demo work is
included in v0.3.0; this changelog does not retroactively attribute it to v0.2.0.
