package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/justyn-clark/loopexec/internal/recipes"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"github.com/spf13/cobra"
)

func newRecipesCmd() *cobra.Command {
	root := &cobra.Command{Use: "recipes", Short: "Discover JC recipes, validate task inputs and export portable skills/Pi resources"}
	load := func() (recipes.Catalog, error) { return recipes.Load() }
	emit := func(cmd *cobra.Command, status string, result recipes.Result, human string) error {
		result.CatalogSHA256 = recipes.Digest()
		if jsonOutput {
			return printResponse(cmd, response{Tool: toolName, Version: toolVersion, Status: status, Recipes: &result, Errors: []string{}})
		}
		_, err := fmt.Fprint(cmd.OutOrStdout(), human)
		return err
	}
	bad := func(cmd *cobra.Command, err error) error {
		return failResponse(cmd, "", exitWorkspaceInvalid, "workspace_invalid", err.Error())
	}
	var category string
	list := &cobra.Command{Use: "list", Short: "List the catalog without running tools or models", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := load()
		if err != nil {
			return bad(cmd, err)
		}
		entries := c.List(category)
		var b strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&b, "%-25s %-15s %s [%s]\n", e.ID, e.Category, e.Title, e.AdapterStatus)
		}
		return emit(cmd, "ok", recipes.Result{Entries: entries}, b.String())
	}}
	list.Flags().StringVar(&category, "category", "", "Filter to an exact catalog category")
	show := &cobra.Command{Use: "show <id>", Short: "Show one recipe's inputs, checks, evidence and original idea", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := load()
		if err != nil {
			return bad(cmd, err)
		}
		r, err := c.Find(args[0])
		if err != nil {
			return bad(cmd, err)
		}
		return emit(cmd, "ok", recipes.Result{Definition: &r}, recipes.Skill(r)+"\nOriginal idea:\n"+r.SourceIdea+"\n")
	}}
	var limit int
	suggest := &cobra.Command{Use: "suggest <task words>", Short: "Rank keyword/tag matches deterministically without a model", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if limit < 1 || limit > 33 {
			return bad(cmd, fmt.Errorf("limit must be between 1 and 33"))
		}
		c, err := load()
		if err != nil {
			return bad(cmd, err)
		}
		matches := c.Suggest(strings.Join(args, " "), limit)
		var b strings.Builder
		for _, s := range matches {
			fmt.Fprintf(&b, "%s — %s (matched: %s)\n", s.Entry.ID, s.Entry.Title, strings.Join(s.Matched, ", "))
		}
		if len(matches) == 0 {
			b.WriteString("No keyword matches; use recipes list to browse all recipes.\n")
		}
		return emit(cmd, "ok", recipes.Result{Suggestions: matches}, b.String())
	}}
	suggest.Flags().IntVar(&limit, "limit", 5, "Maximum suggestions (1..33)")
	var configFile, inputsJSON string
	var sets []string
	plan := &cobra.Command{Use: "plan <id>", Short: "Validate configurable inputs and preview a contract; executes nothing", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := load()
		if err != nil {
			return bad(cmd, err)
		}
		r, err := c.Find(args[0])
		if err != nil {
			return bad(cmd, err)
		}
		cfg := recipes.Config{SchemaVersion: 1, Recipe: r.ID, Inputs: map[string]json.RawMessage{}}
		if configFile != "" {
			if err := workflow.Read(configFile, &cfg); err != nil {
				return bad(cmd, fmt.Errorf("invalid recipe config: %w", err))
			}
			if cfg.SchemaVersion != 1 {
				return bad(cmd, fmt.Errorf("config schema_version must be 1"))
			}
		}
		if cfg.Inputs == nil {
			cfg.Inputs = map[string]json.RawMessage{}
		}
		if inputsJSON != "" {
			var overrides map[string]json.RawMessage
			if err := workflow.Decode([]byte(inputsJSON), &overrides); err != nil || overrides == nil {
				return bad(cmd, fmt.Errorf("inputs-json must be one JSON object with unique keys"))
			}
			for k, v := range overrides {
				cfg.Inputs[k] = v
			}
		}
		seen := map[string]bool{}
		for _, set := range sets {
			k, v, ok := strings.Cut(set, "=")
			if !ok || k == "" || seen[k] {
				return bad(cmd, fmt.Errorf("each --set must be a unique name=value"))
			}
			seen[k] = true
			raw := json.RawMessage(v)
			if !json.Valid(raw) {
				raw, _ = json.Marshal(v)
			}
			cfg.Inputs[k] = raw
		}
		p, err := recipes.MakePlan(r, cfg)
		if err != nil {
			return bad(cmd, err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "JC %s: %s\nInputs: %s\nBounds: %d attempts, %ds total, %ds per command\n", r.ID, p.Readiness, formatRecipeInputs(p.Inputs), p.Limits.MaxIterations, p.Limits.TimeoutSeconds, p.Limits.CommandTimeoutSeconds)
		if len(p.Missing) > 0 {
			fmt.Fprintf(&b, "Missing inputs: %s\n", strings.Join(p.Missing, ", "))
		}
		fmt.Fprintf(&b, "One attempt: %s\nChecks:\n", p.Attempt)
		for _, s := range p.Checks {
			fmt.Fprintf(&b, "  - %s\n", s)
		}
		fmt.Fprintf(&b, "Execution ready: %t. Planning ran no hooks or models.\n", p.ExecutionReady)
		return emit(cmd, "planned", recipes.Result{Plan: &p}, b.String())
	}}
	plan.Flags().StringVar(&configFile, "config", "", "Recipe config JSON; relative paths resolve from the current directory")
	plan.Flags().StringVar(&inputsJSON, "inputs-json", "", "JSON object overriding recipe input values; data only")
	plan.Flags().StringArrayVar(&sets, "set", nil, "Input override name=value; arrays/numbers use JSON, other text becomes a string (repeatable)")
	var out, packageVersion string
	var public bool
	export := &cobra.Command{Use: "export", Short: "Write 33 skills, 33 prompts and a Pi discovery/run/verify extension to a fresh local directory", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if out == "" {
			return bad(cmd, fmt.Errorf("export requires --out"))
		}
		c, err := load()
		if err != nil {
			return bad(cmd, err)
		}
		result, err := recipes.ExportWithOptions(c, out, recipes.ExportOptions{PackageVersion: packageVersion, Public: public, RuntimeVersion: toolVersion})
		if err != nil {
			return bad(cmd, err)
		}
		return emit(cmd, "exported", recipes.Result{Export: &result}, fmt.Sprintf("Exported %d JC skills, %d Pi prompts and the Pi extension to %s\n", result.Skills, result.Prompts, result.Path))
	}}
	export.Flags().BoolVar(&public, "public", false, "Generate a public distributable pack; does not publish it")
	export.Flags().StringVar(&packageVersion, "package-version", "0.1.0-dev", "Package version for the generated Pi/skills pack")
	export.Flags().StringVar(&out, "out", "", "New directory to create; parent must already exist and existing targets are refused")
	catalog := &cobra.Command{Use: "catalog", Short: "Render the complete maintained catalog as Markdown (or JSON)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := load()
		if err != nil {
			return bad(cmd, err)
		}
		return emit(cmd, "ok", recipes.Result{Catalog: &c}, recipes.CatalogMarkdown(c))
	}}
	root.AddCommand(list, show, suggest, plan, export, catalog, newRecipeDemoCmd(), newRecipesRunCmd(), newRecipesVerifyCmd(), newRecipeHookCmd(), newRecipeDemoWorkCmd())
	return root
}
func formatRecipeInputs(v map[string]any) string { b, _ := json.Marshal(v); return string(b) }

