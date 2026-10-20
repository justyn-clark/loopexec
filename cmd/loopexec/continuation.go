package main

import (
	"context"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"path/filepath"
	"strings"
)

func continuationParent(ctx context.Context, cfg runConfig, w *workflow.Config) (*loopState, func(), error) {
	noop := func() {}
	if w == nil || w.Continuation == nil {
		return nil, noop, nil
	}
	p := w.Continuation
	if p.Workdir == "" || filepath.IsAbs(p.Workdir) || !validRunID(p.RunID) || p.RunID == cfg.runID || !validCandidateID(p.StateSHA256) || !validCandidateID(p.ReceiptSHA256) {
		return nil, noop, fmt.Errorf("invalid continuation link")
	}
	root, err := filepath.Abs(filepath.Join(cfg.workdir, p.Workdir))
	if err != nil {
		return nil, noop, fmt.Errorf("invalid continuation root")
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil || real != root {
		return nil, noop, fmt.Errorf("unsafe continuation root")
	}
	for _, rel := range []string{".loopexec", filepath.Join(".loopexec", p.RunID), filepath.Join(".loopexec", perRunStateName(p.RunID)), filepath.Join(".loopexec", "run-"+p.RunID+".jsonl")} {
		if _, err = workflow.SafePath(root, rel); err != nil {
			return nil, noop, fmt.Errorf("unsafe continuation evidence path")
		}
	}
	unlock := noop
	if root != cfg.workdir {
		unlock, err = workflow.Lock(filepath.Join(root, ".loopexec"))
		if err != nil {
			return nil, noop, fmt.Errorf("continuation parent is active or unavailable")
		}
	}
	fail := func(message string) (*loopState, func(), error) {
		unlock()
		return nil, noop, fmt.Errorf("%s", message)
	}
	statePath := filepath.Join(root, ".loopexec", perRunStateName(p.RunID))
	for _, item := range []struct{ path, hash string }{{statePath, p.StateSHA256}, {filepath.Join(root, ".loopexec", "run-"+p.RunID+".jsonl"), p.ReceiptSHA256}} {
		sum, _, err := workflow.FileHash(ctx, item.path)
		if err != nil || sum != item.hash {
			return fail("continuation evidence changed")
		}
	}
	prior, err := readState(statePath)
	if err != nil || prior.RunID != p.RunID || prior.Workflow == nil || prior.WorkflowHash != workflow.JSONHash(prior.Workflow) || prior.Phase != "halted" || prior.Budget == nil {
		return fail("continuation parent is not a completed bounded contract")
	}
	book, err := loadResumeBudget(filepath.Join(root, ".loopexec", p.RunID, "budget.json"), prior.Budget.Mode, prior.Budget)
	if err != nil || workflow.JSONHash(book) != workflow.JSONHash(prior.Budget) || book.Pending != nil || !book.accounting().TokensKnown {
		return fail("continuation has unresolved accounting")
	}
	if prior.Candidate != nil && prior.Candidate.Status != "accepted" && prior.Candidate.Status != "restored" && prior.Candidate.Status != "initial" {
		return fail("continuation has unverified restoration")
	}
	if prior.Candidate != nil {
		var journal candidateState
		if err := workflow.Read(filepath.Join(root, ".loopexec", p.RunID, "candidate.json"), &journal); err != nil || workflow.JSONHash(journal) != workflow.JSONHash(prior.Candidate) {
			return fail("continuation restoration journal changed")
		}
	}
	if cfg.maxIterations > prior.MaxIterations || w.Meter.Mode != prior.Workflow.Meter.Mode {
		return fail("continuation cannot renew iteration or metering bounds")
	}
	old := prior.Workflow.Meter
	if old.MaxCalls > 0 && (w.Meter.MaxCalls == 0 || w.Meter.MaxCalls > old.MaxCalls) || old.MaxTokens > 0 && (w.Meter.MaxTokens == 0 || w.Meter.MaxTokens > old.MaxTokens) {
		return fail("continuation cannot renew call or token bounds")
	}
	cap, err := microUSD(cfg.budgetUSD)
	if err != nil || old.Mode == "strict" && prior.BudgetLimitMicroUSD <= 0 || prior.BudgetLimitMicroUSD > 0 && (cap <= 0 || cap > prior.BudgetLimitMicroUSD) || old.Mode == "observed" && prior.BudgetLimitMicroUSD == 0 && prior.RuntimeVersion == "" {
		return fail("continuation cannot renew monetary bound")
	}
	for _, id := range prior.Lineage {
		if id == cfg.runID {
			return fail("continuation cycle")
		}
	}
	if strings.TrimSpace(prior.RunPolicyHash) == "" {
		return fail("continuation parent has no pinned contract")
	}
	return &prior, unlock, nil
}
