package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justyn-clark/loopexec/internal/modelpolicy"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"github.com/spf13/cobra"
)

func TestModelHookHelper(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "model-helper" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	phase, mode := os.Args[marker+1], os.Args[marker+2]
	var req workflow.Request
	if workflow.Read(os.Args[len(os.Args)-1], &req) != nil || req.Models == nil {
		os.Exit(2)
	}
	identity := req.Models.Find("critic").Identity
	write := func(v any) { json.NewEncoder(os.Stdout).Encode(v) }
	id := workflow.CallID(req.RunID, req.Iteration, req.Phase, "critic")
	switch phase {
	case "reserve":
		if mode == "reserve-drift" {
			identity.ModelID = "unapproved"
		}
		write(workflow.Preflight{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Enforced: true, Calls: []workflow.Reservation{{ID: id, Phase: "critic", Model: &identity, MaxMicroUSD: 100000, MaxTokens: 10}}})
	case "exec":
		identity.ModelID, identity.ReasoningEffort = os.Args[marker+3], os.Args[marker+4]
		if workflow.Atomic("execution.json", identity) != nil {
			os.Exit(2)
		}
	case "usage":
		if workflow.Read("execution.json", &identity) != nil {
			os.Exit(2)
		}
		if mode == "usage-drift" {
			identity.ReasoningEffort = "low"
		}
		write(workflow.Usage{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Calls: []workflow.Call{{ID: id, Phase: "critic", Model: &identity, Status: "actual", MicroUSD: money(100000), Tokens: money(10)}}})
	case "check":
		if mode == "reject" {
			os.Exit(1)
		}
		os.Stdout.WriteString("stable\n")
	case "score":
		v := 5.0
		write(workflow.Score{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, CandidateID: req.CandidateID, Technical: true, Reviewed: true, Value: &v})
	}
	os.Exit(0)
}

func modelFixture(t *testing.T, mode string) (runConfig, *workflow.Config, func(string) []string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	hook := func(phase string) []string {
		return []string{self, "-test.run=^TestModelHookHelper$", "model-helper", phase, mode}
	}
	command := append(hook("exec"), "{critic.model}", "{critic.reasoning}")
	policy := &modelpolicy.Policy{Mode: "explicit", Task: "hardware-review", Profiles: map[string]modelpolicy.Profile{
		"visual": {Identity: modelpolicy.Identity{Provider: "offline", ModelID: "fixture-vision", ReasoningEffort: "high"}, Capabilities: []string{"vision"}},
	}, Tasks: map[string]modelpolicy.Task{"hardware-review": {Roles: map[string]modelpolicy.Assignment{"critic": {Profile: "visual", Reason: "Compare saved equipment captures with reference images", Requires: []string{"vision"}}}, Command: command}}}
	w := &workflow.Config{SchemaVersion: 1, Models: policy, Check: hook("check"), Meter: workflow.MeterPolicy{Mode: "strict", Reserve: hook("reserve"), Usage: hook("usage"), MaxCalls: 1, MaxTokens: 10}}
	cfg := runConfig{runID: "models", workdir: t.TempDir(), workflowFile: "workflow.json", budgetUSD: .5, maxIterations: 1, commandTimeout: time.Second, grace: 10 * time.Millisecond}
	writeModelWorkflow(t, cfg, w)
	return cfg, w, hook
}

func writeModelWorkflow(t *testing.T, cfg runConfig, w *workflow.Config) {
	t.Helper()
	if err := workflow.Atomic(filepath.Join(cfg.workdir, cfg.workflowFile), w); err != nil {
		t.Fatal(err)
	}
}

func TestModelExplicitDispatchAndReceipt(t *testing.T) {
	cfg, _, _ := modelFixture(t, "normal")
	code, st := runForTest(t, cfg)
	if code != 10 || st.ModelSelection == nil || st.Budget.Calls != 1 {
		t.Fatalf("%d %+v", code, st)
	}
	var called modelpolicy.Identity
	if err := workflow.Read(filepath.Join(cfg.workdir, "execution.json"), &called); err != nil || called.ModelID != "fixture-vision" || called.ReasoningEffort != "high" {
		t.Fatalf("actual argv not selected: %+v %v", called, err)
	}
	events, _, status := readReceiptEvents(filepath.Join(cfg.workdir, ".loopexec/run-models.jsonl"), cfg.runID)
	if status != "readable" {
		t.Fatal(status)
	}
	found := false
	for _, event := range events {
		found = found || (event.Event == "model_selection" && event.ModelSelection.Find("critic").Profile == "visual")
	}
	if !found {
		t.Fatal("typed selection evidence missing")
	}
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"report", "--workdir", cfg.workdir, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result response
	if json.Unmarshal(out.Bytes(), &result) != nil || result.Report.ModelSelection.Find("critic").ModelID != called.ModelID || result.Report.Budget.Entries[workflow.CallID(cfg.runID, 1, "exec", "critic")].Model == nil {
		t.Fatal(out.String())
	}
	key := []byte("fixture-signing-key")
	sig, _ := signReceipt(st, key)
	st.ModelSelection.Roles[0].ReasoningEffort = "low"
	changed, _ := signReceipt(st, key)
	if sig == changed {
		t.Fatal("selection is not covered by attestation")
	}
}