// The fixture repairs actual Go source against a frozen Go test, without a model.
func newRecipeDemoCmd() *cobra.Command {
	var dir, tools string
	cmd := &cobra.Command{Use: "demo <id>", Short: "Run a real domain fixture in a fresh workspace and verify its computed outcome", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, e := recipes.Load()
		if e != nil {
			return e
		}
		if _, e = c.Find(args[0]); e != nil {
			return failResponse(cmd, "", 30, "workspace_invalid", e.Error())
		}
		return runLiveRecipeDemo(cmd, args[0], dir, tools)
	}}
	cmd.Flags().StringVar(&dir, "workdir", "", "Fresh directory with an existing parent; fixture files and receipts are retained")
	cmd.Flags().StringVar(&tools, "tools", "", "Private JSON mappings for installed domain tools")
	return cmd
}
func prepareRecipeDemo(dir string) error {
	if err := os.Mkdir(dir, 0755); err != nil {
		return fmt.Errorf("code-repair demo requires a fresh directory: %w", err)
	}
	files := map[string]string{
		"go.mod":       "module jc-code-repair-demo\n\ngo 1.20\n",
		"sum.go":       "package demo\n\nfunc Sum(a, b int) int { return a - b }\n",
		"fixed.go.txt": "package demo\n\nfunc Sum(a, b int) int { return a + b }\n",
		"sum_test.go":  "package demo\n\nimport \"testing\"\n\nfunc TestSum(t *testing.T) { if got := Sum(2, 3); got != 5 { t.Fatalf(\"Sum(2,3) = %d; want 5\", got) } }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			return err
		}
	}
	return nil
}
