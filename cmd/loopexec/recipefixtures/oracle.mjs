import { readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { resolve, relative, join } from "node:path";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { performance } from "node:perf_hooks";
const c = JSON.parse(readFileSync(process.argv[2], "utf8")),
  i = c.inputs,
  o = c.adapter.options,
  tool = (n) => c.adapter.tools?.[n] || n,
  p = (p) => resolve(c.root, p);
const run = (name, args) =>
  execFileSync(tool(name), args, {
    cwd: c.root,
    encoding: "utf8",
    maxBuffer: 8 * 1024 * 1024,
    timeout: c.timeout_seconds * 1000,
  });
const report = {
  schema_version: 1,
  identity: "jc-demo-independent-oracle-v1/" + c.recipe,
  artifacts: {},
  checks: {},
  metrics: {},
};
if (c.recipe === "prompt-optimization") {
  const dataset = JSON.parse(readFileSync(p(i.evaluation_set), "utf8")),
    prompt = readFileSync(p(i.prompt), "utf8");
  let good = 0,
    held = 0;
  const started = performance.now(),
    results = [];
  for (const item of dataset) {
    const prediction = prompt.includes("lowercase")
      ? item.input.toLowerCase()
      : item.input;
    const pass = prediction === item.label;
    if (pass) good++;
    if (item.split === "holdout") held++;
    results.push({ id: item.id, prediction, passed: pass, split: item.split });
  }
  report.checks = {
    "frozen-cases": results.length === dataset.length,
    "held-out": held > 0,
    "hard-correctness": true,
    "cost-known": true,
  };
  report.metrics = {
    score: good / dataset.length,
    latency_ms: performance.now() - started,
    cost_microusd: 0,
  };
  writeFileSync(
    join(c.evidence_dir, "case-results.json"),
    JSON.stringify(results),
  );
} else if (c.recipe === "dependency-remediation") {
  const manifest = JSON.parse(readFileSync(p(i.manifests[0]), "utf8")),
    lock = JSON.parse(readFileSync(p(i.manifests[1]), "utf8")),
    advisory = JSON.parse(readFileSync(p(o.advisory), "utf8"));
  const installed = JSON.parse(
    readFileSync(p("candidate/node_modules/lodash/package.json"), "utf8"),
  ).version;
  const version = lock.packages["node_modules/lodash"].version;
  const cmp = (a, b) => {
    const x = a.split(".").map(Number),
      y = b.split(".").map(Number);
    for (let k = 0; k < 3; k++) {
      if (x[k] !== y[k]) return x[k] - y[k];
    }
    return 0;
  };
  report.checks = {
    "lock-consistency":
      manifest.dependencies.lodash === version && installed === version,
    "scanner-complete": advisory.id === "GHSA-35jh-r3h4-6jhm",
  };
  for (const file of i.manifests)
    report.checks["manifest:" + file] = statSync(p(file)).isFile();
  for (const severity of i.severities)
    report.metrics["findings:" + severity] =
      severity === "high" && cmp(version, advisory.fixed) < 0 ? 1 : 0;
  writeFileSync(
    join(c.evidence_dir, "advisory-scope.json"),
    JSON.stringify({
      scanner: i.scanner,
      advisory,
      observed: version,
      installed,
    }),
  );
} else if (c.recipe === "creative-workflow") {
  let log = "";
  // FFmpeg sends the actual metric to stderr.
  const result = (await import("node:child_process")).spawnSync(
    tool("ffmpeg"),
    [
      "-hide_banner",
      "-nostats",
      "-i",
      p(i.references[0]),
      "-i",
      p(i.output),
      "-lavfi",
      "[0:v][1:v]ssim",
      "-f",
      "null",
      "-",
    ],
    { encoding: "utf8" },
  );
  if (result.status !== 0) throw Error(result.stderr);
  log = result.stderr;
  const score = Number([...log.matchAll(/All:([0-9.]+)/g)].at(-1)?.[1]);
  if (!Number.isFinite(score)) throw Error("missing image metric");
  report.metrics = { score };
  report.checks = {
    "independent-review": true,
    technical: true,
    "reviewer-accepts": score >= i.target_score,
  };
  for (const criterion of i.criteria) {
    if (!["dimensions", "palette"].includes(criterion))
      throw Error("unsupported demo criterion");
    report.checks[criterion] =
      criterion === "dimensions" || score >= i.target_score;
  }
  writeFileSync(join(c.evidence_dir, "critic-ssim.txt"), log);
} else if (c.recipe === "asset-optimization") {
  const log = run("blender", [
    "--background",
    "--factory-startup",
    "--python",
    p("references/asset.py"),
    "--",
    "measure",
    process.argv[2],
  ]);
  writeFileSync(join(c.evidence_dir, "blender.log"), log);
  Object.assign(
    report,
    JSON.parse(log.split("RECIPE_REPORT:").at(-1).split("\n")[0]),
  );
} else if (c.recipe === "godot-performance") {
  const log = run("godot", [
    "--path",
    p(i.project),
    "--rendering-method",
    "gl_compatibility",
    "--resolution",
    "320x240",
    "--audio-driver",
    "Dummy",
    "--script",
    p("references/benchmark.gd"),
    "--",
    process.argv[2],
  ]);
  writeFileSync(join(c.evidence_dir, "godot.log"), log);
  Object.assign(
    report,
    JSON.parse(log.split("RECIPE_REPORT:").at(-1).split("\n")[0]),
  );
} else throw Error("unknown oracle");
function inventory(path) {
  const s = statSync(path);
  if (s.isDirectory()) {
    for (const name of readdirSync(path).sort()) inventory(join(path, name));
  } else
    report.artifacts[relative(c.root, path).split("\\").join("/")] = createHash(
      "sha256",
    )
      .update(readFileSync(path))
      .digest("hex");
}
for (const path of c.adapter.artifacts) inventory(p(path));
process.stdout.write(JSON.stringify(report));
