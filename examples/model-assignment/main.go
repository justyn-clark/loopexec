// This adapter simulates two model roles. It never invokes a model or engine.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justyn-clark/loopexec/internal/modelpolicy"
	"github.com/justyn-clark/loopexec/internal/workflow"
)

type executed struct {
	Role   string                `json:"role"`
	Model  *modelpolicy.Identity `json:"model,omitempty"`
	Status string                `json:"status"`
}

func initialize(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	dir := flags.String("dir", "", "New fixture workspace")
	scenario := flags.String("scenario", "normal", "normal, budget, reserve-drift, usage-drift, technical")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("--dir required")
	}
	if _, err := os.Stat(*dir); !os.IsNotExist(err) {
		return fmt.Errorf("fixture directory must not exist")
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	adapter, err := filepath.Rel(root, self)
	if err != nil {
		return err
	}
	if filepath.Dir(adapter) == "." {
		adapter = "." + string(filepath.Separator) + adapter
	}
	policy := &modelpolicy.Policy{Mode: "explicit", Task: "studio-review", Profiles: map[string]modelpolicy.Profile{
		"routine":       {Identity: modelpolicy.Identity{Provider: "offline", ModelID: "fixture-small", ReasoningEffort: "low"}, Capabilities: []string{"text"}},
		"visual-review": {Identity: modelpolicy.Identity{Provider: "offline", ModelID: "fixture-vision", ReasoningEffort: "high"}, Capabilities: []string{"text", "vision"}},
	}, Tasks: map[string]modelpolicy.Task{"studio-review": {Roles: map[string]modelpolicy.Assignment{
		"builder": {Profile: "routine", Reason: "Simulate a bounded routine preparation task"},
		"critic":  {Profile: "visual-review", Reason: "Simulate independent comparison of equipment captures with references", Requires: []string{"vision"}},
	}, Command: []string{adapter, "attempt", "{builder.model}", "{builder.reasoning}", "{critic.model}", "{critic.reasoning}"}}}}
	w := workflow.Config{SchemaVersion: 1, Models: policy, Check: []string{adapter, "check"}, Meter: workflow.MeterPolicy{Mode: "unmetered", Reserve: []string{adapter, "reserve"}, Usage: []string{adapter, "usage"}, MaxCalls: 2, MaxTokens: 20}}
	if *scenario == "budget" {
		w.Meter.MaxCalls = 1
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if err := workflow.Atomic(filepath.Join(root, "workflow.json"), w); err != nil {
		return err
	}
	return workflow.Atomic(filepath.Join(root, "scenario.json"), *scenario)
}

func execute(args []string) (int, error) {
	if len(args) == 0 {
		return 2, fmt.Errorf("command required")
	}
	if args[0] == "init" {
		return 0, initialize(args[1:])
	}
	if len(args) < 2 {
		return 2, fmt.Errorf("request required")
	}
	var req workflow.Request
	if err := workflow.Read(args[len(args)-1], &req); err != nil || req.Models == nil || len(req.Models.Roles) != 2 {
		return 2, fmt.Errorf("invalid fixture request")
	}
	var scenario string
	if err := workflow.Read("scenario.json", &scenario); err != nil {
		return 2, err
	}
	output := json.NewEncoder(os.Stdout)
	switch args[0] {
	case "reserve":
		calls := []workflow.Reservation{}
		for _, role := range req.Models.Roles {
			model := role.Identity
			if scenario == "reserve-drift" && role.Role == "critic" {
				model.ModelID = "wrong-fixture-model"
			}
			calls = append(calls, workflow.Reservation{ID: workflow.CallID(req.RunID, req.Iteration, req.Phase, role.Role), Phase: role.Role, Model: &model, MaxTokens: 10})
		}
		return 0, output.Encode(workflow.Preflight{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Calls: calls})
	case "attempt":
		if len(args) != 6 {
			return 2, fmt.Errorf("expected two model/effort pairs and request")
		}
		calls := []executed{}
		for i, role := range req.Models.Roles {
			model := role.Identity
			model.ModelID, model.ReasoningEffort = args[1+2*i], args[2+2*i]
			call := executed{Role: role.Role, Model: &model, Status: "unknown"}
			if scenario == "technical" && role.Role == "critic" {
				call.Model, call.Status = nil, "skipped"
			}
			calls = append(calls, call)
		}
		return 0, workflow.Atomic("simulated-execution.json", calls)
	case "usage":
		var executedCalls []executed
		if err := workflow.Read("simulated-execution.json", &executedCalls); err != nil {
			return 2, err
		}
		calls := []workflow.Call{}
		for _, actual := range executedCalls {
			if scenario == "usage-drift" && actual.Role == "critic" {
				actual.Model.ModelID = "wrong-fixture-model"
			}
			tokens := int64(5)
			if actual.Status == "skipped" {
				tokens = 0
			}
			calls = append(calls, workflow.Call{ID: workflow.CallID(req.RunID, req.Iteration, req.Phase, actual.Role), Phase: actual.Role, Model: actual.Model, Status: actual.Status, Tokens: &tokens})
		}
		return 0, output.Encode(workflow.Usage{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Calls: calls})
	case "check":
		if _, err := os.Stat("simulated-execution.json"); err != nil {
			return 1, nil
		}
		if scenario == "technical" {
			return 1, nil
		}
		fmt.Println("synthetic fixture accepted; not a visual quality judgment")
		return 0, nil
	}
	return 2, fmt.Errorf("unknown fixture phase")
}

func main() {
	code, err := execute(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 2
	}
	os.Exit(code)
}
