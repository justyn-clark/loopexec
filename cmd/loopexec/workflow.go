package main

import (
	"fmt"
	"github.com/justyn-clark/loopexec/internal/subprocess"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"strings"
)

type workflowRuntime struct {
	config             *workflow.Config
	hash, dir, workdir string
	book               *budgetBook
	cap                int64
	runner             commandRunner
}

func loadWorkflow(cfg runConfig) (*workflow.Config, string, error) {
	if cfg.workflowFile == "" {
		if cfg.budgetUSD > 0 {
			return nil, "", fmt.Errorf("--budget-usd requires --workflow metering; choose strict or explicitly observed mode")
		}
		if cfg.resume {
			return nil, "", fmt.Errorf("--resume requires --workflow")
		}
		return nil, "", nil
	}
	p := cfg.workflowFile
	if !filepath.IsAbs(p) {
		p = filepath.Join(cfg.workdir, p)
	}
	var w workflow.Config
	if e := workflow.Read(p, &w); e != nil {
		return nil, "", e
	}
	if w.SchemaVersion != 1 {
		return nil, "", fmt.Errorf("unsupported workflow schema")
	}
	if len(w.Check) == 0 {
		return nil, "", fmt.Errorf("workflow requires an argv check")
	}
	if e := validScorePolicy(w.Score); e != nil {
		return nil, "", e
	}
	if w.Candidate != nil {
		if e := workflow.ValidateManifestPolicy(*w.Candidate); e != nil {
			return nil, "", e
		}
	}
	m := w.Meter
	if m.Mode != "strict" && m.Mode != "observed" && m.Mode != "unmetered" {
		return nil, "", fmt.Errorf("metering mode must be strict, observed, or unmetered")
	}
	if len(m.Usage) == 0 || m.MaxCalls < 0 || m.MaxTokens < 0 || !finite(m.Sigma) || m.Sigma < 0 {
		return nil, "", fmt.Errorf("metering requires a valid usage hook and limits")
	}
	if (m.Mode == "strict" || m.MaxCalls > 0 || m.MaxTokens > 0) && len(m.Reserve) == 0 {
		return nil, "", fmt.Errorf("strict/call/token limits require reservations")
	}
	if m.Mode == "strict" && cfg.budgetUSD <= 0 {
		return nil, "", fmt.Errorf("strict mode requires --budget-usd")
	}
	if m.Mode == "unmetered" && cfg.budgetUSD > 0 {
		return nil, "", fmt.Errorf("unmetered money is unknown; --budget-usd is invalid")
	}
	if _, e := microUSD(cfg.budgetUSD); e != nil {
		return nil, "", e
	}
	return &w, workflow.JSONHash(w), nil
}
func runPolicyHash(cfg runConfig, hash string) string {
	return workflow.JSONHash([]any{hash, cfg.check, cfg.execCmd, cfg.failuresCmd, cfg.integrityCmd, cfg.maxIterations, cfg.noProgressK, cfg.budgetUSD, cfg.totalTimeout, cfg.commandTimeout, cfg.grace, cfg.comprehensionEvery})
}
func (r commandRunner) argv(iter int, phase string, argv []string) subprocess.Result {
	r.receipt.command(iter, "command_start", phase, subprocess.Result{})
	v := subprocess.Run(r.ctx, subprocess.Options{Dir: r.dir, Argv: argv, Timeout: r.timeout, Grace: r.grace})
	event := "command_end"
	if v.Cause == "deadline_exceeded" || v.Cause == "pipe_timeout" {
		event = "command_timeout"
	}
	if v.Cause == "cancelled" {
		event = "command_cancelled"
	}
	r.receipt.command(iter, event, phase, v)
	return v
}
func (w *workflowRuntime) request(iter int, phase, id string) workflow.Request {
	return workflow.Request{SchemaVersion: 1, RunID: w.runner.receipt.runID, Iteration: iter, Phase: phase, ConfigHash: w.hash, CandidateID: id}
}
func (w *workflowRuntime) hook(req workflow.Request, label string, argv []string) subprocess.Result {
	if w.runner.ctx.Err() != nil {
		return subprocess.Result{ExitCode: -1, Cause: causeOfContext(w.runner.ctx)}
	}
	path := filepath.Join(w.dir, fmt.Sprintf("request-%06d-%s.json", req.Iteration, label))
	if e := workflow.Atomic(path, req); e != nil {
		return subprocess.Result{ExitCode: -1, Cause: "request_write_failed"}
	}
	args := append(append([]string{}, argv...), path)
	return w.runner.argv(req.Iteration, label, args)
}
func structuredResult(v subprocess.Result, target any) error {
	if v.ExitCode != 0 || v.Cause != "" || v.Truncated || strings.TrimSpace(v.Stderr) != "" {
		return fmt.Errorf("collector_failed")
	}
	return workflow.Decode([]byte(v.Stdout), target)
}
func (w *workflowRuntime) saveBudget() error {
	return workflow.Atomic(filepath.Join(w.dir, "budget.json"), w.book)
}
func (w *workflowRuntime) reconcile(req workflow.Request) (string, string) {
	var usage workflow.Usage
	v := w.hook(req, "usage."+req.Phase, w.config.Meter.Usage)
	if e := structuredResult(v, &usage); e != nil {
		return "cost_anomaly", "usage_unavailable"
	}
	reason := w.book.reconcile(usage, req, w.config.Meter, w.cap)
	if e := w.saveBudget(); e != nil {
		return "internal_error", "budget_persist_failed"
	}
	return reason, "usage_invalid_or_limit"
}
func (w *workflowRuntime) paid(iter int, phase, legacy string, argv []string) (subprocess.Result, string, string) {
	req := w.request(iter, phase, "")
	p := w.config.Meter
	if len(p.Reserve) > 0 {
		var reservation workflow.Preflight
		v := w.hook(req, "reserve."+phase, p.Reserve)
		if e := structuredResult(v, &reservation); e != nil {
			return v, "cost_anomaly", "reservation_unavailable"
		}
		if reason := w.book.reserve(reservation, req, p, w.cap); reason != "" {
			return v, reason, "reservation_denied"
		}
		req.Reservations = reservation.Calls
	} else {
		if w.cap > 0 && w.book.MicroUSD >= w.cap {
			return subprocess.Result{}, "budget_exceeded", "observed_allowance_exhausted"
		}
		w.book.Pending = &workflow.Preflight{SchemaVersion: 1, RunID: req.RunID, Iteration: iter, Phase: phase}
	}
	if e := w.saveBudget(); e != nil {
		return subprocess.Result{}, "internal_error", "budget_persist_failed"
	}
	var v subprocess.Result
	if len(argv) > 0 {
		v = w.hook(req, phase, argv)
	} else {
		v = w.runner.run(iter, phase, legacy)
	}
	if w.runner.ctx.Err() != nil {
		return v, "execution_failure", causeOfContext(w.runner.ctx)
	}
	reason, cause := w.reconcile(req)
	return v, reason, cause
}
func (w *workflowRuntime) restorePending() (string, string) {
	if w.book.Pending == nil {
		return "", ""
	}
	p := w.book.Pending
	req := w.request(p.Iteration, p.Phase, "")
	req.Reservations = p.Calls
	return w.reconcile(req)
}
func ensureNewRun(dir, runID string, resume bool) error {
	_, e := os.Stat(filepath.Join(dir, perRunStateName(runID)))
	if resume {
		if e != nil {
			return fmt.Errorf("resume state missing")
		}
		return nil
	}
	if e == nil {
		return fmt.Errorf("workflow run-id exists; use --resume or a fresh run-id")
	}
	if !os.IsNotExist(e) {
		return e
	}
	return nil
}
func validRunID(id string) bool { return runIDRe.MatchString(id) && id != "." && id != ".." }
