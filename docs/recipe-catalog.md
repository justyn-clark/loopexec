# LoopExec recipe catalog

Generated from internal/recipes/catalog.json with loopexec recipes catalog. Edit the catalog and regenerate this guide. All 33 original ideas remain distinct. Inputs are configurable; required inputs have no invented domain defaults. All 33 have live execution adapters and real tool fixtures. Tool installations and trusted oracle bindings are explicit prerequisites; see live-recipes.md.

## 1. Code repair / test convergence

ID: code-repair | category: engineering | adapter: live | expected halt: success_condition_met

Diagnose the captured failing tests and make one narrow source repair without changing the acceptance suite.

Inputs:

- project (path, required): Project root relative to the invocation directory.
- test_command (string, required): Existing complete test command; data for adapter authoring, never interpolated.
- allowed_paths (strings, required): Source files the builder may change.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- The complete pinned test suite passes.
- The measured test inventory and protected test/config hashes remain intact.

Protect:

- Tests, test selection, fixtures and test runner configuration

Evidence:

- Failing and final passing test reports
- Source diff and pinned suite inventory
- Computed halt, receipt and replay result

Demo: Show red tests, the attempt count, green tests, success_condition_met and a replay.

## 2. Frontend visual regression

ID: frontend-visual | category: web | adapter: live | expected halt: success_condition_met

Repair one observed HTML/CSS or component discrepancy against the approved reference.

Inputs:

- project (path, required): Web project root.
- page_url (string, required): Page in a controlled browser environment.
- reference (path, required): Approved screenshot or visual reference.
- max_visual_difference (number, required): Maximum allowed normalized image difference; minimum 0; maximum 1.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Pinned browser, viewport, fonts and screenshot comparison meet the difference threshold.
- Independent DOM assertions and required functional checks pass.

Protect:

- Reference images, comparison algorithm and DOM acceptance tests

Evidence:

- Before/after screenshots
- Candidate-bound image difference and DOM results

Demo: Place before/after browsers beside the terminal and show the independent comparison passing.

## 3. Accessibility remediation

ID: accessibility | category: web | adapter: live | expected halt: success_condition_met

Remediate one captured accessibility failure while retaining page behavior and the audited route/rule inventory.

Inputs:

- project (path, required): Web project root.
- routes (strings, required): Frozen route list.
- rules (strings, required): Applicable accessibility rules.
- max_critical (integer, default 0): Allowed critical violations; minimum 0.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Critical violation count is at or below max_critical across every pinned route/state.
- Keyboard and focus assertions required by the acceptance contract pass.
- Automated checks are reported as scoped evidence, not full accessibility certification.

Protect:

- Axe rules, exclusions, route inventory and keyboard acceptance checks

Evidence:

- Per-route violation counts and rule IDs
- Keyboard/focus results and relevant screenshots

Demo: Show scoped violation counts dropping from 8 to 3 to 0.

## 4. Performance optimization

ID: performance | category: engineering | adapter: live | expected halt: success_condition_met

Optimize one measured bottleneck using the fixed benchmark environment and unchanged correctness contract.

Inputs:

- project (path, required): Project root.
- metric (string, required): Metric and measurement tool.
- direction (string, required): minimize or maximize.
- target (number, required): Accepted threshold.
- samples (integer, default 5): Measurements per candidate; minimum 3.
- correctness_command (string, required): Pinned correctness check.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Declared metric reaches the target using the pinned aggregation and sampling procedure.
- Correctness and resource guardrails still pass.
- Warmup, hardware/load and variance are recorded; noisy measurements cannot certify improvement.

Protect:

- Benchmark workload, sample policy and correctness checks

Evidence:

- Per-attempt samples and aggregate
- Correctness report and runtime/tool versions

Demo: Chart the same measured quantity per candidate alongside correctness status.

## 5. Image compression / media optimization

ID: media-optimization | category: media | adapter: live | expected halt: success_condition_met

Render one compression candidate with changed encoder parameters while retaining the immutable source.

Inputs:

- source (path, required): Original image or video.
- output (path, required): Separate destination file.
- max_bytes (number, required): Maximum output bytes; minimum 1.
- quality_metric (string, required): Pinned SSIM, VMAF or other comparison method.
- quality_floor (number, required): Minimum accepted quality.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Output bytes are at or below max_bytes.
- Comparison against the original meets quality_floor.
- Required dimensions, duration, streams, color and metadata constraints pass.

