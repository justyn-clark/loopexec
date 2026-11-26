package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRCPilotCorruptSnapshotDoesNotTouchAttempt(t *testing.T) {
	c, _ := candidateFixture(t)
	os.WriteFile(filepath.Join(c.root, "assets/zzz.bin"), []byte("baseline"), 0600)
	id, err := c.attempted()
	if err != nil {
		t.Fatal(err)
	}
	if err = c.promote(&checkFingerprint{ExitCode: 1}); err != nil {
		t.Fatal(err)
	}
	c.begin()
	os.WriteFile(filepath.Join(c.root, "assets/generated.bin"), []byte("valuable repair"), 0600)
	os.WriteFile(filepath.Join(c.store, id, "files/assets/zzz.bin"), []byte("corrupt"), 0600)
	if c.restore() == nil {
		t.Fatal("corrupt snapshot accepted")
	}
	data, _ := os.ReadFile(filepath.Join(c.root, "assets/generated.bin"))
	if string(data) != "valuable repair" {
		t.Fatal("rollback modified the attempt before validating the complete snapshot")
	}
}

func TestRCPilotIndependentRecoveryTimeoutFlag(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"run", "--json", "--workdir", t.TempDir(), "--check", "false", "--max-iterations", "1", "--recovery-timeout", "1s"})
	err := cmd.Execute()
	var cli *cliError
	if !errors.As(err, &cli) || cli.Code != 12 {
		t.Fatalf("independent recovery timeout unavailable: %v", err)
	}
}

func TestRCPilotReportDistinguishesUnresolvedUsage(t *testing.T) {
	cfg, _ := workflowFixture(t, "bad-usage")
	_, st := runForTest(t, cfg)
	if st.Budget.Pending == nil {
		t.Fatal("fixture must have pending usage")
	}
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"report", "--json", "--workdir", cfg.workdir, "--run-id", cfg.runID})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Report struct {
			Accounting struct {
				UnresolvedReservations int  `json:"unresolved_reservations"`
				CallsKnown             bool `json:"calls_known"`
			} `json:"accounting"`
		}
	}
	if json.Unmarshal(out.Bytes(), &result) != nil || result.Report.Accounting.UnresolvedReservations != 1 || result.Report.Accounting.CallsKnown {
		t.Fatal("pending invocation presented as settled zero without explicit unknown total")
	}
	for _, field := range []string{"total_calls", "total_tokens", "total_microusd"} {
		if !bytes.Contains(out.Bytes(), []byte(`"`+field+`":null`)) {
			t.Fatalf("unknown %s not null", field)
		}
	}
}

func TestRCPilotUnsettledObservationIsNotSettledUsage(t *testing.T) {
	req, p, usage, policy := budgetFixture()
	b := newBudget("strict")
	if reason := b.reserve(p, req, policy, 600000); reason != "" {
		t.Fatal(reason)
	}
	b.DispatchPending = true
	usage.Calls[0].Tokens = nil
	b.Unsettled = &usage
	if reason := b.reconcile(usage, req, policy, 600000); reason != "cost_anomaly" {
		t.Fatal("incomplete usage settled")
	}
	a := b.accounting()
	if a.SettledCalls != 0 || a.ReportedExecutedUnsettled != 2 || a.UnresolvedReservations != 2 || a.TotalCalls != nil || a.TotalTokens != nil || a.TotalMicroUSD != nil {
		t.Fatalf("unsettled evidence confused with zero use: %+v", a)
	}
}

