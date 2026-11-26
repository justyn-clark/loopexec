// Behavioral test against an exported package; no Pi process or model is started.
import assert from "node:assert/strict";
import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { resolve } from "node:path";

const pack = resolve(process.argv[2]);
const catalog = JSON.parse(readFileSync(resolve(pack, "catalog.json"), "utf8"));
const { default: extension } = await import(
  pathToFileURL(resolve(pack, "extensions/jc-recipes.mjs"))
);
const commands = new Map();
const calls = [];
const messages = [];
const notices = [];
let mode = "normal";
const pi = {
  registerCommand(name, options) {
    commands.set(name, options);
  },
  sendMessage(message) {
    messages.push(message);
  },
  async exec(command, args, options) {
    calls.push({ command, args, options });
    let result = { catalog_sha256: catalog.catalog_sha256 };
    if (args[1] === "suggest") {
      if (mode !== "empty")
        result.suggestions = [{ recipe: catalog.recipes[0] }];
    } else if (args[1] === "list") {
      result.entries = catalog.recipes;
    } else if (args[1] === "run" || args[1] === "verify") {
      result.verification = {
        receipt_verified: true,
        halt_reason: "budget_exceeded",
        acceptance_met: false,
      };
    } else {
      const overridesIndex = args.indexOf("--inputs-json");
      const inputs =
        overridesIndex < 0 ? {} : JSON.parse(args[overridesIndex + 1]);
      result.plan = {
        readiness: overridesIndex < 0 ? "needs_inputs" : "adapter_required",
        execution_ready: false,
        missing_inputs: overridesIndex < 0 ? ["source", "target_lufs"] : [],
        inputs,
      };
    }
    if (mode === "ready" && result.plan) {
      result.plan.execution_ready = true;
      result.plan.missing_inputs = [];
      result.plan.readiness = "configured";
    }
    if (mode === "drift") result.catalog_sha256 = "different";
    return {
      code: args[1] === "run" ? 18 : 0,
      stderr: "",
      stdout: JSON.stringify({ tool: "loopexec", recipes: result }),
    };
  },
};
extension(pi);
assert.equal(calls.length, 0, "loading the extension must not execute a child");
assert.equal(commands.size, 4);
const completion = commands.get("jc-recipe").getArgumentCompletions("audio");
assert.equal(completion[0].value, "audio-mastering");
const answers = ["audio/input $(touch injected).wav", "-14"];
const ctx = {
  cwd: pack,
  hasUI: true,
  ui: {
    async input() {
      return answers.shift();
    },
    async select(_title, labels) {
      return labels[0];
    },
    notify(message, type) {
      notices.push({ message, type });
    },
    setStatus() {},
  },
};
await commands.get("jc-recipe").handler("audio-mastering", ctx);
assert.equal(notices.length, 0);
assert.equal(calls.length, 2);
const argv = calls[1].args;
const inputs = JSON.parse(argv[argv.indexOf("--inputs-json") + 1]);
assert.equal(inputs.source, "audio/input $(touch injected).wav");
assert.equal(inputs.target_lufs, -14);
assert.equal(messages.length, 1);
assert.equal(messages[0].display, true);
assert.match(messages[0].content, /adapter_required/);
assert.ok(
  !("sendUserMessage" in pi),
  "the planning UI must not initiate a model task",
);
mode = "empty";
await commands.get("jc-recipes").handler("unrelatedzzz", ctx);
assert.match(messages.at(-1).content, /No matching recipes/);
mode = "drift";
await commands.get("jc-recipe").handler("audio-mastering", ctx);
assert.match(notices.at(-1).message, /catalog differ/);
mode = "normal";
ctx.ui.input = async () => undefined;
await commands.get("jc-recipe").handler("audio-mastering", ctx);
assert.match(notices.at(-1).message, /cancelled/);
const before = calls.length;
await commands.get("jc-recipe-run").handler("audio-mastering", ctx);
assert.equal(calls.length, before, "missing configuration must not run tools");
assert.match(notices.at(-1).message, /Save inputs/);
mkdirSync(resolve(pack, ".jc-recipes"));
writeFileSync(
  resolve(pack, ".jc-recipes/audio-mastering.json"),
  JSON.stringify({ limits: { timeout_seconds: 60 } }),
);
mode = "ready";
await commands.get("jc-recipe-run").handler("audio-mastering", ctx);
assert.equal(calls.at(-1).args[1], "run");
assert.equal(calls.at(-1).options.timeout, 120000);
assert.match(messages.at(-1).content, /budget_exceeded/);
assert.match(messages.at(-1).content, /"acceptance_met": false/);
await commands.get("jc-recipe-verify").handler("saved-run", ctx);
assert.equal(calls.at(-1).args[1], "verify");
assert.equal(
  calls.at(-1).args[calls.at(-1).args.indexOf("--run-id") + 1],
  "saved-run",
);
const count = calls.length;
await commands.get("jc-recipe-verify").handler("../../bad", ctx);
assert.equal(calls.length, count);
console.log(
  "Pi extension behavior passed: discovery, input, argv isolation, cancellation, drift, explicit run, verified guard, read-only verify",
);
