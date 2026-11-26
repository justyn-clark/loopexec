# Original recipe ideas

User-supplied source, retained without collapsing or dropping any of the 33 ideas.
This is the originating discussion; implementation status is in [recipes](recipes.md).

A good LoopExec demo should make the loop visible: something starts wrong or incomplete, LoopExec iterates against an external check, and the proof ends with a computed halt reason plus receipt. The domain can vary widely.

- Code repair / test convergence
  Start with a failing test suite. --exec runs a repair script or coding agent; --check runs go test, pytest, npm test, etc. Visually capture red tests → iteration count → green tests → success_condition_met → receipt replay. This is the clearest baseline demo.

- Frontend visual regression
  Give an agent a page that is visibly wrong. The loop modifies HTML/CSS, while the external check uses screenshot comparison, DOM assertions, Lighthouse thresholds, or Playwright tests. Show the browser before/after beside the terminal. This is much more visually compelling than pure CLI output.

- Accessibility remediation
  Start with a page failing axe/Playwright accessibility checks. Iterate until critical violations reach zero. Capture the accessibility report dropping from, say, 8 violations → 3 → 0. This is strong because the success criterion is objective and understandable.

- Performance optimization
  Have LoopExec repeatedly optimize a web app or command until a measurable threshold is met: Lighthouse performance score, bundle size, startup time, memory use, query latency, or benchmark duration. Show a chart or terminal values improving each iteration. This demonstrates numeric progress rather than binary pass/fail.

- Image compression / media optimization
  Start with a large image or video and iterate compression parameters until size is below a target while SSIM/VMAF or another quality metric stays above a floor. Visually show file size shrinking while quality stays acceptable. This is a very good non-code demonstration.

- Audio mastering / loudness normalization
  Feed an audio file through an offline processing chain and loop until LUFS, true peak, dynamic range, or spectral constraints fall within target bounds. Show meters or waveform analysis changing between iterations. This fits your audio background particularly well and makes LoopExec look broader than “AI coding infrastructure.”

- Music mix validation
  Iterate on a render until measurable conditions pass: integrated loudness, peak ceiling, silence detection, phase correlation, channel balance, or stem integrity. The external check can use ffmpeg, ffprobe, sox, or custom DSP analysis. A DAW + terminal split-screen would be excellent visually.

- 3D asset optimization
  Use Blender headlessly. Start with a heavy model and iteratively reduce polygon count, texture size, or render cost while preserving a similarity or silhouette threshold. Show Blender statistics or rendered frames beside the LoopExec run. This would be one of the most distinctive demos.

- Godot game performance
  Run a small game scene and iterate until frame time, draw calls, memory, or automated gameplay assertions fall under a threshold. Capture the game running while LoopExec reports objective checks. This makes the project immediately understandable outside backend engineering.

- Responsive UI repair
  Give an agent a page broken at several viewport widths. --check runs Playwright screenshots or layout assertions across mobile/tablet/desktop. LoopExec continues until all viewport checks pass. A tiled visual showing broken → repaired layouts would be extremely shareable.

- Broken API contract repair
  Start with an API implementation that fails an OpenAPI contract test. Iterate until schema validation, status codes, headers, and payloads all pass. Show failing contract cases count decreasing each iteration. This is a strong platform-engineering example.

- Database migration safety
  Run a candidate migration against a disposable database. Check schema invariants, row counts, constraints, backward compatibility, and migration reversibility. LoopExec halts if data integrity regresses. Visually, show the migration attempt plus invariant checks—not flashy, but very credible for senior engineers.

- SQL query optimization
  Give it a slow query and an execution-time target. The loop proposes index/query changes, then the external check uses EXPLAIN ANALYZE and correctness comparisons. Show 850 ms → 260 ms → 74 ms while result hashes remain identical.

- Docker image reduction
  Start with a bloated container image. Iterate Dockerfile changes until image size drops beneath a threshold while health checks/tests still pass. Very easy visual: 1.2 GB → 640 MB → 312 MB, with test status staying green.

- Dependency vulnerability remediation
  Start with a repo containing known vulnerable dependencies. Iterate upgrades or replacements until a scanner reports no high/critical findings while tests still pass. Show vulnerability count falling. Important caveat: frame this as automated remediation assistance, not a security guarantee.

- Lint/type-error convergence
  Begin with dozens of TypeScript, ESLint, compiler, or static-analysis failures. Loop until error count reaches zero. Show the number falling each iteration. This is simple but extremely legible on video.

