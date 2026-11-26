package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/justyn-clark/loopexec/internal/recipeadapter"
	"github.com/justyn-clark/loopexec/internal/recipes"
	"github.com/spf13/cobra"
)

func recipeRaw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func recipeDemoConfig(id string) recipes.Config {
	return recipes.Config{SchemaVersion: 1, Recipe: id, Inputs: map[string]json.RawMessage{}, Limits: &recipes.Limits{MaxIterations: 6, TimeoutSeconds: 180, CommandTimeoutSeconds: 30}, Adapter: &recipes.Adapter{SchemaVersion: 1, Artifacts: []string{"candidate"}, Protected: []string{"references"}, Tools: map[string]string{}, Options: map[string]json.RawMessage{}}}
}
func prepareLiveRecipeDemo(ctx context.Context, id, dir, toolsFile string) (recipes.Config, error) {
	cfg := recipeDemoConfig(id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return cfg, fmt.Errorf("demo workspace must be new")
	}
	for _, p := range []string{"candidate", "references"} {
		if err := os.Mkdir(filepath.Join(dir, p), 0700); err != nil {
			return cfg, err
		}
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return cfg, err
	}
	if toolsFile != "" {
		b, err := os.ReadFile(toolsFile)
		if err != nil {
			return cfg, err
		}
		if err = json.Unmarshal(b, &cfg.Adapter.Tools); err != nil {
			return cfg, err
		}
	}
	put := func(name, body string) error { return os.WriteFile(filepath.Join(root, name), []byte(body), 0600) }
	set := func(key string, value any) { cfg.Inputs[key] = recipeRaw(value) }
	option := func(key string, value any) { cfg.Adapter.Options[key] = recipeRaw(value) }
	self, err := os.Executable()
	if err != nil {
		return cfg, err
	}
	cfg.Adapter.Work = []string{self, "recipes", "_demo-work", id, "{context}"}
	switch id {
	case "metric-gaming":
		if err := os.Mkdir(filepath.Join(root, "candidate", "tests"), 0700); err != nil {
			return cfg, err
		}
		if err := put("candidate/values.json", `{"a":1,"b":0}`); err != nil {
			return cfg, err
		}
		if err := put("candidate/tests/test-a.json", `{"id":"test-a","key":"a","equals":1}`); err != nil {
			return cfg, err
		}
		if err := put("candidate/tests/test-b.json", `{"id":"test-b","key":"b","equals":1}`); err != nil {
			return cfg, err
		}
		set("fixture", "candidate")
		set("measured_ids", []string{"test-a", "test-b"})
		set("protected_paths", []string{"references"})
		cfg.Adapter.Inventory = []string{"candidate/tests"}
		option("suite", map[string]string{"cases": "candidate/tests", "values": "candidate/values.json"})
	case "oscillation":
		if err := put("candidate/state.json", `{"value":0}`); err != nil {
			return cfg, err
		}
		set("fixture", "candidate")
		set("states", []string{"A", "B"})
		set("failure_ids", []string{"must-be-zero", "must-be-one"})
		option("state_file", "candidate/state.json")
		option("conditions", []map[string]any{{"id": "must-be-zero", "key": "value", "equals": 0}, {"id": "must-be-one", "key": "value", "equals": 1}})
	case "regression":
		if err := put("candidate/state.json", `{"a":false,"b":false,"c":true}`); err != nil {
			return cfg, err
		}
		set("fixture", "candidate")
		set("conditions", []string{"a", "b", "c"})
		set("allowed_paths", []string{"candidate/state.json"})
		option("state_file", "candidate/state.json")
		option("conditions", []map[string]any{{"id": "a", "key": "a", "equals": true}, {"id": "b", "key": "b", "equals": true}, {"id": "c", "key": "c", "equals": true}})
	case "cost-budget":
		if err := put("candidate/launches.json", "[]"); err != nil {
			return cfg, err
		}
		set("objective", "Complete the task with two budgeted child invocations; the third must not start")
		set("scope_id", "jc-budget-demo")
		set("max_calls", 2)
		set("phases", []string{"builder"})
		option("completion_marker", "candidate/completed.marker")
		option("launch_log", "candidate/launches.json")
	case "frontend-visual":
		good := "<!doctype html><html lang=\"en\"><meta charset=\"utf-8\"><title>JC frontend repair</title><style>body{margin:0;background:#101827;color:#e5edf8;font:24px Arial}main{margin:80px;width:640px;padding:40px;background:#172740;border:2px solid #38bdf8;border-radius:20px}h1{font-size:40px;color:#38bdf8}button{background:#38bdf8;color:#101827;border:0;border-radius:8px;padding:16px;font-size:20px}</style><main><h1>JC Loop Recipes</h1><p>Independent checks decide when this is ready.</p><button>Run a bounded task</button></main></html>"
		bad := strings.ReplaceAll(strings.ReplaceAll(good, "width:640px", "width:1160px"), "color:#38bdf8", "color:#ff3333")
		if err := put("candidate/index.html", bad); err != nil {
			return cfg, err
		}
		if err := put("references/index.html", good); err != nil {
			return cfg, err
		}
		set("project", "candidate")
		set("page_url", "file://"+filepath.ToSlash(filepath.Join(root, "candidate", "index.html")))
		set("reference", "references/approved.png")
		set("max_visual_difference", 0.001)
		option("assertions", []map[string]any{{"id": "card-width", "selector": "main", "property": "rect.width", "max": 740}, {"id": "button", "selector": "button", "property": "visible", "equals": true}})
		capture := recipeadapter.Context{Root: root, EvidenceDir: filepath.Join(root, "reference-capture"), Recipe: "frontend-capture", Inputs: map[string]any{"page_url": "file://" + filepath.ToSlash(filepath.Join(root, "references", "index.html"))}, Adapter: *cfg.Adapter, TimeoutSeconds: 30}
		if err := os.Mkdir(capture.EvidenceDir, 0700); err != nil {
			return cfg, err
		}
		capture.Adapter.Options = map[string]json.RawMessage{}
		if _, err := recipeadapter.BrowserCapture(ctx, capture); err != nil {
			return cfg, err
		}
		data, err := os.ReadFile(filepath.Join(capture.EvidenceDir, "page-000.png"))
		if err != nil {
			return cfg, err
		}
		if err = os.WriteFile(filepath.Join(root, "references", "approved.png"), data, 0600); err != nil {
			return cfg, err
		}
	case "audio-mastering":
		cfg.Adapter.Work = nil
		set("source", "references/original.wav")
		set("output", "candidate/master.wav")
		set("target_lufs", -16)
		set("lufs_tolerance", 0.4)
		set("max_true_peak_dbtp", -1)
		set("min_dynamic_range", 0)
		c := recipeadapter.Context{Root: root, EvidenceDir: root, Adapter: *cfg.Adapter, TimeoutSeconds: 30}
		r, err := c.Run(ctx, root, c.Tool("ffmpeg"), "-hide_banner", "-nostats", "-y", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=12", "-af", "volume=0.015", "-c:a", "pcm_s16le", filepath.Join(root, "references", "original.wav"))
		if err != nil || r.ExitCode != 0 {
			return cfg, fmt.Errorf("unable to generate real audio fixture")
		}
		data, err := os.ReadFile(filepath.Join(root, "references", "original.wav"))
		if err != nil {
			return cfg, err
		}
		if err = os.WriteFile(filepath.Join(root, "candidate", "master.wav"), data, 0600); err != nil {
			return cfg, err
		}
	default:
		return prepareAdditionalRecipeDemo(ctx, id, root, cfg)
	}
	if err := recipeWriteExclusive(filepath.Join(root, "recipe.json"), cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
func runLiveRecipeDemo(cmd *cobra.Command, id, dir, tools string) error {
	if dir == "" {
		return failResponse(cmd, "", 30, "workspace_invalid", "demo requires --workdir")
	}
	cfg, err := prepareLiveRecipeDemo(cmd.Context(), id, dir, tools)
	if err != nil {
		return failResponse(cmd, "", 30, "workspace_invalid", err.Error())
	}
	if id == "api-contract" {
		url, closeServer, e := recipeDemoServer(dir)
		if e != nil {
			return failResponse(cmd, "", 30, "workspace_invalid", e.Error())
		}
		defer closeServer()
		cfg.Inputs["base_url"] = recipeRaw(url)
	}
	s, file, err := recipeInitialize(cmd.Context(), dir, "jc-"+id, cfg)
	if err != nil {
		return failResponse(cmd, "", 30, "workspace_invalid", err.Error())
	}
	state, err := recipeExecute(cmd, s, file)
	if err != nil {
		return failResponse(cmd, s.RunID, 40, "execution_failure", err.Error())
	}
	if err = recipeSealRun(s); err != nil {
		return failResponse(cmd, s.RunID, 40, "execution_failure", err.Error())
	}
	verification, err := recipeVerify(cmd.Context(), s)
	if err != nil {
		return failResponse(cmd, s.RunID, 13, "objective_unverified", err.Error())
	}
	if verification.BaselinePassed {
		return failResponse(cmd, s.RunID, 13, "objective_unverified", "demo baseline unexpectedly passed")
	}
	if verification.HaltReason != verification.ExpectedHalt {
		return failResponse(cmd, s.RunID, 13, "objective_unverified", "demo did not reach its expected computed halt")
	}
	verified := verification.ReceiptVerified
	return printResponse(cmd, response{Tool: toolName, Version: toolVersion, Status: "verified", RunID: s.RunID, Iteration: state.Iteration, HaltReason: state.HaltReason, CheckExit: state.LastCheckExit, Verified: &verified, Receipt: filepath.Join(dir, ".loopexec", "run-"+s.RunID+".jsonl"), Recipes: &recipes.Result{CatalogSHA256: recipes.Digest(), Verification: &verification}, Errors: []string{}})
}
func newRecipeDemoWorkCmd() *cobra.Command {
	return &cobra.Command{Use: "_demo-work <id> <context>", Hidden: true, Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		var c recipeadapter.Context
		if err = json.Unmarshal(data, &c); err != nil {
			return err
		}
		write := func(path string, v any) error {
			b, _ := json.Marshal(v)
			return os.WriteFile(filepath.Join(c.Root, path), b, 0600)
		}
		switch args[0] {
		case "metric-gaming":
			return os.Remove(filepath.Join(c.Root, "candidate", "tests", "test-b.json"))
		case "oscillation":
			return write("candidate/state.json", map[string]any{"value": c.Iteration % 2})
		case "regression":
			states := []map[string]bool{{"a": false, "b": false, "c": true}, {"a": true, "b": false, "c": true}, {"a": false, "b": true, "c": false}}
			return write("candidate/state.json", states[min(c.Iteration-1, 2)])
		case "cost-budget":
			p := filepath.Join(c.Root, "candidate", "launches.json")
			var launches []int
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(b, &launches); err != nil {
				return err
			}
			launches = append(launches, c.Iteration)
			return write("candidate/launches.json", launches)
		case "frontend-visual":
			source, err := os.Open(filepath.Join(c.Root, "references", "index.html"))
			if err != nil {
				return err
			}
			defer source.Close()
			out, err := os.OpenFile(filepath.Join(c.Root, "candidate", "index.html"), os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, source)
			closeErr := out.Close()
			if err == nil {
				err = closeErr
			}
			return err
		}
		return additionalRecipeDemoWork(cmd.Context(), args[0], c)
	}}
}