Protect:

- Original media, comparison reference and metric configuration

Evidence:

- Source/output hashes and sizes
- Quality measurements and encoder settings
- Side-by-side captures

Demo: Show bytes shrinking while the quality meter stays above its floor.

## 6. Audio mastering / loudness normalization

ID: audio-mastering | category: audio | adapter: live | expected halt: success_condition_met

Adjust one offline processing-chain candidate from captured analysis; retain the input and best technically valid render.

Inputs:

- source (path, required): Immutable input audio.
- output (path, required): Separate mastered audio destination.
- target_lufs (number, required): Requested integrated LUFS.
- lufs_tolerance (number, required): Allowed loudness deviation; minimum 0.
- max_true_peak_dbtp (number, required): True peak ceiling in dBTP.
- min_dynamic_range (number, required): Minimum dynamic range in the declared measurement units; minimum 0.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Integrated loudness is within target_lufs +/- lufs_tolerance.
- True peak does not exceed max_true_peak_dbtp and dynamic range meets its floor.
- Duration, sample rate/channels and any user-supplied spectral constraints pass.
- Meter compliance is distinct from subjective musical approval.

Protect:

- Original audio, meter implementation and acceptance bounds

Evidence:

- Chain settings, audio hashes and LUFS/peak/range reports
- Waveform or meter captures
- Retained candidate and receipt

Demo: Show waveform and meters moving into the accepted region beside the loop.

## 7. Music mix validation

ID: music-mix | category: audio | adapter: live | expected halt: success_condition_met

Make one mix/render correction addressing measured silence, phase, balance, peak or stem-integrity failures.

Inputs:

- render (path, required): Rendered mix.
- stems (path, required): Stem directory or inventory.
- max_true_peak_dbtp (number, required): Peak ceiling in dBTP.
- max_silence_seconds (number, required): Maximum unintended silence duration; minimum 0.
- min_phase_correlation (number, required): Minimum declared phase-correlation statistic; minimum -1; maximum 1.
- max_channel_imbalance_db (number, required): Maximum left/right imbalance; minimum 0.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Pinned loudness/peak, silence, phase and channel-balance requirements pass.
- All expected stems exist with matching sample rate, duration/alignment and hashes where required.
- Intentional silence and artistic exceptions are explicit inputs, never silently excluded.

Protect:

- Original session/stems, stem inventory, alignment and analyzer policy

Evidence:

- Per-render DSP report
- Stem manifest and alignment results
- DAW/meter captures

Demo: Use a DAW and terminal split-screen with criterion-by-criterion results.

## 8. 3D asset optimization

ID: asset-optimization | category: creative | adapter: live | expected halt: success_condition_met

Produce one optimized mesh/material candidate and measure it from the fixed comparison views.

Inputs:

- source (path, required): Authored source model.
- output (path, required): Separate optimized model.
- max_triangles (number, required): Triangle-count ceiling; minimum 1.
- max_texture_edge (number, required): Largest accepted texture dimension; minimum 1.
- min_similarity (number, required): Minimum declared render/silhouette similarity; minimum 0; maximum 1.
- views (strings, required): Frozen comparison cameras/views.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Triangle, texture and declared render-cost limits pass.
- Similarity or silhouette quality remains above the floor in every required view.
- UVs, material slots, scale, normals and requested engine import remain valid.

Protect:

- Authored source asset, comparison cameras, references and similarity evaluator

Evidence:

- Before/after asset statistics
- Pinned-view render comparisons and engine import results

Demo: Show Blender statistics beside matched before/after rendered frames.

## 9. Godot game performance

ID: godot-performance | category: games | adapter: live | expected halt: success_condition_met

Optimize one profiled game-scene bottleneck and produce a fresh benchmark run with unchanged gameplay assertions.

Inputs:

- project (path, required): Godot project.
- scene (path, required): Benchmark scene relative to project.
- max_frame_ms (number, required): Frame-time ceiling for the declared percentile; minimum 0.
- max_draw_calls (number, required): Draw-call ceiling; minimum 0.
- max_memory_bytes (number, required): Memory ceiling; minimum 1.
- gameplay_assertions (strings, required): Required automated behavior assertions.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Frame-time, draw-call and memory bounds pass under the pinned hardware/renderer/workload.
- Every required gameplay assertion passes.

