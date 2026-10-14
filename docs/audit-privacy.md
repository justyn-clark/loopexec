# Audit Privacy

New canonical tracking must not disclose machine-specific filesystem paths.
This applies to every `.small` YAML file, including progress, handoff, plan,
intent, constraints and workspace metadata, and to `.small-command-proofs`.
It covers commands, summaries, evidence, notes and embedded scripts.

Use repository-relative paths such as `build/proofs/run.json`, or logical
aliases such as `[workspace]` and `[local-cache]`. Keep alias-to-machine
mappings in untracked private configuration. Prefer helper scripts invoked
by relative path rather than capturing large inline scripts. Do not expose
usernames, home directories, temporary host directories, absolute tool
locations, Windows drives, network shares or local file URIs.
Named-home shell references are machine-specific too; use a logical alias instead.

## Enforcement

Run `go run ./tools/audit-privacy` before publication. CI and the release
workflow run the same check. It inspects tracked and untracked canonical files,
parses YAML values (including escaped values), and rejects new absolute paths.
Failures identify only the relative record location, not the offending value.
This privacy lint complements secret scanning and human semantic review.

The historical baseline in `scripts/audit-privacy-policy.json` pins existing
published history. Progress entries must remain unchanged; historical proof
bytes must remain identical. Baseline changes require an explicit owner policy
decision, never an agent exception to clear a failing check.

## Historical Records

Never obfuscate an original receipt or hash-bound proof in place. Changing
command bytes invalidates its digest and provenance. Unpublished path-bearing
captures stay private and are not imported into public delivery branches.
Preserve them locally; record fresh validation with portable commands through
SMALL on a clean delivery branch retaining the published history.

Already published historical records need a separate owner-approved cleanup
or protocol migration. This gate does not remove old public Git history.

## SMALL Boundary

SMALL v1.1.2 does not enforce this privacy property by itself. A fixture with a
synthetic absolute path in progress evidence passed its strict check. A written
constraint must not be presented as executable enforcement. LoopExec adds a
publication gate; upstream SMALL should reject unsafe new authored fields
before recording them or executing a command. A future redacted export must
preserve private original bytes and hashes with a separate public provenance
contract. Never silently sanitize receipts, weaken exact-byte proof, or publish
the private command cache.
