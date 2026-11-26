package recipes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestCatalogPreservesIdeasAndEveryRecipePlans(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Recipes) != 33 || len(c.SourceSocialPriorities) != 5 {
		t.Fatal("source ideas were dropped")
	}
	source, err := os.ReadFile("../../docs/recipe-ideas-source.md")
	if err != nil {
		t.Fatal(err)
	}
	priorities := map[int]string{}
	for i, r := range c.Recipes {
		if r.SourceIndex != i+1 || !strings.Contains(string(source), r.SourceIdea) {
			t.Fatalf("source idea %s missing/reordered", r.ID)
		}
		if r.SocialPriority > 0 {
			priorities[r.SocialPriority] = r.ID
		}
		cfg := Config{SchemaVersion: 1, Recipe: r.ID, Inputs: map[string]json.RawMessage{}}
		for _, in := range r.Inputs {
			if !in.Required {
				continue
			}
			var v any
			switch in.Type {
			case "path":
				v = "fixture/" + in.Name
			case "string":
				v = "configured " + in.Name
				if in.Name == "direction" {
					v = "minimize"
				}
			case "strings":
				v = []string{"criterion-a", "criterion-b"}
			case "number", "integer":
				v = float64(1)
				if in.Minimum != nil {
					v = *in.Minimum
				}
				if in.Maximum != nil && v.(float64) > *in.Maximum {
					v = *in.Maximum
				}
			}
			cfg.Inputs[in.Name], _ = json.Marshal(v)
		}
		p, err := MakePlan(r, cfg)
		if err != nil || len(p.Missing) > 0 || p.ExecutionReady || p.Readiness != "adapter_required" {
			t.Fatalf("%s plan: %+v, %v", r.ID, p, err)
		}
	}
	for i, id := range []string{"metric-gaming", "oscillation", "frontend-visual", "audio-mastering", "cost-budget"} {
		if priorities[i+1] != id {
			t.Fatal("social priority changed")
		}
	}
}
func TestPlanRejectsBadInputsAndKeepsIncompleteHonest(t *testing.T) {
	c, _ := Load()
	r, _ := c.Find("audio-mastering")
	p, err := MakePlan(r, Config{})
	if err != nil || p.ExecutionReady || p.Readiness != "needs_inputs" || len(p.Missing) == 0 {
		t.Fatalf("incomplete plan: %+v %v", p, err)
	}
	for _, tc := range []struct{ name, key, value string }{
		{"wrong-type", "target_lufs", `"loud"`},
		{"negative-tolerance", "lufs_tolerance", "-1"},
		{"escape", "source", `"../input.wav"`},
		{"absolute", "source", `"/input.wav"`},
		{"drive", "source", `"C:\\input.wav"`},
		{"blank", "source", `" "`},
		{"null", "source", "null"},
		{"unknown", "invented_input", "12"},
		{"empty-extra-checks", "additional_constraints", "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := MakePlan(r, Config{Inputs: map[string]json.RawMessage{tc.key: json.RawMessage(tc.value)}})
			if err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
	if _, err := MakePlan(r, Config{Recipe: "another"}); err == nil {
		t.Fatal("accepted wrong recipe")
	}
	if _, err := MakePlan(r, Config{Limits: &Limits{1, 5, 6}}); err == nil {
		t.Fatal("accepted command timeout beyond total")
	}
	if _, err := MakePlan(r, Config{Inputs: map[string]json.RawMessage{"source": []byte(`"same.wav"`), "output": []byte(`"same.wav"`)}}); err == nil {
		t.Fatal("accepted destructive source/output equality")
	}
	p, err = MakePlan(r, Config{Inputs: map[string]json.RawMessage{"additional_constraints": []byte(`["Preserve mono compatibility"]`)}})
	if err != nil || len(p.Checks) != len(r.Checks)+2 || r.Checks[len(r.Checks)-1] == "Preserve mono compatibility" {
		t.Fatal("additional constraints replaced/mutated original acceptance")
	}
}
func TestSuggestionsAreDeterministicAndNoMatchIsEmpty(t *testing.T) {
	c, _ := Load()
	for _, tc := range []struct{ query, want string }{{"make audio louder", "audio-mastering"}, {"agent cheat metric", "metric-gaming"}, {"a11y", "accessibility"}, {"cycle", "oscillation"}} {
		first := c.Suggest(tc.query, 3)
		second := c.Suggest(tc.query, 3)
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(second)
		if len(first) == 0 || first[0].Entry.ID != tc.want || string(a) != string(b) {
			t.Fatalf("%q: %+v", tc.query, first)
		}
	}
	if len(c.Suggest("unrelatedzzzz", 5)) != 0 {
		t.Fatal("invented suggestion")
	}
}
func TestExportIsCompletePortableAndRefusesOverwrite(t *testing.T) {
	c, _ := Load()
	out := filepath.Join(t.TempDir(), "pack")
	result, err := Export(c, out)
	if err != nil || result.Skills != 33 || result.Prompts != 33 {
		t.Fatalf("%+v %v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest ExportManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for name, want := range manifest.Files {
		body, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(body)
		if hex.EncodeToString(h[:]) != want {
			t.Fatal("manifest hash mismatch")
		}
	}
	for _, r := range c.Recipes {
		body, err := os.ReadFile(filepath.Join(out, "skills", "loopexec-"+r.ID, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(string(body), "---", 3)
		if len(parts) != 3 {
			t.Fatal("invalid skill frontmatter")
		}
		var front map[string]any
		if err := yaml.Unmarshal([]byte(parts[1]), &front); err != nil {
			t.Fatal(err)
		}
		if front["name"] != "loopexec-"+r.ID {
			t.Fatal("wrong skill name")
		}
		if _, ok := front["description"].(string); !ok {
			t.Fatal("non-string skill description")
		}
		for _, check := range r.Checks {
			if !strings.Contains(parts[2], asciiText(check)) {
				t.Fatalf("%s lost acceptance", r.ID)
			}
		}
		if _, err := os.Stat(filepath.Join(out, "prompts", "loopexec-"+r.ID+".md")); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.ReadFile(filepath.Join(out, "client.json"))
	if _, err := Export(c, out); err == nil {
		t.Fatal("overwrote existing export")
	}
	after, _ := os.ReadFile(filepath.Join(out, "client.json"))
	if string(before) != string(after) {
		t.Fatal("existing package changed")
	}
	if _, err := exec.LookPath("node"); err == nil {
		cmd := exec.Command("node", "--check", filepath.Join(out, "extensions", "loopexec-recipes.mjs"))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("JS syntax: %v %s", err, output)
		}
	}
}
func TestCatalogDocumentationMatchesSource(t *testing.T) {
	c, _ := Load()
	data, err := os.ReadFile("../../docs/recipe-catalog.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != CatalogMarkdown(c) {
		t.Fatal("recipe guide drift: regenerate with loopexec recipes catalog")
	}
}

func TestPiExtensionBehavior(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node unavailable; Pi extension behavior requires Node")
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Export(c, out); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "pi-extension.test.mjs", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Pi extension behavior: %v: %s", err, output)
	}
}
