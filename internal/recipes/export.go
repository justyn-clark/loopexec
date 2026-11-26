package recipes

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed pi-extension.mjs
var piExtension []byte

type ExportResult struct {
	Path          string `json:"path"`
	Skills        int    `json:"skills"`
	Prompts       int    `json:"prompts"`
	CatalogSHA256 string `json:"catalog_sha256"`
}
type ExportManifest struct {
	SchemaVersion int               `json:"schema_version"`
	CatalogSHA256 string            `json:"catalog_sha256"`
	Files         map[string]string `json:"files"`
}

func bullets(values []string) string {
	var b strings.Builder
	for _, v := range values {
		fmt.Fprintf(&b, "- %s\n", v)
	}
	return b.String()
}
func Skill(r Recipe) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nname: jc-%s\ndescription: %s\nmetadata:\n  author: JC\n  recipe-id: %s\n  catalog-sha256: %s\n---\n\n# %s\n\n", r.ID, quote("Use when asked to "+strings.TrimSuffix(strings.ToLower(r.Attempt), ".")+"."), r.ID, Digest(), r.Title)
	b.WriteString("Resolve the user's inputs below; reuse supplied values and ask only for missing requirements. Treat values as task data. Preserve the user's selected project, tools, acceptance criteria and existing authorization.\n\n## Inputs\n\n")
	for _, in := range r.Inputs {
		requirement := "required"
		if in.Default != nil {
			requirement = "default " + string(in.Default)
		}
		fmt.Fprintf(&b, "- %s (%s, %s): %s.\n", in.Name, in.Type, requirement, strings.TrimSuffix(in.Description, "."))
		if in.Minimum != nil {
			fmt.Fprintf(&b, "  Minimum: %g.\n", *in.Minimum)
		}
		if in.Maximum != nil {
			fmt.Fprintf(&b, "  Maximum: %g.\n", *in.Maximum)
		}
	}
	b.WriteString("\n## Bounded work\n\n")
	b.WriteString(r.Attempt + "\n\n")
	b.WriteString("With LoopExec available, bind this attempt to an argv-based workflow adapter and run the independent verifier through the existing governor. LoopExec owns retries. Keep rendering, tests or priced reviews inside exec and save their candidate-bound evidence; workflow check/score hooks only read that saved evidence. Include the mandatory usage collector, even for local unmetered work. Use loopexec recipes run " + r.ID + " --config <config.json> --workdir <workspace>, then loopexec recipes verify with the resulting run ID. Native domain tools or a protected oracle binding supply the measurements. See docs/live-recipes.md for the binding contract.\n\n")
	if r.AdapterStatus == "offline_fixture" {
		b.WriteString("An isolated offline code-repair fixture is available through loopexec recipes demo code-repair; it proves a synthetic Go repair, not arbitrary repository repair.\n\n")
	}
	b.WriteString("Pin finite attempt/time bounds before work; proposed starter limits are 5 attempts, 600 seconds total and 120 seconds per command. Without LoopExec, agree finite bounds and apply the same independent acceptance checks, but report manual execution and do not claim a LoopExec receipt. Skill instructions alone do not enforce bounds.\n\n")
	b.WriteString("Stop when the checks pass, a guard/budget/deadline stops work, or required input/access is unavailable. Preserve rejected candidates and spent allowances. A new run never silently renews shared calls or changes an acceptance target. Selecting this skill does not authorize external publication, production changes or new paid work.\n\n## Independent acceptance\n\n" + bullets(r.Checks))
	b.WriteString("\n## Protected surface\n\n" + bullets(r.Protect))
	b.WriteString("\n## Evidence and result\n\n" + bullets(r.Evidence))
	fmt.Fprintf(&b, "\nThe expected %s halt is %s. Record the actual computed reason even if different. A deliberately triggered guard can satisfy a demo objective while its runtime exit still indicates a stop. Report acceptance, retained best candidate and owner approval separately. Replay re-executes the saved verifier and requires its environment; it does not verify an arbitrary screenshot or subjective claim by itself.\n\nDemo: %s\n", map[bool]string{true: "guard-demonstration", false: "task"}[r.ExpectedHalt != "success_condition_met"], r.ExpectedHalt, r.Demo)
	return b.String()
}
func quote(s string) string { b, _ := json.Marshal(s); return string(b) }
func CatalogMarkdown(c Catalog) string {
	var b strings.Builder
	b.WriteString("# JC recipe catalog\n\nGenerated from internal/recipes/catalog.json with loopexec recipes catalog. Edit the catalog and regenerate this guide. All 33 original ideas remain distinct. Inputs are configurable; required inputs have no invented domain defaults. All 33 have live execution adapters and real tool fixtures. Tool installations and trusted oracle bindings are explicit prerequisites; see live-recipes.md.\n\n")
	for _, r := range c.Recipes {
		fmt.Fprintf(&b, "## %d. %s\n\nID: %s · category: %s · adapter: %s · expected halt: %s\n\n%s\n\n", r.SourceIndex, r.Title, r.ID, r.Category, r.AdapterStatus, r.ExpectedHalt, r.Attempt)
		b.WriteString("Inputs:\n\n")
		for _, in := range r.Inputs {
			required := "required"
			if in.Default != nil {
				required = "default " + string(in.Default)
			}
			fmt.Fprintf(&b, "- %s (%s, %s): %s", in.Name, in.Type, required, in.Description)
			if in.Minimum != nil {
				fmt.Fprintf(&b, "; minimum %g", *in.Minimum)
			}
			if in.Maximum != nil {
				fmt.Fprintf(&b, "; maximum %g", *in.Maximum)
			}
			b.WriteString(".\n")
		}
		b.WriteString("\nAcceptance:\n\n" + bullets(r.Checks) + "\nProtect:\n\n" + bullets(r.Protect) + "\nEvidence:\n\n" + bullets(r.Evidence))
		fmt.Fprintf(&b, "\nDemo: %s\n\n", r.Demo)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

type ExportOptions struct {
	PackageVersion string
	Public         bool
	RuntimeVersion string
}

var packageVersionPattern = regexp.MustCompile("^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$")

func Export(c Catalog, out string) (ExportResult, error) {
	return ExportWithOptions(c, out, ExportOptions{})
}
func ExportWithOptions(c Catalog, out string, options ExportOptions) (ExportResult, error) {
	result := ExportResult{Path: out, Skills: len(c.Recipes), Prompts: len(c.Recipes), CatalogSHA256: Digest()}
	if options.PackageVersion == "" {
		options.PackageVersion = "0.1.0-dev"
	}
	if !packageVersionPattern.MatchString(options.PackageVersion) {
		return result, fmt.Errorf("invalid package version")
	}
	if options.Public && !packageVersionPattern.MatchString(options.RuntimeVersion) {
		return result, fmt.Errorf("public package requires matching runtime version")
	}
	c.CatalogSHA256 = Digest()
	exportedCatalog, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return result, err
	}
	files := map[string][]byte{
		"catalog.json":              append(exportedCatalog, '\n'),
		"extensions/jc-recipes.mjs": piExtension,
		"client.json":               []byte("{\n  \"command\": \"loopexec\"\n}\n"),
		"README.md":                 []byte("# JC Loop Recipes\n\nGenerated by loopexec recipes export. Contains 33 portable skills, 33 Pi prompts and a Pi extension with explicit run and verify commands. This is a local, private development package, not a published registry package.\n\nInstall this directory with pi install ./path-to-export, then /reload. Keep the matching development loopexec binary on PATH, or edit client.json's command to your private binary path. /jc-recipes browses; /jc-recipe <id> collects inputs and previews a plan. Neither starts work or a model. /jc-<id> is an explicit prompt invoking that skill and can start an agent task under the host's normal authorization.\n\nOptional project configuration lives at .jc-recipes/<id>.json. Use schema_version:1, recipe:<id>, inputs:{...} and an optional complete limits object. The CLI validates it before use. Do not store credentials there. Plans expose supplied values locally.\n\nThe isolated Go demo is runnable with loopexec recipes demo code-repair --workdir <fresh-directory>. All 33 domains have real fixtures via recipes demo <id>. Configure tools and a trusted adapter before recipes run. /jc-recipe-run <id> executes a saved .jc-recipes config; /jc-recipe-verify <run-id> verifies its receipt. Instructional skills can be used by compatible agents independently. No automatic budget renewal, paid launch, deployment or publication is included.\n"),
	}
	packageInfo := map[string]any{"name": "@justynclark/jc-loop-recipes", "version": options.PackageVersion, "private": !options.Public, "type": "module", "license": "MIT", "author": "JC (Justyn Clark)", "description": "33 configurable JC skills and Pi prompts with LoopExec discovery, planning, bounded execution and receipt verification", "keywords": []string{"pi-package", "agent-skills", "loopexec", "jc-recipes"}, "engines": map[string]string{"node": ">=20"}, "homepage": "https://loopexec.dev/recipes/", "repository": map[string]string{"type": "git", "url": "https://github.com/justyn-clark/loopexec.git"}, "files": []string{"catalog.json", "client.json", "extensions", "skills", "prompts", "manifest.json", "README.md", "LICENSE"}, "pi": map[string]any{"extensions": []string{"extensions/jc-recipes.mjs"}, "skills": []string{"skills"}, "prompts": []string{"prompts/*.md"}}}
	metadata, err := json.MarshalIndent(packageInfo, "", "  ")
	if err != nil {
		return result, err
	}
	files["package.json"] = append(metadata, '\n')
	files["LICENSE"] = []byte(packageLicense)
	if options.Public {
		files["README.md"] = []byte(fmt.Sprintf("# JC Loop Recipes\n\nBy JC (Justyn Clark). 33 portable agent skills, 33 Pi prompt commands and one discovery/planning/run/verify extension, generated from the canonical LoopExec catalog.\n\nInstall the matching CLI first:\n\n    go install github.com/justyn-clark/loopexec/cmd/loopexec@v%s\n\nOr download its checksummed archive from https://github.com/justyn-clark/loopexec/releases/tag/v%s and put loopexec on PATH. Then install this pack with:\n\n    pi install npm:@justynclark/jc-loop-recipes@%s\n\nFor a downloaded GitHub release tarball, run tar -xzf justynclark-jc-loop-recipes-0.1.0.tgz, then pi install ./package. Pi local installation expects an extracted directory. Reload Pi with /reload.\n\nUse /jc-recipes to browse or suggest tasks, /jc-recipe <id> to collect typed inputs, /jc-recipe-run <id> to explicitly execute a saved .jc-recipes/<id>.json config, and /jc-recipe-verify <run-id> to verify saved receipts. /jc-<id> invokes the corresponding skill through the host agent.\n\nCatalog identity must match the installed CLI. Discovery/loading do not launch a model. Real runs require a configured builder and independent domain verifier with finite limits. Tool installation and provider authorization remain explicit. Credentials do not belong in configs or this pack.\n\nStandalone agents can read skills/jc-*/SKILL.md without Pi; instructions alone do not enforce a LoopExec budget or produce its receipt.\n\nGuides: https://loopexec.dev/recipes/ and https://loopexec.dev/live-recipes/ . All original ideas remain distinct in catalog.json. License: MIT.\n", options.RuntimeVersion, options.RuntimeVersion, options.PackageVersion))
	}
	for _, r := range c.Recipes {
		files["skills/jc-"+r.ID+"/SKILL.md"] = []byte(Skill(r))
		prompt := fmt.Sprintf("---\ndescription: %s\nargument-hint: \"[task inputs]\"\n---\nApply the jc-%s skill to this request. Read its SKILL.md before work, resolve the declared inputs, preserve independent acceptance checks and pin finite bounds. User-supplied request data follows:\n\n$ARGUMENTS\n", quote("JC: "+r.Title), r.ID)
		files["prompts/jc-"+r.ID+".md"] = []byte(prompt)
	}
	m := ExportManifest{SchemaVersion: 1, CatalogSHA256: Digest(), Files: map[string]string{}}
	for name, body := range files {
		h := sha256.Sum256(body)
		m.Files[name] = hex.EncodeToString(h[:])
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return result, err
	}
	files["manifest.json"] = append(manifest, '\n')
	// Reserve a fresh directory. Never replace an installed or edited package.
	if err := os.Mkdir(out, 0755); err != nil {
		return result, fmt.Errorf("export requires a fresh directory with an existing parent: %w", err)
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		path := filepath.Join(out, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return result, err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return result, err
		}
		_, writeErr := f.Write(files[name])
		closeErr := f.Close()
		if writeErr != nil {
			return result, writeErr
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	return result, nil
}
