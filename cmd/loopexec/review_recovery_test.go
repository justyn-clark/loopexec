package main

import (
	"context"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewResumeAllowsAccountingAndCandidateProgressAhead(t *testing.T) {
	req, p, usage, policy := budgetFixture()
	b := newBudget("strict")
	b.reserve(p, req, policy, 600000)
	b.DispatchPending = true
	prior := *b
	if reason := b.reconcile(usage, req, policy, 600000); reason != "" {
		t.Fatal(reason)
	}
	file := filepath.Join(t.TempDir(), "budget.json")
	workflow.Atomic(file, b)
	loaded, err := loadResumeBudget(file, "strict", &prior)
	if err != nil || loaded.Calls != 2 {
		t.Fatalf("valid settlement ahead of state rejected: %v", err)
	}
	c, _ := candidateFixture(t)
	c.attempted()
	c.promote(&checkFingerprint{})
	before := c.state
	c.begin()
	os.WriteFile(filepath.Join(c.root, "assets/generated.bin"), []byte("new committed generation"), 0600)
	c.iteration = 1
	c.attempted()
	c.promote(&checkFingerprint{})
	if !candidateResumeProgress(c.state, before, 1) {
		t.Fatal("valid promotion ahead of state rejected")
	}
	fresh, err := newCandidate(context.Background(), filepath.Dir(c.root), filepath.Dir(c.store), c.policy)
	if err != nil || fresh.state.Best != c.state.Best {
		t.Fatal("advanced journal unreadable")
	}
}

func TestReviewObservedDispatchCannotDisappearWithoutSettlement(t *testing.T) {
	for _, mode := range []string{"observed", "unmetered"} {
		t.Run(mode, func(t *testing.T) {
			prior := newBudget(mode)
			prior.Pending = &workflow.Preflight{SchemaVersion: 1, RunID: "flow", Iteration: 1, Phase: "exec"}
			prior.DispatchPending = true
			file := filepath.Join(t.TempDir(), "budget.json")
			if err := workflow.Atomic(file, newBudget(mode)); err != nil {
				t.Fatal(err)
			}
			if _, err := loadResumeBudget(file, mode, prior); err == nil {
				t.Fatal("unreserved dispatched phase disappeared without settlement")
			}
			// A valid saved settlement ahead of the state remains resumable.
			next := *prior
			u := workflow.Usage{SchemaVersion: 1, RunID: "flow", Iteration: 1, Phase: "exec", Calls: []workflow.Call{{ID: workflow.CallID("flow", 1, "exec", "builder"), Phase: "builder", Status: "actual", MicroUSD: money(100), Tokens: money(20)}}}
			if reason := next.reconcile(u, workflow.Request{RunID: "flow", Iteration: 1, Phase: "exec"}, workflow.MeterPolicy{Mode: mode}, 0); reason != "" {
				t.Fatal(reason)
			}
			if err := workflow.Atomic(file, &next); err != nil {
				t.Fatal(err)
			}
			if loaded, err := loadResumeBudget(file, mode, prior); err != nil || loaded.Calls != 1 || loaded.Tokens != 20 {
				t.Fatalf("valid observed settlement rejected: %+v %v", loaded, err)
			}
		})
	}
}

func TestReviewRetainedBaselineReplaysAfterFailedAttempt(t *testing.T) {
	cfg, w := reviewCandidateFixture(t, "review-baseline-retained")
	w.Candidate.Baseline = true
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st := runForTest(t, cfg)
	if code != 40 || st.Candidate.BestIteration != 0 || !st.Candidate.Exposed {
		t.Fatalf("fixture did not retain verified baseline: %d %+v", code, st.Candidate)
	}
	if _, verified, err := verifyWorkflowFingerprint(st, cfg.workdir); err != nil || !verified {
		t.Fatalf("retained baseline cannot replay: %v", err)
	}
}

func TestReviewObservedUsageCannotSettleAsLessOrSkipped(t *testing.T) {
	for _, kind := range []string{"skipped", "tokens", "money"} {
		t.Run(kind, func(t *testing.T) {
			prior := newBudget("observed")
			prior.Pending = &workflow.Preflight{SchemaVersion: 1, RunID: "flow", Iteration: 1, Phase: "exec"}
			prior.DispatchPending = true
			call := workflow.Call{ID: workflow.CallID("flow", 1, "exec", "builder"), Phase: "builder", Status: "actual", MicroUSD: money(100), Tokens: money(20)}
			prior.Unsettled = &workflow.Usage{SchemaVersion: 1, RunID: "flow", Iteration: 1, Phase: "exec", Calls: []workflow.Call{call}}
			switch kind {
			case "skipped":
				call.Status, call.MicroUSD, call.Tokens = "skipped", money(0), money(0)
			case "tokens":
				call.Tokens = money(19)
			case "money":
				call.MicroUSD = money(99)
			}
			next := newBudget("observed")
			u := workflow.Usage{SchemaVersion: 1, RunID: "flow", Iteration: 1, Phase: "exec", Calls: []workflow.Call{call}}
			if reason := next.reconcile(u, workflow.Request{RunID: "flow", Iteration: 1, Phase: "exec"}, workflow.MeterPolicy{Mode: "observed"}, 0); reason != "" {
				t.Fatal(reason)
			}
			file := filepath.Join(t.TempDir(), "budget.json")
			workflow.Atomic(file, next)
			if _, err := loadResumeBudget(file, "observed", prior); err == nil {
				t.Fatal("saved accounting discarded observed executed usage")
			}
		})
	}
}

func reviewCandidateFixture(t *testing.T, mode string) (runConfig, *workflow.Config) {
	t.Helper()
	cfg, w := workflowFixture(t, mode)
	cfg.commandTimeout = 2 * time.Second
	os.MkdirAll(filepath.Join(cfg.workdir, "candidate/assets"), 0700)
	os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/value"), []byte("baseline"), 0600)
	os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/payload"), []byte("payload"), 0600)
	w.Candidate = &workflow.CandidatePolicy{Root: "candidate", Include: []string{"assets"}}
	w.Score = nil
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	return cfg, w
}

func TestReviewResumeRejectsMissingOrRegressedAccounting(t *testing.T) {
	for _, kind := range []string{"missing", "regressed"} {
		t.Run(kind, func(t *testing.T) {
			cfg, _ := workflowFixture(t, "bad-score")
			cfg.commandTimeout = 2 * time.Second
			_, prior := runForTest(t, cfg)
			book := filepath.Join(cfg.workdir, ".loopexec/flow/budget.json")
			if kind == "missing" {
				os.Remove(book)
			} else {
				workflow.Atomic(book, newBudget("strict"))
			}
			before, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
			cfg.resume = true
			code, after := runForTest(t, cfg)
			actual, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
			if code != 18 || string(before) != string(actual) || after.Budget.Calls != prior.Budget.Calls {
				t.Fatalf("accounting reset admitted: %d prior=%+v after=%+v", code, prior.Budget, after.Budget)
			}
		})
	}
}

func TestReviewObservedContinuationCannotRenewMoney(t *testing.T) {
	for _, cap := range []float64{0, 2} {
		t.Run(string(rune('a'+int(cap))), func(t *testing.T) {
			cfg, w := workflowFixture(t, "bad-score")
			cfg.commandTimeout = 2 * time.Second
			w.Meter.Mode = "observed"
			workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
			runForTest(t, cfg)
			state, _ := os.ReadFile(filepath.Join(cfg.workdir, ".loopexec", perRunStateName("flow")))
			receipt, _ := os.ReadFile(filepath.Join(cfg.workdir, ".loopexec/run-flow.jsonl"))
			w.Continuation = &workflow.Continuation{Workdir: ".", RunID: "flow", StateSHA256: sha256hex(state), ReceiptSHA256: sha256hex(receipt)}
			workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
			cfg.runID = "linked"
			cfg.budgetUSD = cap
			code, _ := runForTest(t, cfg)
			if code != 30 {
				t.Fatalf("observed monetary cap renewed: %d", code)
			}
		})
	}
}

func TestReviewResumeRejectsStaleCommittedBest(t *testing.T) {
	cfg, w := reviewCandidateFixture(t, "review-best-journal")
	w.Score = numericPolicy()
	w.Score.Command = []string{os.Args[0], "-test.run=^TestWorkflowHookHelper$", "workflow-helper", "score", "review-best-journal"}
	cfg.maxIterations = 2
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	_, prior := runForTest(t, cfg)
	var stale candidateState
	if err := workflow.Read(filepath.Join(cfg.workdir, "stale-journal.json"), &stale); err != nil {
		t.Fatal(err)
	}
	if stale.Best == prior.Candidate.Best {
		t.Fatal("fixture did not commit a new best")
	}
	workflow.Atomic(filepath.Join(cfg.workdir, ".loopexec/flow/candidate.json"), stale)
	cfg.resume = true
	code, after := runForTest(t, cfg)
	value, _ := os.ReadFile(filepath.Join(cfg.workdir, "candidate/assets/value"))
	if code != 13 || after.Candidate.Best != prior.Candidate.Best || string(value) != "2" {
		t.Fatalf("stale journal discarded committed best: %d %+v %s", code, after.Candidate, value)
	}
}

func TestReviewResumeCannotExposeCorruptStoredBest(t *testing.T) {
	cfg, _ := reviewCandidateFixture(t, "normal")
	_, prior := runForTest(t, cfg)
	os.WriteFile(filepath.Join(cfg.workdir, ".loopexec/flow/candidates", prior.Candidate.Best, "files/assets/payload"), []byte("corrupt"), 0600)
	cfg.resume = true
	code, after := runForTest(t, cfg)
	if code == 10 || after.BestCandidatePath != "" || after.Candidate.Exposed {
		t.Fatal("corrupt stored best exposed")
	}
}

func TestReviewReplayBindsCompleteCandidateAndSnapshot(t *testing.T) {
	for _, kind := range []string{"working", "snapshot", "during"} {
		t.Run(kind, func(t *testing.T) {
			cfg, _ := reviewCandidateFixture(t, "review-replay-mutation")
			_, st := runForTest(t, cfg)
			switch kind {
			case "working":
				os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/payload"), []byte("other bytes"), 0600)
			case "snapshot":
				os.WriteFile(filepath.Join(cfg.workdir, ".loopexec/flow/candidates", st.Candidate.Best, "files/assets/payload"), []byte("corrupt"), 0600)
			case "during":
				os.WriteFile(filepath.Join(cfg.workdir, "replay-mutation-enabled"), []byte("enable"), 0600)
			}
			_, verified, err := verifyWorkflowFingerprint(st, cfg.workdir)
			if verified && err == nil {
				t.Fatal("replay verified bytes not bound to the receipt")
			}
		})
	}
}

func TestReviewBaselineCannotPromoteHookMutations(t *testing.T) {
	cfg, w := reviewCandidateFixture(t, "review-baseline-mutation")
	w.Candidate.Baseline = true
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st := runForTest(t, cfg)
	if code != 13 || st.Candidate.Best != "" {
		t.Fatalf("unchecked baseline promoted: %d %+v", code, st.Candidate)
	}
	if _, err := os.Stat(filepath.Join(cfg.workdir, "executions")); !os.IsNotExist(err) {
		t.Fatal("invalid baseline dispatched work")
	}
}

func TestReviewRestorationHandlesFileDirectoryTransitions(t *testing.T) {
	for _, kind := range []string{"file-to-dir", "dir-to-file", "excluded"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := candidateFixture(t)
			p := filepath.Join(c.root, "assets/generated.bin")
			if kind == "dir-to-file" {
				os.Remove(p)
				os.Mkdir(p, 0700)
				os.WriteFile(filepath.Join(p, "nested"), []byte("baseline nested"), 0600)
			}
			if kind == "excluded" {
				c.policy.Exclude = append(c.policy.Exclude, "assets/generated.bin/cache")
			}
			id, _ := c.attempted()
			if err := c.promote(&checkFingerprint{}); err != nil {
				t.Fatal(err)
			}
			c.begin()
			if kind == "dir-to-file" {
				os.Remove(filepath.Join(p, "nested"))
				os.Remove(p)
				os.WriteFile(p, []byte("file attempt"), 0600)
			} else {
				os.Remove(p)
				os.Mkdir(p, 0700)
				os.WriteFile(filepath.Join(p, "nested"), []byte("nested attempt"), 0600)
			}
			if kind == "excluded" {
				os.Mkdir(filepath.Join(p, "cache"), 0700)
				os.WriteFile(filepath.Join(p, "cache/keep"), []byte("keep excluded"), 0600)
			}
			err := c.restore()
			if kind == "excluded" {
				b, _ := os.ReadFile(filepath.Join(p, "cache/keep"))
				if err == nil || string(b) != "keep excluded" || c.state.Exposed {
					t.Fatal("excluded topology conflict was destroyed or exposed")
				}
				return
			}
			if err != nil || c.state.Restored != id || !c.state.Exposed {
				t.Fatalf("ordinary topology not restored: %v %+v", err, c.state)
			}
		})
	}
}

