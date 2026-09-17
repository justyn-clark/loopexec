package main

import (
	"encoding/json"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This child-process helper exercises the actual argv/request/receipt boundary.
func TestWorkflowHookHelper(t *testing.T) {
	marker := -1
	for i, a := range os.Args {
		if a == "workflow-helper" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	phase, mode := os.Args[marker+1], os.Args[marker+2]
	var req workflow.Request
	if workflow.Read(os.Args[len(os.Args)-1], &req) != nil {
		os.Exit(2)
	}
	if mode == "hang-"+phase {
		time.Sleep(time.Hour)
	}
	write := func(v any) { json.NewEncoder(os.Stdout).Encode(v) }
	switch phase {
	case "reserve":
		write(workflow.Preflight{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Enforced: true, Calls: []workflow.Reservation{{ID: workflow.CallID(req.RunID, req.Iteration, req.Phase, "builder"), Phase: "builder", MaxMicroUSD: 100000, MaxTokens: 10}}})
	case "exec":
		f, e := os.OpenFile("executions", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			os.Exit(2)
		}
		f.WriteString(strconv.Itoa(req.Iteration) + "\n")
		f.Close()
	case "usage":
		if mode == "bad-usage" {
			os.Stdout.WriteString(`{"schema_version":1,"calls":"bad"}`)
			break
		}
		if mode == "unknown" {
			write(workflow.Usage{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Calls: []workflow.Call{{ID: workflow.CallID(req.RunID, req.Iteration, req.Phase, "builder"), Phase: "builder", Status: "unknown", Tokens: money(10)}}})
			break
		}
		write(workflow.Usage{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Calls: []workflow.Call{{ID: workflow.CallID(req.RunID, req.Iteration, req.Phase, "builder"), Phase: "builder", Status: "actual", MicroUSD: money(100000), Tokens: money(10)}}})
	case "check":
		if mode == "bad-checkpoint" {
			os.Rename(".loopexec/flow/candidate.json", ".loopexec/flow/candidate.saved")
			os.Mkdir(".loopexec/flow/candidate.json", 0700)
		}
		os.Stdout.WriteString("stable-verdict\n")

		if mode == "improve" || mode == "technical" || mode == "cap" {
			os.Exit(1)
		}
	case "score":
		if mode == "bad-score" {
			os.Stdout.WriteString(`{"value":"NaN"}`)
			break
		}
		value := 8.0
		if mode == "improve" {
			value = 7 + float64(req.Iteration)*.2
		}
		if mode == "technical" && req.Iteration == 2 {
			write(workflow.Score{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, CandidateID: req.CandidateID, Reviewed: false, Technical: false})
			break
		}
		write(workflow.Score{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, CandidateID: req.CandidateID, Reviewed: true, Technical: true, Value: &value, Veto: mode == "veto"})
	}
	os.Exit(0)
}
func workflowFixture(t *testing.T, mode string) (runConfig, *workflow.Config) {
	t.Helper()
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	hook := func(phase string) []string {
		return []string{self, "-test.run=^TestWorkflowHookHelper$", "workflow-helper", phase, mode}
	}
	p := numericPolicy()
	p.Command = hook("score")
	w := &workflow.Config{SchemaVersion: 1, Exec: hook("exec"), Check: hook("check"), Meter: workflow.MeterPolicy{Mode: "strict", Reserve: hook("reserve"), Usage: hook("usage"), MaxCalls: 4, MaxTokens: 40}, Score: p}
	cfg := runConfig{runID: "flow", workdir: t.TempDir(), workflowFile: "workflow.json", budgetUSD: 1, maxIterations: 4, commandTimeout: 200 * time.Millisecond, grace: 10 * time.Millisecond}
	if e = workflow.Atomic(filepath.Join(cfg.workdir, cfg.workflowFile), w); e != nil {
		t.Fatal(e)
	}
	return cfg, w
}
func TestWorkflowNumericGuardAndStableFailureIDs(t *testing.T) {
	cfg, _ := workflowFixture(t, "improve")
	cfg.failuresCmd = "printf 'visual.target\n'"
	code, st := runForTest(t, cfg)
	if code != 12 || st.Numeric.Reviewed != 4 || st.Numeric.Stale != 0 || st.Numeric.Best == nil || *st.Numeric.Best != 7.8 {
		t.Fatalf("%d %+v", code, st)
	}
	cfg, _ = workflowFixture(t, "normal")
	code, st = runForTest(t, cfg)
	if code != 17 || st.HaltReason != "no_progress_detected" || st.Iteration != 3 {
		t.Fatalf("green bypassed numeric guard: %d %+v", code, st)
	}
}
func TestWorkflowBudgetStopsBeforeFurtherWorkAndResume(t *testing.T) {
	cfg, _ := workflowFixture(t, "improve")
	cfg.budgetUSD = .2
	code, st := runForTest(t, cfg)
	if code != 18 || st.Budget.MicroUSD != 200000 || st.Budget.Calls != 2 {
		t.Fatalf("%d %+v", code, st)
	}
	b, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if string(b) != "1\n2\n" {
		t.Fatal(string(b))
	}
	cfg.resume = true
	code, st = runForTest(t, cfg)
	again, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if code != 18 || st.Budget.MicroUSD != 200000 || string(again) != string(b) {
		t.Fatalf("resume charged/launched again: %d %+v %s", code, st, again)
	}
}
func TestWorkflowHookFailuresDominateGreen(t *testing.T) {
	for _, mode := range []string{"bad-usage", "bad-score", "hang-reserve", "hang-usage", "hang-score"} {
		t.Run(mode, func(t *testing.T) {
			cfg, _ := workflowFixture(t, mode)
			cfg.commandTimeout = 40 * time.Millisecond
			start := time.Now()
			code, st := runForTest(t, cfg)
			if code == 10 || st.Phase != "halted" || time.Since(start) > 2*time.Second {
				t.Fatalf("%d %+v", code, st)
			}
			if strings.HasPrefix(mode, "hang") {
				b, _ := os.ReadFile(filepath.Join(cfg.workdir, ".loopexec", "run-flow.jsonl"))
				if !strings.Contains(string(b), "command_timeout") {
					t.Fatalf("%s", b)
				}
			}
		})
	}
}
func TestWorkflowTotalDeadlineCoversHookAndPendingUsage(t *testing.T) {
	cfg, _ := workflowFixture(t, "hang-usage")
	cfg.totalTimeout = 500 * time.Millisecond
	cfg.commandTimeout = time.Second
	start := time.Now()
	code, st := runForTest(t, cfg)
	if code == 10 || time.Since(start) > 2*time.Second || st.Budget.Pending == nil || st.Budget.Calls != 0 {
		t.Fatalf("%d %+v", code, st)
	}
	cfg.resume = true
	code, st = runForTest(t, cfg)
	b, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if code == 10 || string(b) != "1\n" || st.Budget.Pending == nil {
		t.Fatal("expired run relaunched work", code, string(b))
	}
}
func TestBudgetFlagRequiresHonestMode(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cfg := runConfig{runID: "budget", workdir: t.TempDir(), budgetUSD: 1, check: "touch should-not-exist", maxIterations: 1}
	if exitCode(executeRun(cmd, cfg)) != 30 {
		t.Fatal("unenforced budget allowed")
	}
	if _, e := os.Stat(filepath.Join(cfg.workdir, "should-not-exist")); !os.IsNotExist(e) {
		t.Fatal("work launched")
	}
}

func TestWorkflowResumeRevalidatesBestAndProtectedPolicy(t *testing.T) {
	cfg, w := workflowFixture(t, "normal")
	w.Score.Target = 8
	cfg.workdir, _ = filepath.EvalSymlinks(cfg.workdir)
	if e := os.Mkdir(filepath.Join(cfg.workdir, "candidate"), 0700); e != nil {
		t.Fatal(e)
	}
	asset := filepath.Join(cfg.workdir, "candidate", "asset.bin")
	os.WriteFile(asset, []byte{0, 255, 7}, 0600)
	os.WriteFile(filepath.Join(cfg.workdir, "policy.txt"), []byte("frozen"), 0600)
	w.Candidate = &workflow.CandidatePolicy{Root: "candidate", Include: []string{"asset.bin"}, Exclude: []string{}, Protected: []string{"policy.txt", "workflow.json"}}
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st := runForTest(t, cfg)
	if code != 10 || st.BestCandidatePath == "" {
		t.Fatalf("%d %+v", code, st)
	}
	cfg.resume = true
	os.WriteFile(asset, []byte("tampered"), 0600)
	code, st = runForTest(t, cfg)
	b, _ := os.ReadFile(asset)
	calls, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if code != 10 || st.BestCandidatePath == "" || string(b) != string([]byte{0, 255, 7}) || string(calls) != "1\n" {
		t.Fatalf("%d %+v %v %s", code, st, b, calls)
	}
	os.WriteFile(filepath.Join(cfg.workdir, "policy.txt"), []byte("weakened"), 0600)
	code, st = runForTest(t, cfg)
	if code != 13 || st.BestCandidatePath != "" || st.Candidate.Exposed {
		t.Fatalf("invalid resumed candidate exposed: %d %+v", code, st)
	}
}

func TestCheckpointFailureDominatesGreen(t *testing.T) {
	cfg, w := workflowFixture(t, "bad-checkpoint")
	w.Score.Target = 8
	cfg.workdir, _ = filepath.EvalSymlinks(cfg.workdir)
	os.Mkdir(filepath.Join(cfg.workdir, "candidate"), 0700)
	os.WriteFile(filepath.Join(cfg.workdir, "candidate/asset"), []byte("original"), 0600)
	w.Candidate = &workflow.CandidatePolicy{Root: "candidate", Include: []string{"asset"}, Protected: []string{"workflow.json"}}
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st := runForTest(t, cfg)
	if code != 40 || st.BestCandidatePath != "" || st.Candidate.Exposed || st.HaltReason == "success_condition_met" {
		t.Fatalf("%d %+v", code, st)
	}
}

func TestWorkflowObservedAndUnmeteredModes(t *testing.T) {
	cfg, w := workflowFixture(t, "normal")
	cfg.budgetUSD = .05
	w.Meter.Mode = "observed"
	w.Meter.Reserve = nil
	w.Meter.MaxCalls = 0
	w.Meter.MaxTokens = 0
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st := runForTest(t, cfg)
	if code != 18 || st.Budget.Mode != "observed" || st.Budget.MicroUSD != 100000 || st.Budget.Calls != 1 {
		t.Fatalf("%d %+v", code, st)
	}
	cfg, w = workflowFixture(t, "unknown")
	cfg.budgetUSD = 0
	w.Meter.Mode = "unmetered"
	w.Score.Target = 8
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st = runForTest(t, cfg)
	if code != 10 || st.Budget.Mode != "unmetered" || st.Budget.MonetaryKnown || st.Budget.Calls != 1 || st.Budget.Tokens != 10 {
		t.Fatalf("%d %+v", code, st)
	}
}

func TestCriticVetoDominatesGreen(t *testing.T) {
	cfg, w := workflowFixture(t, "veto")
	w.Score.Target = 8
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st := runForTest(t, cfg)
	if code != 17 || st.Numeric.Best != nil || st.Numeric.Reviewed != 2 {
		t.Fatalf("%d %+v", code, st)
	}
}
