package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/justyn-clark/loopexec/internal/workflow"
)

func TestOfflineCrashRuntimeHelper(t *testing.T) {
	for i, arg := range os.Args {
		if arg != "offline-crash-runtime" {
			continue
		}
		cfg := runConfig{runID: "flow", workdir: os.Args[i+1], workflowFile: "workflow.json", budgetUSD: 1, maxIterations: 3, commandTimeout: 10 * time.Second, grace: 10 * time.Millisecond}
		runForTest(t, cfg)
		t.Fatal("governor returned instead of being killed during dispatch")
	}
}

func TestOfflineInterruptedDispatchRejectsStaleBudget(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("real crash/resume requires the advisory-lock platform implementation")
	}
	cfg, w := workflowFixture(t, "offline-crash-window")
	cfg.maxIterations, cfg.commandTimeout = 3, 10*time.Second
	w.Meter.MaxCalls, w.Meter.MaxTokens, w.Score = 2, 20, nil
	if err := workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestOfflineCrashRuntimeHelper$", "offline-crash-runtime", cfg.workdir)
	if output, err := child.CombinedOutput(); err == nil {
		t.Fatalf("governor was not interrupted: %s", output)
	}
	before, err := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if err != nil || string(before) != "1\n2\n" {
		t.Fatalf("crash did not follow two actual stand-in dispatches: %q %v", before, err)
	}
	stale, err := os.ReadFile(filepath.Join(cfg.workdir, "settled-before-crash.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(cfg.workdir, ".loopexec/flow/budget.json"), stale, 0600); err != nil {
		t.Fatal(err)
	}
	cfg.resume = true
	code, st := runForTest(t, cfg)
	after, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if code != 18 || string(after) != string(before) || st.Budget.Pending == nil || !st.Budget.DispatchPending {
		t.Fatalf("interrupted call erased by stale journal: code=%d executions=%q pending=%+v", code, after, st.Budget.Pending)
	}
}

func TestOfflineConflictingUsageCannotSettleCheaper(t *testing.T) {
	cfg, w := workflowFixture(t, "offline-conflicting-usage")
	cfg.maxIterations, cfg.commandTimeout, w.Score = 1, 3*time.Second, nil
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, initial := runForTest(t, cfg)
	if code != 18 || initial.Budget.Unsettled == nil || initial.Budget.Unsettled.Calls[0].MicroUSD == nil || *initial.Budget.Unsettled.Calls[0].MicroUSD != 80 {
		t.Fatal("fixture did not retain the first rejected report")
	}
	first := workflow.JSONHash(initial.Budget.Unsettled)
	cfg.resume = true
	code, st := runForTest(t, cfg)
	if code != 18 || st.Budget.Pending == nil || st.Budget.Calls != 0 || st.Budget.MicroUSD != 0 || workflow.JSONHash(st.Budget.Unsettled) != first {
		t.Fatalf("conflicting replacement erased the observation: code=%d budget=%+v", code, st.Budget)
	}
	files, err := os.ReadDir(filepath.Join(cfg.workdir, ".loopexec/flow/usage-reports"))
	if err != nil || len(files) != 2 {
		t.Fatalf("both exact collector reports must survive: %v count=%d", err, len(files))
	}
	amounts := map[int64]bool{}
	for _, file := range files {
		var u workflow.Usage
		if err := workflow.Read(filepath.Join(cfg.workdir, ".loopexec/flow/usage-reports", file.Name()), &u); err != nil {
			t.Fatal(err)
		}
		amounts[*u.Calls[0].MicroUSD] = true
	}
	if !amounts[80] || !amounts[1] {
		t.Fatal("original and conflicting evidence were not retained separately")
	}
}

func TestOfflineResumeRejectsChangedUnsettledContents(t *testing.T) {
	req, reservation, u, policy := budgetFixture()
	b := newBudget("strict")
	b.reserve(reservation, req, policy, 600000)
	b.DispatchPending, b.Unsettled = true, &u
	data, _ := json.Marshal(b)
	var changed budgetBook
	if err := workflow.Decode(data, &changed); err != nil {
		t.Fatal(err)
	}
	changed.Unsettled.Calls[0].MicroUSD = money(1)
	file := filepath.Join(t.TempDir(), "budget.json")
	workflow.Atomic(file, &changed)
	if _, err := loadResumeBudget(file, "strict", b); err == nil {
		t.Fatal("same pending reservation admitted lower observed usage")
	}
}