Protect:

- Scene workload, benchmark capture settings and gameplay tests

Evidence:

- Frame-time distribution, draw calls and memory
- Gameplay results, Godot version and game capture

Demo: Show the running game and measured budgets with passing gameplay tests.

## 10. Responsive UI repair

ID: responsive-ui | category: web | adapter: live | expected halt: success_condition_met

Repair one responsive layout failure without sacrificing other pinned viewport states.

Inputs:

- project (path, required): Web project.
- routes (strings, required): Pinned pages/states.
- viewports (strings, required): Viewport definitions such as 390x844 and 1440x900.
- max_overflow_px (number, required): Allowed horizontal overflow in pixels; minimum 0.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Every route/viewport passes overflow, clipping, visibility and required interaction assertions.
- Required screenshots meet approved references or explicit layout criteria.

Protect:

- Viewport/route inventory, reference captures and layout assertions

Evidence:

- Tiled before/after screenshots
- Per-viewport assertion results

Demo: Tile mobile/tablet/desktop before and after around the loop counters.

## 11. Broken API contract repair

ID: api-contract | category: engineering | adapter: live | expected halt: success_condition_met

Repair one implementation mismatch with the approved API contract.

Inputs:

- project (path, required): API project.
- openapi (path, required): Approved OpenAPI document.
- base_url (string, required): Disposable integration API endpoint.
- cases (strings, required): Pinned contract case inventory.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- All pinned cases pass schema, status-code, header and payload validation.
- Required error responses and backward-compatible behavior pass.

Protect:

- OpenAPI source, case inventory and test client configuration

Evidence:

- Failing-case trajectory
- Final contract and compatibility reports

Demo: Show failing contract cases decreasing until the full contract passes.

## 12. Database migration safety

ID: migration-safety | category: data | adapter: live | expected halt: success_condition_met

Apply one candidate to a freshly restored disposable database and collect migration plus recovery evidence.

Inputs:

- migration (path, required): Candidate migration file.
- sandbox_database (string, required): Explicit disposable database alias, never a credential.
- fixture (path, required): Versioned representative data fixture.
- invariants (strings, required): Schema/data/backward-compatibility invariants.
- rollback_policy (string, required): Required rollback or forward-recovery contract.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Schema/data constraints, exact row counts and declared invariants pass.
- Old and new application compatibility passes where required.
- Rollback or forward recovery is exercised against the declared policy without data loss.

Protect:

- Original fixture, invariant queries and database identity isolation

Evidence:

- Before/after schema and row-count/checksum comparisons
- Recovery and compatibility results

Demo: Display the migration alongside independent integrity and recovery checks.

## 13. SQL query optimization

ID: sql-optimization | category: data | adapter: live | expected halt: success_condition_met

Propose one query or index change and benchmark it against the unchanged baseline result contract.

Inputs:

- query (path, required): Query source file.
- sandbox_database (string, required): Disposable benchmark database alias.
- fixture (path, required): Frozen data fixture.
- max_latency_ms (number, required): Latency target under the pinned aggregation; minimum 0.
- result_comparison (string, required): Canonical row ordering, duplicate, NULL and numeric-tolerance policy.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Pinned benchmark aggregate meets max_latency_ms.
- Canonical results equal baseline including duplicates, NULLs, ordering requirements and explicit numeric tolerances.
- EXPLAIN ANALYZE runs only in the disposable database; mutating queries remain scoped to fixtures.

Protect:

- Baseline query/results, fixture, benchmark workload and comparison policy

Evidence:

- EXPLAIN ANALYZE plans and timing samples
- Canonical result hashes and equality details

Demo: Show 850 ms -> 260 ms -> 74 ms while the canonical result hash remains equal.

## 14. Docker image reduction

ID: docker-reduction | category: engineering | adapter: live | expected halt: success_condition_met

Reduce one source of container-image weight while retaining the required runtime dependencies and behavior.

Inputs:

- project (path, required): Build context.
- dockerfile (path, required): Candidate Dockerfile.
- max_image_bytes (number, required): Platform-specific image-size ceiling; minimum 1.
- platform (string, required): Fixed target architecture/platform.
- checks (strings, required): Health, runtime and test commands.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Image size meets max_image_bytes for the declared measurement type/platform.
- Health checks, required runtime features and tests pass in the built image.