func TestReviewExecutionAndAccountingFailuresBothSurvive(t *testing.T) {
	for _, mode := range []string{"review-exec-usage", "review-timeout-usage"} {
		t.Run(mode, func(t *testing.T) {
			cfg, _ := workflowFixture(t, mode)
			cfg.commandTimeout = 2 * time.Second
			code, st := runForTest(t, cfg)
			if code != 40 || len(st.Failures) != 2 || st.Failures[0].Reason != "execution_failure" || st.Failures[1].Reason != "cost_anomaly" || st.Budget.Pending == nil {
				t.Fatalf("execution/accounting failure lost: %d %+v", code, st.Failures)
			}
		})
	}
}

func TestReviewResumePinsLegacyIdentityAndSampling(t *testing.T) {
	for _, field := range []string{"model", "provider", "version", "temperature", "seed", "max-tokens"} {
		t.Run(field, func(t *testing.T) {
			cfg, w := workflowFixture(t, "normal")
			cfg.commandTimeout = 2 * time.Second
			w.Score = nil
			workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
			cfg.modelID = "original"
			cfg.modelProvider = "provider"
			cfg.modelVersion = "v1"
			cfg.temperature = .2
			cfg.seed = 3
			cfg.maxTokens = 128
			runForTest(t, cfg)
			before, _ := os.ReadFile(filepath.Join(cfg.workdir, ".loopexec/run-flow.jsonl"))
			cfg.resume = true
			switch field {
			case "model":
				cfg.modelID = "other"
			case "provider":
				cfg.modelProvider = "other"
			case "version":
				cfg.modelVersion = "v2"
			case "temperature":
				cfg.temperature = .9
			case "seed":
				cfg.seed = 4
			case "max-tokens":
				cfg.maxTokens = 129
			}
			code, _ := runForTest(t, cfg)
			after, _ := os.ReadFile(filepath.Join(cfg.workdir, ".loopexec/run-flow.jsonl"))
			if code != 30 || string(before) != string(after) {
				t.Fatal("legacy resume rewrote original pins or receipt")
			}
		})
	}
}
