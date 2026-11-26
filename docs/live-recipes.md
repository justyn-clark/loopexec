# Live JC recipes

All 33 catalog recipes have configurable execution adapters and real domain
fixtures. This is development source, with local tool verification. It is not a
published release or evidence that an arbitrary project meets its acceptance
contract.

## Run and verify

Build the matching executable, then select a recipe:

```sh
go build -o build/loopexec ./cmd/loopexec
build/loopexec recipes suggest "repair mobile overflow"
build/loopexec recipes show responsive-ui
build/loopexec recipes demo responsive-ui --workdir build/responsive-demo --tools tools.json
build/loopexec recipes verify --workdir build/responsive-demo --run-id jc-responsive-ui --json
```

The demo directory must be new. A demo saves a complete recipe.json, original
references, a deliberately failing baseline, candidate files, actual tool output,
per-attempt measurements, a typed receipt, and a private integrity seal. Edit a
copy of its config to bind your own inputs and builder. Replace fixture-only
\_demo-work commands with your real one-attempt command. Do not reuse demo
acceptance as approval for another project.

```sh
build/loopexec recipes plan code-repair --config recipe.json
build/loopexec recipes run code-repair --config recipe.json --workdir work --run-id repair-1 --json
build/loopexec recipes verify --workdir work --run-id repair-1 --json
```

Run returns the governor's exit: 10 for convergence, 17 for integrity/progress
guards, 18 for exhausted budgets, and other documented stop codes. Demo returns
0 only when its expected computed outcome and receipt verify. Verify returns 0
for an intact, independently reproducible receipt, including expected guard
stops. A verified guard receipt does not mean the task was accepted.

Each result separates receipt_verified, oracle_passed, acceptance_met,
baseline_passed and halt_reason. For metric gaming, the narrowed suite can be
green while acceptance remains false because integrity failed.

## Configuration

A config has schema_version:1, recipe, inputs, a complete limits object and
adapter. Typed inputs and thresholds come from recipes show; defaults are only
the ones declared in the catalog. Inputs are data, never shell interpolation.

```json
{
  "schema_version": 1,
  "recipe": "code-repair",
  "inputs": {
    "project": "candidate",
    "test_command": "go test ./...",
    "allowed_paths": ["candidate/sum.go"]
  },
  "limits": {
    "max_iterations": 5,
    "timeout_seconds": 600,
    "command_timeout_seconds": 120
  },
  "adapter": {
    "schema_version": 1,
    "work": ["my-builder", "--context", "{context}"],
    "artifacts": ["candidate"],
    "protected": ["candidate/sum_test.go", "candidate/go.mod"]
  }
}
```

work is one argv invocation. Whole arguments may be {context}, {root},
{iteration}, or {input.name}. Context contains the resolved inputs, attempt
number, evidence directory and tool mappings. The governor owns retries.
An explicitly selected shell can be bound with ["sh","-c",...]; ordinary
command strings use literal quote parsing and reject shell operators.

artifacts declares candidate ownership and evidence identity. protected freezes
reference/checker bytes; inventory freezes measured file identities while
allowing legitimate source edits. allowed_paths is relative to the workspace;
other files within artifacts are frozen, including newly added files.
Supply all reference, verifier, source-fixture and policy paths in protected.
Source, reference, truth, OpenAPI and evaluation-set input files are also
automatically protected. extra checks are adapter.checks entries with id and
argv. Additional natural-language constraints need corresponding explicit
probes; their wording is never silently interpreted as an executable test.

tools maps logical names to private installed executables or modules. Browser
checks need Node, Playwright/Chromium; accessibility also needs tools.axe pointing
to axe.min.js. Audio/media need ffmpeg and ffprobe; OCR needs tesseract; databases
need sqlite3. Native engine fixtures use tools.blender and tools.godot. Tools are
not downloaded implicitly except the explicit dependency demo, which installs
two pinned npm packages with lifecycle scripts disabled.

These commands execute with the caller's permissions. Workspace path validation,
manifests and integrity guards are not an OS sandbox. Review executable adapters
and keep production targets separate.