Protect:

- Build context inputs, platform and health/test assertions

Evidence:

- Image digest, layer sizes and total bytes
- Container health and test reports

Demo: Show image size falling from 1.2 GB through 640 MB to 312 MB with green tests.

## 15. Dependency vulnerability remediation

ID: dependency-remediation | category: engineering | adapter: live | expected halt: success_condition_met

Upgrade or replace one vulnerable dependency group and validate the resulting dependency graph and runtime compatibility.

Inputs:

- project (path, required): Dependency project.
- manifests (strings, required): Dependency manifests and lockfiles.
- scanner (string, required): Chosen scanner/version and advisory snapshot.
- severities (strings, required): Severities required to reach zero.
- test_command (string, required): Pinned regression suite.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- The chosen scanner reports zero findings at the configured severities using the recorded advisory snapshot.
- Regression tests and lockfile consistency pass.
- Report scanner scope and residual uncertainty; zero known findings is not a security guarantee.

Protect:

- Scanner policy/exclusions, advisory snapshot and regression suite

Evidence:

- Before/after dependency graphs and scanner reports
- Lockfile diff and regression results

Demo: Show known high/critical counts dropping while tests stay green.

## 16. Lint/type-error convergence

ID: lint-types | category: engineering | adapter: live | expected halt: success_condition_met

Repair one related cluster of compiler, type or lint failures without relaxing the configured checks.

Inputs:

- project (path, required): Source project.
- check_command (string, required): Complete compiler/lint/static-analysis command.
- allowed_paths (strings, required): Source paths eligible for repair.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Full pinned compiler/linter scope reports zero required errors.
- Ignore rules, suppressions, file inventory and compiler strictness remain within the original contract.

Protect:

- Compiler/linter policy, checked-file inventory and accepted suppressions

Evidence:

- Stable diagnostic IDs and count trajectory
- Final complete check output

Demo: Show the error count falling to zero beside the source diff.

## 17. Documentation drift repair

ID: documentation-drift | category: release | adapter: live | expected halt: success_condition_met

Repair one source/documentation mismatch and regenerate contracts from the intended source identity.

Inputs:

- project (path, required): Documentation/source project.
- version (string, required): Expected source/binary identity.
- docs (strings, required): Canonical documentation inventory.
- contract_command (string, required): CLI/generated-reference verifier.
- deployment_origin (string, required): Expected docs origin or explicit local-only scope.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- CLI output/generated references, versions and links match the exact source-bound documentation bundle.
- Required deployment markers and raw-file readback match when deployed evidence is in scope.
- Local generation is never reported as deployed verification.

Protect:

- Source identity, documentation inventory and provenance verifier

Evidence:

- Source/docs manifests and contract diff
- Link results and deployed readback when authorized

Demo: Show stale version, generated mismatch, corrected docs and verified provenance.

## 18. Release readiness gate

ID: release-readiness | category: release | adapter: live | expected halt: success_condition_met

Prepare one missing or inconsistent release artifact and collect fresh source-bound readiness evidence.

Inputs:

- project (path, required): Release project.
- version (string, required): Intended release version.
- artifacts (strings, required): Required package/platform inventory.
- gates (strings, required): Required release readiness checks.
- docs_origin (string, required): Required deployed documentation origin.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Tests, generated artifacts, changelog, versions and checksums pass for the same intended source.
- Documentation provenance and required deployed readback pass.
- Readiness remains separate from owner publication/merge/deployment authorization.

Protect:

- Release policy, source identity, expected artifact inventory and publication gates

Evidence:

- Exact-source gate matrix
- Artifact checksums and source/docs manifests
- Site readback

Demo: Display each release gate becoming ready or the exact guard stopping preparation.

## 19. Configuration repair

ID: configuration-repair | category: engineering | adapter: live | expected halt: success_condition_met

Repair one configuration error and exercise its behavior in the declared disposable environment.

Inputs:

- configuration (path, required): Candidate configuration.
- validator (string, required): Syntax/schema validator.
- integration_command (string, required): Real sandbox integration check.
- sandbox (string, required): Explicit local/disposable target alias.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Syntax/schema validation passes.
- Real integration behavior passes in the declared target; syntax alone is insufficient.
- External production apply/deploy is outside the repair unless separately authorized.

