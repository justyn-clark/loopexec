package main

import (
	"context"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func linkFixture(t *testing.T, mode string) (runConfig, *workflow.Config, loopState, []byte) {
	t.Helper()
	cfg, w := workflowFixture(t, mode)
	cfg.commandTimeout = 2 * time.Second
	cfg.totalTimeout = time.Minute
	_, parent := runForTest(t, cfg)
	state := filepath.Join(cfg.workdir, ".loopexec", perRunStateName(cfg.runID))
	receipt := filepath.Join(cfg.workdir, ".loopexec", "run-"+cfg.runID+".jsonl")
	stateBytes, _ := os.ReadFile(state)
	receiptBytes, _ := os.ReadFile(receipt)
	w.Continuation = &workflow.Continuation{Workdir: ".", RunID: cfg.runID, StateSHA256: sha256hex(stateBytes), ReceiptSHA256: sha256hex(receiptBytes)}
	cfg.runID = "linked"
	cfg.totalTimeout = 0
	w.Score = nil
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	return cfg, w, parent, receiptBytes
}

func TestContinuationCarriesSpendIterationAndDeadline(t *testing.T) {
	cfg, _, parent, receipt := linkFixture(t, "bad-score")
	code, child := runForTest(t, cfg)
	if code != 10 || child.Budget.Calls != parent.Budget.Calls+1 || child.Iteration != parent.Iteration+1 || child.DeadlineNS != parent.DeadlineNS || len(child.Lineage) != 1 {
		t.Fatalf("continuation reset bounds: %d %+v", code, child)
	}
	old, _ := os.ReadFile(filepath.Join(cfg.workdir, ".loopexec/run-flow.jsonl"))
	if string(old) != string(receipt) {
		t.Fatal("parent receipt rewritten")
	}
	data, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if string(data) != "1\n2\n" {
		t.Fatal("completed work replayed")
	}
}

func TestContinuationRejectsUnknownAccountingAndRenewedBounds(t *testing.T) {
	for _, scenario := range []string{"bad-usage", "calls", "iterations", "receipt", "tokens", "money"} {
		t.Run(scenario, func(t *testing.T) {
			mode := "bad-score"
			if scenario == "bad-usage" {
				mode = scenario
			}
			cfg, w, _, receipt := linkFixture(t, mode)
			switch scenario {
			case "calls":
				w.Meter.MaxCalls++
			case "tokens":
				w.Meter.MaxTokens++
			case "iterations":
				cfg.maxIterations++
			case "money":
				cfg.budgetUSD = 2
			case "receipt":
				os.WriteFile(filepath.Join(cfg.workdir, ".loopexec/run-flow.jsonl"), append(receipt, '\n'), 0600)
			}
			workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
			before, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
			code, _ := runForTest(t, cfg)
			if code != 30 {
				t.Fatalf("unsafe continuation accepted: %d", code)
			}
			after, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
			if string(before) != string(after) {
				t.Fatal("rejected link invoked work")
			}
		})
	}
}

func TestContinuationExpiredDeadlineDoesNotRun(t *testing.T) {
	cfg, w, parent, _ := linkFixture(t, "bad-score")
	parent.DeadlineNS = time.Now().Add(-time.Second).UnixNano()
	dir := filepath.Join(cfg.workdir, ".loopexec")
	writeStateFile(dir, perRunStateName("flow"), parent)
	data, _ := os.ReadFile(filepath.Join(dir, perRunStateName("flow")))
	w.Continuation.StateSHA256 = sha256hex(data)
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	before, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	code, child := runForTest(t, cfg)
	if code == 10 || child.DeadlineNS != parent.DeadlineNS {
		t.Fatal("deadline renewed")
	}
	after, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if string(before) != string(after) {
		t.Fatal("expired link launched work")
	}
}

func TestContinuationRejectsStaleRecoveryJournal(t *testing.T) {
	cfg, w, parent, _ := linkFixture(t, "bad-score")
	parent.Candidate = &candidateState{Status: "restored"}
	dir := filepath.Join(cfg.workdir, ".loopexec")
	if err := writeStateFile(dir, perRunStateName("flow"), parent); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, perRunStateName("flow")))
	w.Continuation.StateSHA256 = sha256hex(data)
	if err := workflow.Atomic(filepath.Join(dir, "flow/candidate.json"), candidateState{Status: "restoring"}); err != nil {
		t.Fatal(err)
	}
	if _, release, err := continuationParent(context.Background(), cfg, w); err == nil {
		release()
		t.Fatal("stale state concealed partial restoration")
	}
}

func TestContinuationRejectsSymlinkedEvidenceAndActiveParent(t *testing.T) {
	for _, scenario := range []string{"symlink", "active", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			cfg, w, _, _ := linkFixture(t, "bad-score")
			ctx := context.Background()
			switch scenario {
			case "symlink":
				dir := filepath.Join(cfg.workdir, ".loopexec/flow")
				if err := os.Rename(dir, dir+"-saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("flow-saved", dir); err != nil {
					t.Fatal(err)
				}
			case "active":
				release, err := workflow.Lock(filepath.Join(cfg.workdir, ".loopexec"))
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				cfg.workdir = filepath.Join(cfg.workdir, "child")
				w.Continuation.Workdir = ".."
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, release, err := continuationParent(ctx, cfg, w); err == nil {
				release()
				t.Fatal("unsafe link accepted")
			}
		})
	}
}

func TestContinuationCannotResetExhaustedCalls(t *testing.T) {
	cfg, w, parent, _ := linkFixture(t, "bad-score")
	w.Meter.MaxCalls = parent.Budget.Calls
	if err := workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	code, child := runForTest(t, cfg)
	after, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if code != 18 || child.Budget.Calls != parent.Budget.Calls || string(before) != string(after) {
		t.Fatalf("continuation renewed spent calls: %d %+v", code, child.Budget)
	}
}

func TestResumeRejectsChangedContextBeforeReceiptMutation(t *testing.T) {
	cfg, _ := workflowFixture(t, "bad-score")
	cfg.contextFiles = []string{"adapter-pin"}
	pin := filepath.Join(cfg.workdir, "adapter-pin")
	if err := os.WriteFile(pin, []byte("original adapter"), 0600); err != nil {
		t.Fatal(err)
	}
	runForTest(t, cfg)
	receipt := filepath.Join(cfg.workdir, ".loopexec/run-flow.jsonl")
	before, _ := os.ReadFile(receipt)
	executions, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if err := os.WriteFile(pin, []byte("changed adapter"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.resume = true
	code, _ := runForTest(t, cfg)
	after, _ := os.ReadFile(receipt)
	actual, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if code != 30 || string(before) != string(after) || string(executions) != string(actual) {
		t.Fatal("changed-context resume mutated original evidence or launched work")
	}
}