## Domain bindings

| Recipe                 | Real verifier and required binding                                                                                                    |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| code-repair            | test_command, protected tests and allowed source paths                                                                                |
| frontend-visual        | Chromium PNG comparison against reference; options.assertions for DOM rules, optional viewport                                        |
| accessibility          | axe rules over routes; critical counts must meet max_critical                                                                         |
| performance            | options.benchmark argv, warmup then median samples; process milliseconds or numeric options.metric_path; separate correctness_command |
| media-optimization     | FFmpeg SSIM/VMAF, ffprobe dimensions/duration/streams, actual bytes; one compression_steps candidate per attempt                      |
| audio-mastering        | FFmpeg measured integrated LUFS, true peak, EBU loudness range; two-pass normalization                                                |
| music-mix              | PCM silence, phase correlation, RMS balance, loudness/peak; options.expected_stems inventory with matching duration/sample rate       |
| asset-optimization     | Protected Blender/engine oracle; triangle/texture counts, per-view similarity, UV/material/normal/scale/import checks                 |
| godot-performance      | Protected engine oracle; active renderer, pinned workload, p95 frame time, draw calls, memory and every gameplay assertion            |
| responsive-ui          | Real Chromium viewports, overflow limits and configured DOM assertions                                                                |
| api-contract           | Actual HTTP responses against a frozen OpenAPI subset; options.contract_command argv for a full external contract verifier            |
| migration-safety       | Fresh SQLite per attempt, disposable:true, independent invariant queries and optional rollback script                                 |
| sql-optimization       | Read-only SQLite result equivalence, warmup/median latency, EXPLAIN QUERY PLAN; options.database/baseline_query                       |
| docker-reduction       | Actual Docker build/inspect, platform-specific uncompressed bytes, network-disabled read-only containers for checks; disposable:true  |
| dependency-remediation | Protected scanner oracle, exact selected-severity counts, manifest/lock consistency and actual regression tests                       |
| lint-types             | Actual check_command exit against allowed paths                                                                                       |
| documentation-drift    | version_command, version-bearing guides/links, contract_command and explicit remote readback                                          |
| release-readiness      | version_command, checksums, changelog, every bound release gate and explicit docs readback                                            |
| configuration-repair   | Syntax validator plus an independent integration command in the declared sandbox                                                      |
| data-cleaning          | CSV/JSON IDs, required fields, Go-layout dates, numeric ranges and permitted row loss                                                 |
| etl-reconciliation     | Exact canonical rows/duplicate multiplicity, key cardinalities and rational decimal totals                                            |
| ocr-extraction         | Actual Tesseract text, regex field extraction and frozen ground truth; confidence alone cannot pass                                   |
| prompt-optimization    | Protected evaluator over frozen/held-out cases; score, latency, known cost and hard correctness                                       |
| cost-budget            | Persistent named scope; actual child reservations across fresh run IDs; refusal before an extra launch                                |
| oscillation            | Observed stable failure sets replayed through cycle detection                                                                         |
| regression             | Observed stable failure sets replayed through the ratchet                                                                             |
| metric-gaming          | Measured inventory/protected bytes compared independently before success                                                              |
| batch-transformation   | Frozen source inventory; output mapping, format/rules, dimensions/metadata/checksums                                                  |
| static-site            | Real page loading, internal links/assets and required metadata                                                                        |
| seo-metadata           | Real rendered titles, canonical/OG/description and sitemap membership                                                                 |
| build-system           | Actual build/test/boundary commands across the declared packages                                                                      |
| infrastructure-drift   | Local Terraform plan/show/readback; only allowlisted terraform_data actions in an identified disposable sandbox                       |
| creative-workflow      | Protected candidate-bound critic, hard criteria, reviewer veto, score policy, retained best candidates and shared review scope        |

Native OpenAPI handles exact zero-body cases, declared status/header/content
types, local references, object/array/scalar constraints and compositions.
Unsupported validation keywords fail closed; it is not a complete OpenAPI
implementation. Bind a full verifier for parameters, request bodies, format
validation and other unsupported semantics.