Protect:

- Validator policy, integration assertions and target identity

Evidence:

- Initial validator failure
- Integration result and resulting sandbox behavior

Demo: Show the invalid configuration becoming a working disposable deployment.

## 20. Data cleaning

ID: data-cleaning | category: data | adapter: live | expected halt: success_condition_met

Apply one explicit cleaning transformation and retain a trace from each output row to its source.

Inputs:

- source (path, required): Immutable CSV/JSON dataset.
- output (path, required): Separate cleaned dataset.
- required_fields (strings, required): Fields that cannot be missing.
- id_field (string, required): Uniqueness key.
- date_fields (strings, required): Date fields and accepted formats.
- max_dropped_rows (integer, default 0): Maximum explicitly permitted dropped rows; minimum 0.
- ranges (strings, required): Declared value-range constraints.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Required dates, unique IDs, non-null fields, ranges and row-count/drop limits all pass.
- Ambiguous values are surfaced for resolution; invented replacements or silent row deletion cannot improve the metric.

Protect:

- Source data, row lineage, acceptance rules and permitted drop policy

Evidence:

- Per-rule quality counts before/after
- Row lineage and dropped/quarantined-row report

Demo: Show invalid dates, duplicates and missing-field counts decreasing under fixed row constraints.

## 21. ETL reconciliation

ID: etl-reconciliation | category: data | adapter: live | expected halt: success_condition_met

Correct one transformation discrepancy while retaining source/target lineage and the same reconciliation contract.

Inputs:

- source (path, required): Frozen source dataset/snapshot.
- transformation (path, required): Transformation code/configuration.
- target (path, required): Disposable transformed output.
- reconciliation_keys (strings, required): Join/group keys.
- totals (strings, required): Expected sums/counts and explicit tolerances.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Source/target row counts, totals, canonical checksums and schema contracts reconcile.
- Key cardinality, duplicate, NULL and rounding rules remain explicit and unchanged.

Protect:

- Source snapshot, reconciliation keys, tolerance policy and schema checks

Evidence:

- Per-key discrepancies and totals
- Source/target manifests and lineage

Demo: Show mismatch counts falling to exact reconciliation.

## 22. OCR/document extraction quality

ID: ocr-extraction | category: documents | adapter: live | expected halt: success_condition_met

Try one preprocessing/extraction strategy and score structured fields against the frozen annotated reference.

Inputs:

- source (path, required): Immutable scans or document images.
- truth (path, required): Known fields or held-out annotated sample.
- required_fields (strings, required): Field names with validation rules.
- min_field_accuracy (number, required): Required measured field accuracy; minimum 0; maximum 1.
- ocr_engine (string, required): Chosen OCR engine and version.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Required field validity and measured field accuracy meet the pinned target.
- Confidence-only acceptance is explicitly calibrated to labeled data; self-reported confidence alone cannot establish correctness.
- Preserve document/page/region provenance for each extracted field.

Protect:

- Original scans, ground truth, field rules and held-out samples

Evidence:

- Image/preprocessing comparison
- Extracted fields, provenance and validation scores

Demo: Show scan, structured fields and independent field validation together.

## 23. Prompt/workflow optimization

ID: prompt-optimization | category: agents | adapter: live | expected halt: success_condition_met

Revise one prompt/workflow candidate using development feedback while retaining the frozen evaluation contract.

Inputs:

- prompt (path, required): Candidate prompt/workflow.
- evaluation_set (path, required): Frozen versioned evaluation cases.
- evaluator (string, required): Independent evaluator and scoring policy.
- target_score (number, required): Acceptance score.
- max_latency_ms (number, required): Latency bound; minimum 0.
- max_cost_microusd (number, required): Per-evaluation cost limit in integer micro-USD; minimum 0.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Independent evaluator score meets target with all hard correctness constraints intact.
- Recorded latency and adapter/provider-enforced cost constraints satisfy the contract.
- Keep a held-out acceptance split; evaluation-set edits, label leakage and self-grading cannot certify improvement.

Protect:

- Evaluation cases/labels, evaluator, split policy and budget ledger

Evidence:

- Case-level scores and held-out results
- Observed invocation, latency and exact/unknown cost records