func TestModelDriftAndBudgetGuards(t *testing.T) {
	for _, mode := range []string{"reserve-drift", "usage-drift", "budget", "reject", "numeric"} {
		t.Run(mode, func(t *testing.T) {
			cfg, w, hook := modelFixture(t, mode)
			if mode == "budget" {
				cfg.budgetUSD = .05
			}
			if mode == "numeric" {
				w.Score = &workflow.ScorePolicy{Command: hook("score"), Direction: "maximize", Min: 0, Max: 10, Target: 9, MinImprovement: .1, Tolerance: .01, Patience: 2}
				writeModelWorkflow(t, cfg, w)
			}
			code, st := runForTest(t, cfg)
			if code == 10 {
				t.Fatal("model policy bypassed existing acceptance guard")
			}
			_, executed := os.Stat(filepath.Join(cfg.workdir, "execution.json"))
			if mode == "reserve-drift" || mode == "budget" {
				if !os.IsNotExist(executed) || (mode == "budget" && code != 18) || (mode == "reserve-drift" && code != 19) {
					t.Fatalf("work launched before approval: %d %v", code, executed)
				}
			} else if mode == "usage-drift" && (code != 19 || st.Budget.MicroUSD != 100000 || st.Budget.Calls != 1) {
				t.Fatalf("mismatch lost incurred usage: %d %+v", code, st.Budget)
			}
			if mode == "numeric" && (code != 12 || st.Numeric == nil || st.Numeric.Best == nil || *st.Numeric.Best != 5) {
				t.Fatalf("numeric acceptance changed: %d %+v", code, st.Numeric)
			}
			if mode == "reject" && (code != 12 || st.LastCheckExit == nil || *st.LastCheckExit != 1) {
				t.Fatalf("red check bypassed: %d %+v", code, st)
			}
		})
	}
}

func TestModelGuidanceDoesNotSelectAndResumePinsPolicy(t *testing.T) {
	cfg, w, hook := modelFixture(t, "normal")
	w.Models.Mode = "advisory"
	w.Exec = append(hook("exec"), "adapter-choice", "low")
	writeModelWorkflow(t, cfg, w)
	code, st := runForTest(t, cfg)
	if code != 10 || st.ModelSelection.Assurance != "guidance_only" {
		t.Fatalf("%d %+v", code, st)
	}
	var actual modelpolicy.Identity
	if workflow.Read(filepath.Join(cfg.workdir, "execution.json"), &actual) != nil || actual.ModelID != "adapter-choice" {
		t.Fatal("advice changed adapter dispatch")
	}
	cfg.resume = true
	statePath := filepath.Join(cfg.workdir, ".loopexec", perRunStateName(cfg.runID))
	before, _ := os.ReadFile(statePath)
	profile := w.Models.Profiles["visual"]
	profile.ReasoningEffort = "low"
	w.Models.Profiles["visual"] = profile
	writeModelWorkflow(t, cfg, w)
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if code := exitCode(executeRun(cmd, cfg)); code != 30 {
		t.Fatalf("changed assignment resumed: %d", code)
	}
	after, _ := os.ReadFile(statePath)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected resume changed saved state or allowance")
	}
}

func TestModelsPlanIsReadOnlyAndPreflightRejectsAmbiguity(t *testing.T) {
	cfg, w, _ := modelFixture(t, "normal")
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"models", "plan", "--workflow", cfg.workflowFile, "--workdir", cfg.workdir, "--json"})
	if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), "adapter_report_required") {
		t.Fatalf("%v %s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.workdir, ".loopexec")); !os.IsNotExist(err) {
		t.Fatal("planning initialized a run or invoked a hook")
	}
	w.Exec = []string{"unexpected"}
	writeModelWorkflow(t, cfg, w)
	if _, _, err := loadWorkflow(cfg); err == nil {
		t.Fatal("exec override accepted")
	}
	w.Exec = nil
	w.Meter.Reserve = nil
	writeModelWorkflow(t, cfg, w)
	if _, _, err := loadWorkflow(cfg); err == nil {
		t.Fatal("explicit assignment accepted without pre-call declaration")
	}
}

func TestLegacyAttestationOmitsNewSelection(t *testing.T) {
	b, err := canonicalReceipt(loopState{RunID: "legacy"})
	if err != nil || bytes.Contains(b, []byte("model_selection")) {
		t.Fatalf("legacy signature payload changed: %s %v", b, err)
	}
}

func TestModelRoleEvidenceIsCompleteAndVersionAware(t *testing.T) {
	s := &modelpolicy.Selection{Mode: "explicit", Roles: []modelpolicy.Role{
		{Role: "builder", Identity: modelpolicy.Identity{Provider: "offline", ModelID: "small"}},
		{Role: "critic", Identity: modelpolicy.Identity{Provider: "offline", ModelID: "vision", Version: "r1", ReasoningEffort: "high"}},
	}}
	builder, critic := s.Roles[0].Identity, s.Roles[1].Identity
	calls := []workflow.Reservation{{Phase: "builder", Model: &builder}, {Phase: "critic", Model: &critic}, {Phase: "local-tool"}}
	if !modelReservations(s, calls) || modelReservations(s, calls[:1]) {
		t.Fatal("planned roles incomplete or local tool rejected")
	}
	critic.Version = "r2"
	if modelReservations(s, calls) {
		t.Fatal("version mismatch accepted")
	}
	if !modelUsage(s, []workflow.Call{{Phase: "critic", Status: "skipped"}}) || modelUsage(s, []workflow.Call{{Phase: "critic", Status: "actual"}}) || modelUsage(s, []workflow.Call{{Phase: "undeclared", Model: &builder}}) {
		t.Fatal("missing/undeclared model identity accepted or skipped phase rejected")
	}
}