Dates use field:GoLayout; ranges use field:min:max. Reconciliation defaults to
exact data equivalence, including duplicates and NULL representations. Numeric
tolerances do not relax row equivalence. SQL's built-in is SQLite; other engines
need a separately implemented trusted oracle. min_dynamic_range means EBU
loudness range in LU, not a promise about musical quality.

Documentation/release fixtures use the explicit local-only origin. They do not
certify deployment. Real origins require options.readback with url and contains.
A local gate does not claim CI or hosted branch enforcement.

## Independent oracle protocol

Asset, Godot, prompt, dependency and creative adapters require options.oracle
as argv, usually ["trusted-verifier","{context}"]. The process must emit one JSON
object, exit 0, and write detailed tool evidence into evidence_dir:

```json
{
  "schema_version": 1,
  "identity": "operator-pinned-verifier/version",
  "artifacts": { "candidate/output.png": "sha256-of-current-bytes" },
  "checks": {
    "technical": true,
    "independent-review": true,
    "reviewer-accepts": true
  },
  "metrics": { "score": 0.97 }
}
```

artifacts must cover the complete current candidate inventory. Required checks
and metric names are enforced by internal/recipeadapter/structured.go. Missing,
nonfinite, negative or unsupported reports fail closed. Extra failed checks
remain failures. The chosen verifier is part of the trust boundary: a
builder-written score file or self-review cannot provide independent assurance.
Keep its scripts, workload, labels, advisory snapshot and policy protected.

The bundled prompt fixture uses a deterministic local token interpreter with
held-out labeled cases and known zero provider cost. It demonstrates evaluator
plumbing, not empirical language-model quality. Bind your actual provider runner
and cost authority for model evaluation. The dependency fixture measures one
historical advisory, [GHSA-35jh-r3h4-6jhm](https://github.com/advisories/GHSA-35jh-r3h4-6jhm);
zero findings within that frozen scope is not a comprehensive current security
assessment. No vulnerable template function is executed.

## Budgets, retention and receipts

cost-budget counts declared child invocations, not hidden calls inside an
arbitrary agent. Provider-backed budgets require wrappers that reserve and
report every provider call. Money remains unknown when a tool supplies no
authoritative cost; local execution never fabricates a free monetary claim.

A creative binding requires adapter.candidate. adapter.score optionally sets
the numeric range/improvement/patience; its target must match target_score and
direction must maximize. Defaults use a 0..1 score. The runtime score hook reads
saved critic evidence; replay never spends another review. Baseline and each
critic call consume max_reviews from a persistent review_scope. The default
scope is stable for the workspace and brief; scope_store can select a shared
relative store. Admission fails before a critic invocation if that scope is
already exhausted. Changing run IDs does not reset it.

Candidate snapshots retain accepted best and rejected evidence; verification
uses the exposed/restored iteration, rather than assigning a rejected report to
the restored bytes. User acceptance, critic veto and technical correctness
remain distinct.

Keep the exact executable, workspace and evidence for recipes verify.
verification.json binds the run state, typed receipt, policy and every retained
measurement file; candidate hashes bind the exposed artifact. Its private
verification.key is a local integrity key, not a third-party signature. A
malicious process with the same user/file access can replace both key and seal;
use independent storage/OS isolation for a stronger adversarial boundary.
Do not publish the key or private tool mappings.

## Reproduce the complete proof matrix

```sh
go run ./tools/recipe-proof -binary build/loopexec -out build/recipe-proof -tools tools.json
```

This invokes all 33 real demos in fresh workspaces, then separately verifies
every receipt. coverage.json binds the exact binary/catalog, computed halts,
acceptance, measurements and receipt hashes. It never restarts a failed demo or
changes a threshold. Tool availability, graphical/loopback access and Docker
permissions are prerequisites; missing tools are failures, not mock successes.
Use installed trusted domain tools in a disposable development environment.
