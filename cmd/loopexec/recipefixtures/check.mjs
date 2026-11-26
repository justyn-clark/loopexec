import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createServer, get } from "node:http";
import { execFileSync } from "node:child_process";
const mode = process.argv[2],
  root = mode === "boundaries" ? resolve("..") : process.cwd();
const read = (p) => readFileSync(resolve(root, p), "utf8");
if (mode === "boundaries") {
  if (/\bimport\b/.test(read("candidate/main.go")))
    throw Error("undeclared dependency");
} else if (mode === "config-syntax") {
  JSON.parse(read("candidate/compose.json"));
} else if (mode === "config-integration") {
  const cfg = JSON.parse(read("candidate/compose.json"));
  if (cfg.services.app.port < 1 || cfg.services.app.port > 65535)
    throw Error("invalid service port");
  const s = createServer((q, r) => r.end(String(cfg.services.app.port)));
  await new Promise((r) => s.listen(0, "127.0.0.1", r));
  try {
    const body = await new Promise((res, rej) =>
      get("http://127.0.0.1:" + s.address().port, (r) => {
        let b = "";
        r.on("data", (c) => (b += c));
        r.on("end", () => res(b));
      }).on("error", rej),
    );
    if (body !== "8080") throw Error("runtime config mismatch");
  } finally {
    s.close();
  }
} else if (mode === "benchmark") {
  if (
    execFileSync("/bin/sh", ["candidate/benchmark.sh"], {
      encoding: "utf8",
    }).trim() !== "42"
  )
    throw Error("incorrect workload result");
} else if (mode === "version") {
  console.log("1.2.3");
} else if (mode === "docs") {
  if (!read("candidate/guide.md").includes("1.2.3"))
    throw Error("contract drift");
} else if (mode === "dependencies") {
  const require = (await import("node:module")).createRequire(
    resolve(root, "package.json"),
  );
  const _ = require("lodash");
  if (JSON.stringify(_.chunk([1, 2, 3], 2)) !== "[[1,2],[3]]")
    throw Error("runtime regression");
} else throw Error("unknown check");