func TestOfflineDeadlineRecoveryKeepsVerifiedBest(t *testing.T) {
	c, _ := candidateFixture(t)
	if err := os.WriteFile(filepath.Join(c.root, "assets/generated.bin"), []byte("accepted best"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := c.attempted()
	if err != nil {
		t.Fatal(err)
	}
	if err = c.promote(&checkFingerprint{}); err != nil {
		t.Fatal(err)
	}
	c.begin()
	os.WriteFile(filepath.Join(c.root, "assets/generated.bin"), []byte("rejected attempt"), 0600)
	work, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	c.ctx = work
	if err = restoreCandidate(c, 2*time.Second, &receiptWriter{}, 1); err != nil {
		t.Fatal(err)
	}
	deadline := c.state.Restoration.DeadlineNS
	if !c.state.Exposed || c.state.Best != id || c.state.Restoration.Phase != "verified" || c.ctx.Err() != context.DeadlineExceeded {
		t.Fatal("fixture did not restore the verified best independently of work expiry")
	}
	// executeRun performs this same validation after a rejected candidate is restored.
	if err = c.validateSnapshot(c.state.Best); err != nil {
		t.Fatalf("verified best rejected by expired work context: %v", err)
	}
	if c.state.Restoration.DeadlineNS != deadline || c.ctx.Err() != context.DeadlineExceeded {
		t.Fatal("validation renewed the recovery bound or revived work")
	}
	if err := c.validateSnapshot(c.state.Initial); err == nil {
		t.Fatal("unrelated initial snapshot used best-only recovery allowance")
	}
}

func TestOfflineLateDeadlineCleanupKeepsBest(t *testing.T) {
	cfg, w := reviewCandidateFixture(t, "pilot-cancel")
	cfg.totalTimeout, cfg.commandTimeout, cfg.recoveryTimeout = 5*time.Second, 10*time.Second, 2*time.Second
	w.Candidate.Baseline = true
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st := runForTest(t, cfg)
	executions, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if code != 40 || string(executions) != "1\n" || st.Candidate.Status != "restored" || st.Candidate.Restoration.Phase != "verified" {
		t.Fatalf("deadline fixture did not complete independent recovery: code=%d candidate=%+v", code, st.Candidate)
	}
	value, _ := os.ReadFile(filepath.Join(cfg.workdir, "candidate/assets/value"))
	if !st.Candidate.Exposed || st.BestCandidatePath == "" || string(value) != "baseline" {
		t.Fatalf("verified best hidden by expired work context: %+v best=%q", st.Candidate, st.BestCandidatePath)
	}
	for _, failure := range st.Failures {
		if strings.Contains(failure.Cause, "best_snapshot_invalid") {
			t.Fatal("expired work context mislabeled a verified snapshot as corrupt")
		}
	}
}

func TestOfflineUsageEvidenceExactBoundedAndImmutable(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := &workflowRuntime{workdir: root}
	req, _, _, _ := budgetFixture()
	raw := " { \"schema_version\": 1 }\n"
	name, err := w.preserveUsage(req, raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := w.preserveUsage(req, raw, true); err != nil || again != name {
		t.Fatalf("identical evidence cannot be safely reused: %q %v", again, err)
	}
	file := filepath.Join(w.workdir, name)
	got, err := os.ReadFile(file)
	if err != nil || string(got) != raw {
		t.Fatal("collector bytes were normalized or lost")
	}
	if err := os.WriteFile(file, []byte(strings.Repeat("x", len(raw))), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.preserveUsage(req, raw, true); err == nil {
		t.Fatal("tampered archive was silently replaced")
	}
	if got, _ := os.ReadFile(file); string(got) == raw {
		t.Fatal("historical bytes were rewritten")
	}
	tail, err := w.preserveUsage(req, raw, false)
	if err != nil || !strings.HasSuffix(tail, ".diagnostic-tail") {
		t.Fatal("incomplete capture masquerades as a complete report")
	}
	if _, err := w.preserveUsage(req, strings.Repeat("x", workflow.MaxEvidence+1), true); err == nil {
		t.Fatal("unbounded evidence accepted")
	}
}

func TestOfflineKnownUsageCanGainInformationAndSettle(t *testing.T) {
	req, p, u, policy := budgetFixture()
	b := newBudget("strict")
	if why := b.reserve(p, req, policy, 600000); why != "" {
		t.Fatal(why)
	}
	prior := u
	prior.Calls = append([]workflow.Call(nil), u.Calls...)
	prior.Calls[0].MicroUSD = money(1)
	prior.Calls[1].Tokens, prior.Calls[1].MicroUSD = nil, nil
	b.DispatchPending, b.Unsettled = true, &prior
	if why := b.reconcile(u, req, policy, 600000); why != "" {
		t.Fatal("legitimate reconciliation blocked:", why)
	}
	if b.Calls != 2 || b.MicroUSD != 600000 || b.Pending != nil || b.Unsettled != nil {
		t.Fatalf("valid incurred usage not settled: %+v", b)
	}
}

func TestOfflineCheckpointFailurePreventsDispatch(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			cfg, config := workflowFixture(t, "normal")
			dir := filepath.Join(cfg.workdir, ".loopexec", cfg.runID)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			calls := 0
			w := &workflowRuntime{config: config, dir: dir, workdir: cfg.workdir, book: newBudget("strict"), cap: 1000000,
				runner: commandRunner{ctx: context.Background(), dir: cfg.workdir, timeout: 5 * time.Second, grace: cfg.grace, receipt: &receiptWriter{runID: cfg.runID}},
				checkpoint: func() error {
					calls++
					if calls == failAt {
						return errors.New("injected checkpoint failure")
					}
					return nil
				}}
			_, reason, cause := w.paid(1, "exec", "", config.Exec)
			if reason != "internal_error" || cause != "budget_persist_failed" || w.dispatched || calls != failAt {
				t.Fatalf("dispatch passed failed checkpoint: %s %s calls=%d", reason, cause, calls)
			}
			if _, err := os.Stat(filepath.Join(cfg.workdir, "executions")); !os.IsNotExist(err) {
				t.Fatal("exec ran before durable dispatch checkpoint")
			}
		})
	}
}

func TestOfflineRecoveryValidationDoesNotRenewDeadline(t *testing.T) {
	c, _ := candidateFixture(t)
	if _, err := c.attempted(); err != nil {
		t.Fatal(err)
	}
	if err := c.promote(&checkFingerprint{}); err != nil {
		t.Fatal(err)
	}
	work, cancel := context.WithCancel(context.Background())
	c.ctx = work
	if err := restoreCandidate(c, time.Second, &receiptWriter{}, 1); err != nil {
		t.Fatal(err)
	}
	cancel()
	c.recoveryDeadline = time.Now().Add(-time.Second)
	c.state.Restoration.DeadlineNS = c.recoveryDeadline.UnixNano()
	if err := c.validateSnapshot(c.state.Best); err == nil {
		t.Fatal("final verification renewed expired recovery time")
	}
}

func TestOfflineUsageArchiveFailureKeepsExecutedCallUnsettled(t *testing.T) {
	cfg, w := workflowFixture(t, "offline-evidence-failure")
	cfg.maxIterations, cfg.commandTimeout, w.Score = 1, 5*time.Second, nil
	if err := workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w); err != nil {
		t.Fatal(err)
	}
	code, st := runForTest(t, cfg)
	executions, err := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if err != nil || string(executions) != "1\n" {
		t.Fatal("fixture did not execute one stand-in call")
	}
	if code == 0 || st.FailureCause != "usage_evidence_persist_failed" || st.Budget.Pending == nil || !st.Budget.DispatchPending || st.Budget.Calls != 0 {
		t.Fatalf("archive failure settled executed work: code=%d state=%+v", code, st)
	}
}