func TestRCPilotProductionSizedSlowRestore(t *testing.T) {
	c, _ := candidateFixture(t)
	payload := bytes.Repeat([]byte("baseline"), 1024)
	for i := 0; i < 1387; i++ {
		if err := os.WriteFile(filepath.Join(c.root, "assets", fmt.Sprintf("unit-%04d.bin", i)), payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	id, err := c.attempted()
	if err != nil {
		t.Fatal(err)
	}
	if err = c.promote(&checkFingerprint{}); err != nil {
		t.Fatal(err)
	}
	c.begin()
	os.WriteFile(filepath.Join(c.root, "assets/generated.bin"), []byte("attempt"), 0600)
	original, cancel := context.WithCancel(context.Background())
	cancel()
	c.ctx = original
	calls := 0
	c.copyArtifact = func(ctx context.Context, from, to string, a workflow.Artifact) error {
		calls++
		if calls == 1 {
			select {
			case <-time.After(5100 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if calls%8 == 0 {
			select {
			case <-time.After(2 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return workflow.CopyArtifact(ctx, from, to, a)
	}
	start := time.Now()
	if err = restoreCandidate(c, 0, &receiptWriter{}, 1); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) <= 5*time.Second || c.state.Restored != id || !c.state.Exposed || c.state.Restoration.Phase != "verified" {
		t.Fatalf("unverified or unrealistically fast restore: %+v", c.state)
	}
	if c.state.AttemptArchive == "" || c.state.AttemptArchive == id {
		t.Fatal("failed attempt not retained separately")
	}
}

func TestRCPilotInterruptedRestoreIsNeverExposed(t *testing.T) {
	for _, phase := range []string{"staging", "applying", "timeout"} {
		t.Run(phase, func(t *testing.T) {
			c, root := candidateFixture(t)
			os.WriteFile(filepath.Join(c.root, "assets/zzz.bin"), []byte("base"), 0600)
			id, _ := c.attempted()
			c.promote(&checkFingerprint{})
			c.begin()
			os.WriteFile(filepath.Join(c.root, "assets/generated.bin"), []byte("repair"), 0600)
			count := 0
			c.copyArtifact = func(ctx context.Context, from, to string, a workflow.Artifact) error {
				inApply := to == c.root
				if (phase == "applying") == inApply {
					count++
					if count == 2 {
						if phase == "timeout" {
							<-ctx.Done()
							return ctx.Err()
						}
						return fmt.Errorf("injected I/O interruption")
					}
				}
				return workflow.CopyArtifact(ctx, from, to, a)
			}
			timeout := time.Second
			if phase == "timeout" {
				timeout = 25 * time.Millisecond
			}
			if restoreCandidate(c, timeout, &receiptWriter{}, 1) == nil {
				t.Fatal("failure not injected")
			}
			if c.state.Best != id || c.state.Initial == "" || c.state.Exposed || c.state.Status != "restoring" {
				t.Fatalf("unsafe pointers: %+v", c.state)
			}
			if phase != "applying" {
				data, _ := os.ReadFile(filepath.Join(c.root, "assets/generated.bin"))
				if string(data) != "repair" {
					t.Fatal("staging failure modified candidate")
				}
			}
			next, err := newCandidate(context.Background(), root, filepath.Join(root, "runtime"), c.policy)
			if err != nil {
				t.Fatal(err)
			}
			if err = next.restore(); err != nil || next.state.Restored != id || !next.state.Exposed {
				t.Fatalf("interrupted recovery failed: %v", err)
			}
		})
	}
}

func TestRCPilotPreservesPrimaryAndCleanupFailures(t *testing.T) {
	cfg, w := workflowFixture(t, "pilot-double-failure")
	os.MkdirAll(filepath.Join(cfg.workdir, "candidate/assets"), 0700)
	os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/value"), []byte("baseline"), 0600)
	w.Candidate = &workflow.CandidatePolicy{Root: "candidate", Include: []string{"assets"}}
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	code, st := runForTest(t, cfg)
	if code != 40 || st.FailureCause != "usage_unavailable" || len(st.Failures) != 2 || st.Failures[1].Cause != "restoration_failed" {
		t.Fatalf("primary cause lost: %d %+v", code, st.Failures)
	}
	if st.Budget.Calls != 0 || st.Budget.accounting().CallsKnown || !st.Budget.accounting().DispatchAwaitingUsage {
		t.Fatal("unresolved dispatch was presented as zero use")
	}
	if st.Candidate.AttemptArchive == "" || st.Candidate.Exposed {
		t.Fatal("attempt lost or partial restoration exposed")
	}
	data, _ := os.ReadFile(filepath.Join(cfg.workdir, "candidate/assets/value"))
	if string(data) != "unreviewed repair" {
		t.Fatal("failed restore overwrote attempt")
	}
}

func TestRCPilotCancellationRestoresWithoutAnotherDispatch(t *testing.T) {
	cfg, w := workflowFixture(t, "pilot-cancel")
	cfg.commandTimeout = 4 * time.Second
	if err := os.MkdirAll(filepath.Join(cfg.workdir, "candidate/assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/value"), []byte("baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	w.Candidate = &workflow.CandidatePolicy{Root: "candidate", Include: []string{"assets"}}
	if err := workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if _, err := os.Stat(filepath.Join(cfg.workdir, "executions")); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(ctx)
	code := exitCode(executeRun(cmd, cfg))
	<-done
	st, err := readState(statePath(cfg.workdir))
	if err != nil || code != 40 || st.Candidate == nil || st.Candidate.Status != "restored" || st.Candidate.Restoration.Phase != "verified" {
		t.Fatalf("cancelled run was not deterministically recovered: %d %+v %v", code, st.Candidate, err)
	}
	value, _ := os.ReadFile(filepath.Join(cfg.workdir, "candidate/assets/value"))
	executions, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if string(value) != "baseline" || string(executions) != "1\n" || st.Budget.accounting().CallsKnown || st.Candidate.AttemptArchive == "" {
		t.Fatal("cancelled recovery lost attempt, launched work or fabricated usage")
	}
}

func TestRCPilotResumeRecoveryDoesNotRequireWorkEligibility(t *testing.T) {
	for _, mode := range []string{"bad-usage", "bad-score"} {
		t.Run(mode, func(t *testing.T) {
			cfg, w := workflowFixture(t, mode)
			cfg.totalTimeout = time.Minute
			cfg.commandTimeout = 2 * time.Second
			os.MkdirAll(filepath.Join(cfg.workdir, "candidate/assets"), 0700)
			os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/value"), []byte("baseline"), 0600)
			w.Candidate = &workflow.CandidatePolicy{Root: "candidate", Include: []string{"assets"}}
			workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
			_, prior := runForTest(t, cfg)
			journal := *prior.Candidate
			journal.Status, journal.Exposed, journal.AttemptArchive = "restoring", false, ""
			if err := workflow.Atomic(filepath.Join(cfg.workdir, ".loopexec/flow/candidate.json"), journal); err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/value"), []byte("interrupted attempt"), 0600)
			prior.DeadlineNS = time.Now().Add(-time.Second).UnixNano()
			if err := writeStateFile(filepath.Join(cfg.workdir, ".loopexec"), perRunStateName("flow"), prior); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
			cfg.resume = true
			code, after := runForTest(t, cfg)
			executions, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
			value, _ := os.ReadFile(filepath.Join(cfg.workdir, "candidate/assets/value"))
			if code == 10 || after.Candidate.Status != "restored" || after.Candidate.Restoration.Phase != "verified" || string(before) != string(executions) || string(value) != "baseline" || after.DeadlineNS != prior.DeadlineNS {
				t.Fatalf("recovery renewed work or left partial tree: %d %+v", code, after.Candidate)
			}
			if mode == "bad-usage" && (after.Budget.Pending == nil || after.Budget.accounting().CallsKnown) {
				t.Fatal("recovery waived unresolved accounting")
			}
			archive, err := os.ReadFile(filepath.Join(cfg.workdir, ".loopexec/flow/candidates", after.Candidate.AttemptArchive, "files/assets/value"))
			if err != nil || string(archive) != "interrupted attempt" {
				t.Fatal("resume recovery lost the interrupted attempt")
			}
		})
	}
}

func TestRCPilotResumeMissingJournalCannotRecapturePartialInitial(t *testing.T) {
	cfg, w := workflowFixture(t, "bad-score")
	cfg.commandTimeout = 2 * time.Second
	os.MkdirAll(filepath.Join(cfg.workdir, "candidate/assets"), 0700)
	os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/value"), []byte("baseline"), 0600)
	w.Candidate = &workflow.CandidatePolicy{Root: "candidate", Include: []string{"assets"}}
	workflow.Atomic(filepath.Join(cfg.workdir, "workflow.json"), w)
	_, prior := runForTest(t, cfg)
	journal := filepath.Join(cfg.workdir, ".loopexec/flow/candidate.json")
	if err := os.Rename(journal, journal+".retained"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(cfg.workdir, "candidate/assets/value"), []byte("partial"), 0600)
	before, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	cfg.resume = true
	code, after := runForTest(t, cfg)
	executions, _ := os.ReadFile(filepath.Join(cfg.workdir, "executions"))
	if code != 13 || after.Candidate.Initial != prior.Candidate.Initial || after.Candidate.Exposed || string(before) != string(executions) {
		t.Fatal("missing journal was silently recaptured")
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatal("missing authority was recreated")
	}
}