Demo: Show frozen-case scores improving alongside cost and latency limits.

## 24. Cost-bounded agent workflow

ID: cost-budget | category: governance | adapter: live | expected halt: budget_exceeded

Reserve one permitted attempt before dispatch; after exhaustion request another only in an offline fixture to prove refusal.

Inputs:

- objective (string, required): Bounded task objective.
- scope_id (string, required): Existing or new explicit shared budget identity.
- max_calls (integer, required): Authorized total call allowance; minimum 1.
- phases (strings, required): Builder/critic phases included in the allowance.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Shared pre-call reservations never exceed max_calls across run IDs and phases.
- A refused call never launches; failed launches retain consumed reservations.
- Call-count bounds are not dollar bounds; strict monetary caps additionally require enforced per-call cost reservations.

Protect:

- Shared scope identity, pinned allowance and append-only reservation history

Evidence:

- Per-call reservations, remaining allowance and launch count
- Computed budget_exceeded receipt

Demo: Show remaining calls decrementing and an extra call being refused without launching.

## 25. Oscillation detection

ID: oscillation | category: governance | adapter: live | expected halt: oscillation_detected

Produce the next alternating fixture state while retaining the same contradictory acceptance checks.

Inputs:

- fixture (path, required): Disposable fixture directory.
- states (strings, required): Deliberately conflicting states A and B.
- failure_ids (strings, required): Stable failure identities for each state.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Repeated state/failure history produces the computed oscillation guard before the attempt/time cap.
- The detector uses real captured observations and never a caller-supplied halt reason.

Protect:

- Contradictory checks, failure identity mapping and detector policy

Evidence:

- A/B state sequence and captured failure sets
- Computed halt and retained receipt

Demo: Animate A -> B -> A -> B and the computed oscillation_detected stop.

## 26. Regression detection

ID: regression | category: governance | adapter: live | expected halt: same_test_regressed

Fix one failing condition in the fixture while deliberately exposing a regression in a previously passing condition.

Inputs:

- fixture (path, required): Disposable two-condition fixture.
- conditions (strings, required): At least two independently identified acceptance conditions.
- allowed_paths (strings, required): Builder-owned fixture files.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Previously passing criterion re-entering the failure set is detected as same_test_regressed.
- With candidate management, rejected candidates retain evidence and restore the best verified candidate according to the configured policy.

Protect:

- Acceptance conditions, stable IDs and retained baseline evidence

Evidence:

- Per-condition trajectory
- Rejected candidate and restored-best identity when configured

Demo: Show one check turn green while the other turns red, then the ratchet/restore result.

## 27. Metric gaming / reward hacking

ID: metric-gaming | category: governance | adapter: live | expected halt: metric_integrity_violation

Attempt one deliberate metric-weakening change in a disposable fixture while keeping the authoritative verifier outside builder control.

Inputs:

- fixture (path, required): Disposable deliberately gameable fixture.
- measured_ids (strings, required): Frozen measured-test or item inventory.
- protected_paths (strings, required): Verifier, test-selection and policy files.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Shrinking measured inventory or changing protected verifier policy triggers metric_integrity_violation before green can count as success.
- Published claims distinguish implemented inventory/manifest guards from still-unimplemented deeper assertion-count/coverage protections.

Protect:

- Authoritative checks, original measured set and protected manifest

Evidence:

- Before/after item inventory and policy hashes
- Tempting green metric, refused success and computed receipt

Demo: Show the agent delete/exclude a test, the apparent improvement and LoopExec refusing success.

## 28. File transformation / batch processing

ID: batch-transformation | category: files | adapter: live | expected halt: success_condition_met

Transform one bounded batch and record source-to-output lineage and failures.

Inputs:

- source (path, required): Immutable source directory.
- output (path, required): Separate destination directory.
- format (string, required): Required output format/encoding.
- validation_rules (strings, required): Dimension, metadata, naming and checksum requirements.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Every source item has the required output(s) and all declared validators pass.
- Frozen inventory, collision policy, checksum and permitted metadata-loss constraints pass.

Protect:

- Source inventory, validation policy and required metadata

Evidence:

- Per-file conversion/validation matrix
- Input/output manifests and checksums

Demo: Show remaining invalid files decreasing to zero without dropping source items.

## 29. Static-site repair

