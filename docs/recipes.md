# LoopExec Recipes

LoopExec Recipes preserves all 33 originating ideas as one configurable catalog.
LoopExec supplies bounded execution, Pi supplies discovery/input collection and
explicit run/verify commands, and generated loopexec-\* skills carry the same task
methods to other agents.

The [complete catalog](recipe-catalog.md) keeps every original idea, its order,
typed inputs, independent checks, protected surface and evidence. The
[original discussion](recipe-ideas-source.md) is preserved verbatim. The shared
machine source is internal/recipes/catalog.json.

## Use it

```sh
go build -o build/loopexec ./cmd/loopexec
build/loopexec recipes list
build/loopexec recipes suggest "normalize a music master"
build/loopexec recipes show audio-mastering
build/loopexec recipes plan audio-mastering --config recipe.json
build/loopexec recipes run audio-mastering --config recipe.json --workdir work --run-id master-1 --json
build/loopexec recipes verify --workdir work --run-id master-1 --json
```

All 33 recipes have real tool fixtures and configurable adapters. See
[live execution](live-recipes.md) for tool requirements, config examples,
oracle reports, budgets, candidate retention, receipts and the all-recipe proof
command. This is v0.5.0-rc.4 candidate source; published older executables do not gain
these commands automatically.

A recipe specifies the task and acceptance contract. An adapter binds one work
invocation and a trusted domain verifier. The governor owns retries, finite
time/attempt/call limits, progress guards and retention. A generated skill
provides instructions; it cannot enforce these bounds without a runtime.

<a id="pi-and-portable-skills"></a>

## LoopExec for Pi

```sh
build/loopexec recipes export --out build/loopexec-recipes
pi install ./build/loopexec-recipes
```

Use a fresh export directory. The generated pack contains 33 skills,
33 prompt shortcuts, one extension, the catalog and a hash manifest. Set its
client.json command to the matching executable or keep it on PATH. Then /reload.

- /loopexec-recipes browses the catalog or suggests explained keyword matches.
- /loopexec-recipe <id> collects missing typed inputs and previews a plan.
- /loopexec-recipe-run <id> explicitly runs .loopexec-recipes/<id>.json and displays its verified outcome.
- /loopexec-recipe-verify <run-id> verifies saved evidence without another builder/reviewer call.
- /loopexec-<id> invokes the portable skill through the host's normal agent-task authorization.

Loading, browsing and planning do not start a model or workflow. Running requires
a saved config with inputs, an adapter and finite limits. Catalog digest checks
prevent mixing a pack and an executable with different recipe definitions.
Values travel as separate argv/JSON data. Global Pi settings are never modified
by export. Keep credentials and private tool paths out of published packs.

Standalone compatible agents can use the generated SKILL.md methods with their
own bounded workflow; they must not claim a LoopExec receipt for manual work.

## Configuration and suggestions

Config precedence is catalog defaults, config inputs, inputs-json, then repeated
set overrides. Unknown keys, invalid types, unsafe relative paths and invalid
bounds fail validation. A plan reports missing inputs and adapter readiness
without executing tools. Domain thresholds remain explicit user choices.

Suggestions use deterministic keyword/tag matches, with matched terms visible.
For natural-language ranking, a host agent can select among the same catalog
entries. Per-project .loopexec-recipes configs reuse tools, inputs and limits without
changing the authored recipe identity.

## Ownership and delivery

Maintain one catalog and regenerate the catalog guide and portable exports.
Keep domain policy in adapters; LoopExec remains a general bounded loop governor.
The five social priorities remain metric gaming, oscillation, frontend repair,
audio/mastering and a call-budget halt. A guard demonstration can be verified
while task acceptance is false.

No provider calls, production changes or publication are authorized merely by
selecting a recipe. Public release still requires the repository's source/docs,
privacy and deployment gates. Local fixture proof is a development boundary.

## Shareable distribution

Generate a versioned public pack with `loopexec recipes export --public --package-version 0.1.0 --out build/loopexec-recipes-public`. This writes distribution metadata and an exact file manifest; it does not publish or install anything. The release ships the npm-compatible tarball alongside the core archives, and the same pack is published as `@justynclark/loopexec-recipes@0.1.0` when registry authentication permits. Install with `pi install npm:@justynclark/loopexec-recipes@0.1.0` or extracting the downloaded archive with `tar -xzf justynclark-loopexec-recipes-0.1.0.tgz` and running `pi install ./package`. Standalone agents can use the extracted skills directories. Keep the matching v0.5.0-rc.4 CLI on PATH.

## Migration from the original Pi pack

The package is now @justynclark/loopexec-recipes@0.1.0, displayed as LoopExec for Pi. The catalog is LoopExec Recipes. The new package name starts its own version history; the old @justynclark/jc-loop-recipes@0.1.0 stays installable with unchanged bytes. A registry deprecation notice will point to the replacement after public download and installation are verified.

Install the matching v0.5.0-rc.4 CLI, then run:

```sh
pi remove npm:@justynclark/jc-loop-recipes@0.1.0
pi install npm:@justynclark/loopexec-recipes@0.1.0
```

Reload Pi. Do not load both packs together. Commands and skills now use the loopexec- prefix. Existing .jc-recipes/<id>.json configs remain readable; .loopexec-recipes/<id>.json takes precedence if both exist. An invalid new config fails instead of falling back. No configs, runs or receipts are rewritten. Historical runs retain their matching CLI and original proof.
