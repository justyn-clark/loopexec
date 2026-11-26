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
	"unicode/utf16"
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
	fmt.Fprintf(&b, "---\nname: loopexec-%s\ndescription: %s\nmetadata:\n  author: Justyn Clark\n  recipe-id: %s\n  catalog-sha256: %s\n---\n\n# %s\n\n", r.ID, quote("Use when asked to "+strings.TrimSuffix(strings.ToLower(r.Attempt), ".")+"."), r.ID, Digest(), r.Title)
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
	return asciiText(b.String())
}
func quote(s string) string { b, _ := json.Marshal(asciiText(s)); return string(b) }
func CatalogMarkdown(c Catalog) string {
	var b strings.Builder
	b.WriteString("# LoopExec recipe catalog\n\nGenerated from internal/recipes/catalog.json with loopexec recipes catalog. Edit the catalog and regenerate this guide. All 33 original ideas remain distinct. Inputs are configurable; required inputs have no invented domain defaults. All 33 have live execution adapters and real tool fixtures. Tool installations and trusted oracle bindings are explicit prerequisites; see live-recipes.md.\n\n")
	for _, r := range c.Recipes {
		fmt.Fprintf(&b, "## %d. %s\n\nID: %s | category: %s | adapter: %s | expected halt: %s\n\n%s\n\n", r.SourceIndex, r.Title, r.ID, r.Category, r.AdapterStatus, r.ExpectedHalt, r.Attempt)
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
	return asciiText(strings.TrimRight(b.String(), "\n") + "\n")
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
		"catalog.json":                    append(asciiJSON(exportedCatalog), '\n'),
		"extensions/loopexec-recipes.mjs": piExtension,
		"client.json":                     []byte("{\n  \"command\": \"loopexec\"\n}\n"),
	}
	packageInfo := map[string]any{"name": "@justynclark/loopexec-recipes", "version": options.PackageVersion, "private": !options.Public, "type": "module", "license": "MIT", "author": "Justyn Clark", "description": "33 configurable recipe skills and Pi prompts with LoopExec discovery, planning, bounded execution and receipt verification", "keywords": []string{"pi-package", "agent-skills", "loopexec", "loopexec-recipes"}, "engines": map[string]string{"node": ">=20"}, "homepage": "https://loopexec.dev/recipes/", "repository": map[string]string{"type": "git", "url": "https://github.com/justyn-clark/loopexec.git"}, "files": []string{"catalog.json", "client.json", "extensions", "skills", "prompts", "manifest.json", "README.md", "LICENSE"}, "pi": map[string]any{"extensions": []string{"extensions/loopexec-recipes.mjs"}, "skills": []string{"skills"}, "prompts": []string{"prompts/*.md"}}}
	metadata, err := json.MarshalIndent(packageInfo, "", "  ")
	if err != nil {
		return result, err
	}
	files["package.json"] = append(metadata, '\n')
	files["LICENSE"] = []byte(packageLicense)
	files["README.md"] = []byte(packReadme(options))

	for _, r := range c.Recipes {
		files["skills/loopexec-"+r.ID+"/SKILL.md"] = []byte(Skill(r))
		prompt := fmt.Sprintf("---\ndescription: %s\nargument-hint: \"[task inputs]\"\n---\nApply the loopexec-%s skill to this request. Read its SKILL.md before work, resolve the declared inputs, preserve independent acceptance checks and pin finite bounds. User-supplied request data follows:\n\n$ARGUMENTS\n", quote("LoopExec: "+r.Title), r.ID)
		files["prompts/loopexec-"+r.ID+".md"] = []byte(prompt)
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

// asciiText projects generated prose only. Original source strings remain intact.
func asciiText(s string) string {
	return strings.NewReplacer(
		"\u2014", " - ", "\u2013", "-", "\u2192", "->", "\u201c", "\"", "\u201d", "\"",
		"\u2018", "'", "\u2019", "'", "\u00b1", "+/-", "\u2026", "...", "\u00b7", "|",
	).Replace(s)
}

// asciiJSON preserves parsed source strings while keeping distribution bytes ASCII.
func asciiJSON(b []byte) []byte {
	var out strings.Builder
	for _, r := range string(b) {
		if r < 128 {
			out.WriteRune(r)
		} else if r <= 0xffff {
			fmt.Fprintf(&out, "\\u%04x", r)
		} else {
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&out, "\\u%04x\\u%04x", hi, lo)
		}
	}
	return []byte(out.String())
}

func packReadme(options ExportOptions) string {
	install := "Exported development pack. Install this extracted directory with pi install ./path-to-export."
	if options.Public {
		install = fmt.Sprintf("Install the matching CLI v%s from https://github.com/justyn-clark/loopexec/releases/tag/v%s or with:\n\n    go install github.com/justyn-clark/loopexec/cmd/loopexec@v%s\n\nThen install:\n\n    pi install npm:@justynclark/loopexec-recipes@%s\n\nFor a downloaded tarball, extract justynclark-loopexec-recipes-%s.tgz and run pi install ./package.", options.RuntimeVersion, options.RuntimeVersion, options.RuntimeVersion, options.PackageVersion, options.PackageVersion)
	}
	return "# LoopExec for Pi\n\n33 portable skills, 33 prompt shortcuts and one discovery/planning/run/verify extension from LoopExec Recipes. Pi is optional; compatible agents can use skills/loopexec-*/SKILL.md directly.\n\n" + install + "\n\nKeep the matching loopexec on PATH or set client.json command to one executable. Reload Pi with /reload.\n\nUse /loopexec-recipes to browse or suggest tasks; /loopexec-recipe <id> collects inputs and previews a plan. Neither starts work or a model. /loopexec-recipe-run <id> explicitly runs a saved config; /loopexec-recipe-verify <run-id> verifies its receipt. /loopexec-<id> invokes a skill under the host agent's normal authorization.\n\nSave schema_version:1, recipe:<id>, inputs, adapter and finite limits in .loopexec-recipes/<id>.json. Existing .jc-recipes/<id>.json configs remain readable. If both exist, the new path wins; invalid new configs fail instead of falling back. Credentials do not belong in configs.\n\nMigrating from @justynclark/jc-loop-recipes@0.1.0: install the matching CLI first, remove the old Pi package registration with pi remove npm:@justynclark/jc-loop-recipes@0.1.0, then install this package. Do not load both packs together. Replace /jc- command and skill prefixes with /loopexec-. Existing configs need no rename. Old package versions and receipts stay intact; verify historical runs using their matching CLI.\n\nReal runs require explicit domain tools, a configured builder and independent verifier. Discovery/loading never launches a model. No automatic budget renewal, paid launch, deployment or publication is included. Standalone instructions alone cannot enforce bounds or produce LoopExec receipts.\n\nGuides: https://loopexec.dev/recipes/ and https://loopexec.dev/live-recipes/ . All 33 original ideas remain distinct in catalog.json. MIT, Justyn Clark.\n"
}