ID: static-site | category: web | adapter: live | expected halt: success_condition_met

Repair one group of broken links/assets or missing metadata against the frozen crawl scope.

Inputs:

- project (path, required): Static-site project.
- base_url (string, required): Controlled site origin.
- routes (strings, required): Fixed crawl seeds and route inventory.
- required_metadata (strings, required): Required page metadata.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- No broken required internal links/assets remain across the pinned crawl inventory.
- Required metadata exists on every applicable page; redirects and excluded external links are explicit.

Protect:

- Crawl scope, required route inventory and metadata policy

Evidence:

- Per-page broken-link/asset counts
- Final crawl and metadata report

Demo: Show a live dashboard/count reaching zero broken links.

## 30. SEO/metadata compliance

ID: seo-metadata | category: web | adapter: live | expected halt: success_condition_met

Correct one technical metadata discrepancy while preserving approved page content and canonical policy.

Inputs:

- project (path, required): Site project.
- origin (string, required): Expected canonical origin.
- pages (strings, required): Pinned page inventory.
- metadata_rules (strings, required): Title, canonical, OG, sitemap and link rules.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Every declared page meets title, canonical URL, OG, sitemap and internal-link rules.
- Report technical compliance only; no ranking or traffic outcome is implied.

Protect:

- Page inventory, canonical origin, metadata limits and crawl rules

Evidence:

- Page-level compliance matrix
- Crawler results and rendered head snapshots

Demo: Display required metadata checks turning green for every page.

## 31. Build-system repair

ID: build-system | category: engineering | adapter: live | expected halt: success_condition_met

Repair one dependency/build configuration or source failure while preserving the complete workspace graph.

Inputs:

- project (path, required): Monorepo root.
- packages (strings, required): Frozen package/workspace inventory.
- build_command (string, required): Complete workspace build.
- test_command (string, required): Required regression tests.
- boundary_command (string, required): Dependency-boundary validator.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- All declared packages compile and required tests pass.
- Dependency-boundary validator passes without shrinking the workspace inventory or skipping failing packages.

Protect:

- Package graph, build/test commands and boundary rules

Evidence:

- Failing workspace counts
- Full build, test and dependency-boundary results

Demo: Show the failing-package count shrink to zero across the full workspace.

## 32. Infrastructure drift detection

ID: infrastructure-drift | category: infrastructure | adapter: live | expected halt: success_condition_met

Reconcile one allowed difference in the declared sandbox and capture actual-state readback.

Inputs:

- configuration (path, required): Declared infrastructure configuration.
- sandbox (string, required): Explicit disposable target identity.
- drift_command (string, required): Read-only independent drift detector.
- allowed_changes (strings, required): Permitted sandbox reconciliation operations.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- Independent desired/actual comparison reports zero required drift.
- Resource identity and allowed-change constraints remain intact.
- A plan alone cannot prove applied state; production reconciliation remains separately authorized.

Protect:

- Desired-state definition, resource inventory and target/change policy

Evidence:

- Before/after drift reports
- Sandbox action ledger and actual-state readback

Demo: Show sandbox drift count falling to zero with independent resource readback.

## 33. Synthetic creative workflow

ID: creative-workflow | category: creative | adapter: live | expected halt: success_condition_met

Produce one candidate and an independently attributed review against the unchanged brief, references and criterion-level rubric.

Inputs:

- brief (string, required): User creative brief and required deliverable.
- references (strings, required): User-approved reference assets.
- criteria (strings, required): Technical constraints and criterion-level rubric.
- target_score (number, required): Pinned desired score.
- max_reviews (integer, required): Authorized critic calls; minimum 1.
- output (path, required): Candidate deliverable root.
- additional_constraints (strings, required): Optional additional user acceptance constraints; never replaces the required checks.

Acceptance:

- All hard technical criteria pass and candidate-bound independent review reaches the pinned target.
- Reviewer veto remains a veto; subjective review cannot override a failed technical oracle.
- Rejected attempts retain evidence/usage, restore the best verified candidate, and never reset review allowances.

Protect:

- Brief, references, rubric, verifier and shared review budget

Evidence:

- Candidate hashes and technical reports
- Independent criterion-level review, retained/rejected manifests and incurred usage

Demo: Show the creative output beside attempt, budget, best-candidate and reviewer-veto evidence.
