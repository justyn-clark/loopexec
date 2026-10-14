# Getting Started

## Install and prove the runtime

With Go 1.26 or newer, install the latest published stable release. Ensure
Go's binary directory (`go env GOBIN`, or `$(go env GOPATH)/bin` when GOBIN
is empty) is on PATH.

```sh
go install github.com/justyn-clark/loopexec/cmd/loopexec@latest
loopexec --version
loopexec demo
```

The command resolves the latest public stable release. [GitHub Releases](https://github.com/justyn-clark/loopexec/releases)
is authoritative; documentation source identity does not establish publication.
For a pinned install, substitute a tag listed there as public for `latest`. Prebuilt archives and
checksums are available there if Go is unavailable.

After installation, `loopexec demo` needs no model, API key, Python runner,
or network access. It creates a red local fixture, executes the real bounded
loop, repairs the fixture, observes an external green check, writes a receipt,
and replays the check fingerprint. This proves the execution/check/receipt/replay
contract, not agent intelligence. Its successful process exit is 0.

No clone or hidden fixture is needed. The output gives the retained temporary
workdir and receipt path. For a known location, use `loopexec demo --workdir <fresh-dir>`;
it refuses to overwrite existing `.loopexec` or `status.txt` fixtures.

## Run your own bounded loop

In your own project, supply a real work command and a real check:

```sh
loopexec run \
  --check "go test ./..." \
  --exec "make fix" \
  --max-iterations 10
```

This example assumes that your project has Go tests and a `make fix` target
that performs one repair attempt. Replace them with your project's commands.
LoopExec runs the work command, then the check, until the check passes, a bound
trips, or execution fails. Halt reasons are computed from observed state.

An external check exits 0 when green. A converged `loopexec run` exits **10**,
so shell/CI wrappers must handle that successful convergence class explicitly.
Use the reported receipt and run ID with `loopexec replay` and `loopexec report`;
see the [CLI guide](cli.md) for exact flags and halt classes.

## Go deeper

- [Capabilities](../SPEC.md#11-capability-status-normative-source-for-the-docs-matrix): shipped cores and planned sub-parts.
- [Workflows](workflows.md): deadlines, usage accounting, numeric progress and candidate restoration.
- [Integrations](integrations.md): adapter and operator responsibilities, including isolation hooks.
- [Shared model-call budgets](../examples/scoped-budget/README.md): call limits across run IDs.
- [SMALL](small-integration.md): optional state/audit integration.
- [Acceptance and evidence](acceptance-policy.md): retention, convergence and owner decisions.
