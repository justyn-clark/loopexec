// No execution on load. Discovery/planning are read-only; run is an explicit command.
import { readFileSync, lstatSync } from "node:fs";
import { join } from "node:path";

const catalog = JSON.parse(
  readFileSync(new URL("../catalog.json", import.meta.url), "utf8"),
);
const client = JSON.parse(
  readFileSync(new URL("../client.json", import.meta.url), "utf8"),
);

export default function loopexecRecipes(pi) {
  function display(title, content) {
    pi.sendMessage({
      customType: "loopexec-recipe-plan",
      content: title + "\n\n" + content,
      display: true,
    });
  }
  async function call(args, cwd, timeout = 15000, governed = false) {
    if (
      typeof client.command !== "string" ||
      !client.command.trim() ||
      /[\r\n\0]/.test(client.command)
    ) {
      throw new Error(
        "client.json requires one executable command; arguments are passed separately",
      );
    }
    const result = await pi.exec(
      client.command,
      ["recipes", ...args, "--json"],
      { cwd, timeout },
    );
    if (result.code !== 0 && !governed)
      throw new Error(
        "Recipe CLI rejected the request: " +
          (result.stderr || result.stdout).slice(-1000),
      );
    const response = JSON.parse(result.stdout);
    if (
      response.tool !== "loopexec" ||
      !response.recipes ||
      response.recipes.catalog_sha256 !== catalog.catalog_sha256
    ) {
      throw new Error(
        "Pi package and LoopExec catalog differ; export a new package from this binary",
      );
    }
    if (governed && !response.recipes.verification?.receipt_verified)
      throw new Error(
        "Run did not produce a verified receipt: " +
          JSON.stringify(response.errors || response),
      );
    return response.recipes;
  }
  function configExists(cwd, config) {
    // A dangling new-path link must fail, never select an older config.
    return (
      lstatSync(join(cwd, config), { throwIfNoEntry: false }) !== undefined
    );
  }
  function configPath(id, cwd) {
    const current = join(".loopexec-recipes", id + ".json");
    const legacy = join(".jc-recipes", id + ".json");
    return configExists(cwd, current)
      ? current
      : configExists(cwd, legacy)
        ? legacy
        : current;
  }
  async function plan(id, ctx) {
    const recipe = catalog.recipes.find((r) => r.id === id);
    if (!recipe)
      throw new Error("Unknown recipe; browse with /loopexec-recipes");
    const config = configPath(id, ctx.cwd);
    const args = ["plan", id];
    if (configExists(ctx.cwd, config)) args.push("--config", config);
    let result = await call(args, ctx.cwd);
    const inputs = {};
    if (ctx.hasUI) {
      for (const name of result.plan.missing_inputs) {
        const input = recipe.inputs.find((i) => i.name === name);
        const raw = await ctx.ui.input(
          "LoopExec " + recipe.title + ": " + name,
          input.description + " (" + input.type + ")",
        );
        if (raw === undefined) {
          ctx.ui.notify("Recipe planning cancelled", "info");
          return;
        }
        inputs[name] = ["string", "path"].includes(input.type)
          ? raw
          : JSON.parse(raw);
      }
      if (Object.keys(inputs).length)
        result = await call(
          [...args, "--inputs-json", JSON.stringify(inputs)],
          ctx.cwd,
        );
    }
    display(
      recipe.title + " - " + result.plan.readiness,
      JSON.stringify(result.plan, null, 2),
    );
    if (ctx.hasUI)
      ctx.ui.setStatus(
        "loopexec-recipes",
        "LoopExec " + id + ": " + result.plan.readiness,
      );
  }
  pi.registerCommand("loopexec-recipe", {
    description:
      "Collect LoopExec recipe inputs and preview a bounded plan; starts no work",
    getArgumentCompletions(prefix) {
      return catalog.recipes
        .filter((r) => r.id.startsWith(prefix))
        .map((r) => ({ value: r.id, label: r.id + " - " + r.title }));
    },
    async handler(raw, ctx) {
      try {
        await plan(raw.trim(), ctx);
      } catch (error) {
        ctx.ui.notify(String(error.message ?? error), "error");
      }
    },
  });
  pi.registerCommand("loopexec-recipe-run", {
    description:
      "Explicitly execute a saved, configured recipe with finite bounds and verify its receipt",
    async handler(raw, ctx) {
      try {
        const id = raw.trim(),
          recipe = catalog.recipes.find((r) => r.id === id);
        if (!recipe)
          throw Error("Choose one recipe ID; browse with /loopexec-recipes");
        const config = configPath(id, ctx.cwd);
        if (!configExists(ctx.cwd, config))
          throw Error(
            "Save inputs, adapter and limits in " + config + " before running",
          );
        const data = JSON.parse(readFileSync(join(ctx.cwd, config), "utf8"));
        const preview = await call(["plan", id, "--config", config], ctx.cwd);
        if (!preview.plan.execution_ready)
          throw Error("Recipe still needs inputs or an adapter");
        const result = await call(
          ["run", id, "--config", config, "--workdir", ctx.cwd],
          ctx.cwd,
          (data.limits?.timeout_seconds || 600) * 1000 + 60000,
          true,
        );
        display(
          recipe.title + " - " + result.verification.halt_reason,
          JSON.stringify(result.verification, null, 2),
        );
      } catch (error) {
        ctx.ui.notify(String(error.message ?? error), "error");
      }
    },
  });
  pi.registerCommand("loopexec-recipe-verify", {
    description:
      "Verify a saved recipe receipt and candidate without rerunning the builder or reviewer",
    async handler(raw, ctx) {
      try {
        const id = raw.trim();
        if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$/.test(id))
          throw Error("Supply one run ID");
        const result = await call(
          ["verify", "--workdir", ctx.cwd, "--run-id", id],
          ctx.cwd,
        );
        display(
          "Verified recipe receipt",
          JSON.stringify(result.verification, null, 2),
        );
      } catch (error) {
        ctx.ui.notify(String(error.message ?? error), "error");
      }
    },
  });
  pi.registerCommand("loopexec-recipes", {
    description:
      "Browse all 33 LoopExec recipes or suggest matches to a task; starts no work",
    async handler(raw, ctx) {
      try {
        const query = raw.trim();
        const result = await call(
          query ? ["suggest", query] : ["list"],
          ctx.cwd,
        );
        const entries = query
          ? (result.suggestions ?? []).map((s) => s.recipe)
          : (result.entries ?? []);
        if (!entries.length) {
          display(
            "LoopExec recipes",
            "No matching recipes. Use /loopexec-recipes to browse all.",
          );
          return;
        }
        if (!ctx.hasUI) {
          display(
            "LoopExec recipes",
            entries.map((r) => r.id + ": " + r.title).join("\n"),
          );
          return;
        }
        const labels = entries.map((r) => r.id + " - " + r.title);
        const selected = await ctx.ui.select("LoopExec Recipes", labels);
        if (selected !== undefined)
          await plan(entries[labels.indexOf(selected)].id, ctx);
      } catch (error) {
        ctx.ui.notify(String(error.message ?? error), "error");
      }
    },
  });
}
