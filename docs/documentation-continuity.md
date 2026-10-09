# Code and documentation continuity

LoopExec is a general-purpose bounded agentic loop governor. Canonical product
contracts live in this repository. The website is a presentation of those
contracts, not an independently maintained implementation specification.

## Required change gates

Runtime, CLI, dependencies, examples, tools, CI/release workflows and agent-policy
changes require matching canonical prose and CHANGELOG.md in the same change.
`scripts/check-docs-sync.sh` rejects missing co-changes, generated-reference-only
updates and unrelated audit notes. Human review must still verify semantic
accuracy: editing a documentation path alone cannot prove the behavior is correct.

`TestDocumentationContract` compares the complete declared CLI tree, flags,
defaults, help and JSON struct field contract with the checked-in reference.
Default tests fail on drift. After reviewing the prose, regenerate with:

```sh
go test ./cmd/loopexec -run '^TestDocumentationContract$' -args -update-docs
go test ./cmd/loopexec -run '^TestDocumentationContract' -count=1
```

Cobra's implicit help/completion commands are framework behavior, not declared
product commands. The reference includes default help/version flags. Behavioral
tests and manual contract review remain necessary for value and exit semantics.

Add every public guide and new release note to `docs/documentation-files.json`.
The explicit allowlist excludes private handoffs, state, model logs and secrets.
The bundle builder rejects missing required files, duplicates and symlinks.

## Coupled release artifacts

`scripts/build-release.sh` verifies the CLI reference and source version, builds
six platform archives, and includes the full `documentation/` snapshot and
manifest in every archive. A separate documentation archive is checksummed and
published alongside binaries. Its manifest contains the version, a deterministic
source-content digest, a documentation digest and each public file's SHA256.
No timestamp or current git HEAD substitutes for hashing the actual content.

Platform archives also retain the canonical docs/spec/changelog/example paths at
their root so the root README's relative links work after extraction. The nested
documentation snapshot remains the full manifest-bound release evidence.

Prepare the documentation from the exact intended release source, not a newer
main branch. Label unreleased capabilities as unreleased. Before publication,
deploy the matching docs snapshot while keeping the last published installation
as the default until the binary release is public. Deployment and GitHub are
separate services; these gates do not claim a distributed atomic transaction.

## Website build handshake

The docs-site build must consume the canonical bundle, render its CLI, spec,
guides, capabilities and changelog, and publish these files on the same origin:

- `/loopexec-documentation.json`: byte-equivalent manifest fields.
- `/loopexec-documentation/<path>`: exact allowlisted public source files.
- `/`: HTML containing the build marker generated from that manifest.

```sh
go run ./tools/docs-continuity bundle build/documentation
go run ./tools/docs-continuity verify-bundle build/documentation
go run ./tools/docs-continuity marker build/documentation/manifest.json
go run ./tools/docs-continuity verify-site build/documentation/manifest.json
```

Use a new output directory. The printed HTML comment must come from the manifest
used by the site build; never hardcode it separately. Do not keep old manually
authored capability/install pages beside new generated content. Preserve content
through the site template, then verify rendered pages, installation links and
mobile layout. The automated handshake verifies source provenance and deployed
raw content, not whether a template's prose or layout is truthful or usable.

The publication tool has no origin override: it checks `https://loopexec.dev`,
rejects cross-origin/non-HTTPS redirects and fails on missing/stale/unreachable
manifest, homepage marker or document content. Release CI reads back before draft
creation and again immediately before making the release public. A failure leaves
publication stopped; a created draft remains a draft for investigation.

The Release workflow can also be dispatched on main for a non-publishing CI and
deployed-docs rehearsal. Both dispatch and tag runs require source reachable from
main. Only a tag run may enter the publication job, after CI and docs readback pass.

## Enforcement and rollout boundary

Require the CI checks on protected main, restrict release-tag and GitHub release
writes to authorized maintainers/automation, and retain these gates during review.
An administrator or direct manual publication can bypass repository automation;
account rules and human semantic review are still required. Do not claim they are
configured without live verification.

These gates are local unreleased work until committed and adopted by CI. The
separate website currently needs the handshake integration and deployment; the
new release workflow intentionally blocks until it is ready. Do not weaken the
gate because the current website predates it. No release or site deployment is
authorized merely by preparing or testing a bundle.