- Documentation drift repair
  This one is directly relevant to what you just did with LoopExec itself. Start with code/docs mismatch. The loop updates docs while an external verifier checks runtime CLI output, generated docs, version identity, links, and deployment markers. Show stale version → generated mismatch → corrected docs → verification pass. This demonstrates LoopExec governing release integrity rather than code generation.

- Release readiness gate
  Let the loop prepare a release while the check validates tests, generated artifacts, changelog, docs provenance, version consistency, checksums, and site deployment. It stops only when everything is ready or a guard fails. This is a very mature platform-engineering demo.

- Configuration repair
  Start with a broken Nginx, Terraform, GitHub Actions, Docker Compose, or CI config. Iterate against syntax validation plus a real integration test. Capture the config failure becoming a valid deployment. Again, use a disposable local environment for safety.

- Data cleaning
  Give LoopExec a dirty CSV or JSON dataset and an explicit acceptance contract: zero invalid dates, no duplicate IDs, no nulls in required fields, value ranges preserved, row-count constraints satisfied. Show quality metrics improving over iterations. This expands LoopExec into data engineering.

- ETL reconciliation
  Iterate on a transformation until source/target totals, checksums, row counts, and schema contracts reconcile. Show mismatch → decreasing discrepancies → exact reconciliation. Very compelling to enterprise/platform audiences.

- OCR/document extraction quality
  Run several preprocessing strategies on scanned documents until extraction confidence or comparison against known fields passes a threshold. Show image → extracted structured fields → validation. This is more visually interesting than code.

- Prompt/workflow optimization
  Use a fixed evaluation set and iterate a prompt or agent workflow until the external evaluator crosses a target while cost/latency stays under bounds. The important part is that LoopExec evaluates against a frozen test set rather than accepting the model's own judgment. This is especially aligned with the product thesis.

- Cost-bounded agent workflow
  Let an agent try to solve a task under a strict model-call budget. Visually show remaining calls decreasing and LoopExec refusing a further call once the shared budget is exhausted. This is a great demo of the v0.4.x work because the “failure” itself is the proof.

- Oscillation detection
  Construct a deliberately pathological task where an agent alternates between A and B—perhaps formatting one file in mutually incompatible ways. LoopExec detects the repeated state and halts rather than looping forever. Visually this is excellent: A → B → A → B → oscillation_detected.

- Regression detection
  Give a task with two acceptance conditions. The agent fixes one but breaks the other. LoopExec's ratchet catches that regression and refuses to count the iteration as progress. This may be one of the strongest conceptual demos because it exposes a very real agent failure mode.

- Metric gaming / reward hacking
  Create a deliberately bad example where the agent can make the metric look better by weakening the check—for example deleting tests or excluding files. The integrity guard detects the shrinking measured set and halts with metric_integrity_violation. This is extremely strong social content because the failure is instantly understandable.

- File transformation / batch processing
  Convert a directory of files to a required format until every file satisfies a validator: image dimensions, metadata, encoding, naming convention, checksums, etc. This shows LoopExec as a general deterministic workflow engine, not an AI-specific tool.

- Static-site repair
  Crawl a site, find broken links/assets/metadata, iterate fixes, and stop when the crawl returns zero broken links and required metadata exists. You could show a dashboard/count changing live.

- SEO/metadata compliance
  Iterate a site until all pages satisfy technical SEO requirements: title length, canonical URL, OG tags, sitemap membership, no broken internal links. This is objective and very easy to understand visually.

- Build-system repair
  Start with a deliberately broken monorepo build. Iterate until all packages compile, tests pass, and no dependency boundary violations remain. Show the failing workspace count shrinking.

- Infrastructure drift detection
  Compare actual infrastructure state with declared configuration and iterate reconciliation in a sandbox until drift reaches zero. This is suitable for a controlled local/demo environment and shows LoopExec as an infrastructure governor.

- Synthetic creative workflow
  Have independent builder and critic roles produce and evaluate an image, audio clip, 3D asset, UI, or document under bounded attempts. LoopExec governs attempts, budgets, candidate retention, and reviewer rejection. The output itself is visual, while the terminal proves the governance.

For social content, I would prioritize five demos because they tell different stories:

1. Metric gaming caught — the agent “cheats,” LoopExec refuses success.
2. Oscillation caught — the agent keeps undoing itself, LoopExec stops it.
3. Frontend repair — visible broken UI → fixed UI, with an independent browser test.
4. Audio/mastering convergence — measurable creative work outside coding.
5. Cost-budget halt — agent wants another call, LoopExec says the budget is spent.

Those five together communicate something much bigger than “a coding loop”: LoopExec can govern any iterative process where work can be executed and success can be independently measured.
