package main

import (
	"context"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func executeRun(cmd *cobra.Command, cfg runConfig) error {
	if cfg.maxIterations < 1 || cfg.budgetUSD < 0 || !finite(cfg.budgetUSD) || !finite(cfg.costUSD) || cfg.costUSD < 0 {
		return failResponse(cmd, cfg.runID, exitInvariantFailed, "invariant_failed", "invalid iteration or monetary bound")
	}
	if cfg.totalTimeout < 0 || cfg.commandTimeout < 0 || cfg.grace < 0 || !validRunID(cfg.runID) {
		return failResponse(cmd, cfg.runID, exitWorkspaceInvalid, "workspace_invalid", "invalid duration or run-id")
	}
	if cfg.workdir == "" {
		cfg.workdir = "."
	}
	abs, e := filepath.Abs(cfg.workdir)
	if e != nil {
		return e
	}
	if canonical, err := filepath.EvalSymlinks(abs); err == nil {
		abs = canonical
	}
	cfg.workdir = abs

	w, hash, e := loadWorkflow(cfg)
	if e != nil {
		return failResponse(cmd, cfg.runID, 30, "workspace_invalid", e.Error())
	}
	if strings.TrimSpace(cfg.check) == "" && w == nil {
		return failResponse(cmd, cfg.runID, 30, "workspace_invalid", "a loop requires an external check (--check). no check, no loop")
	}
	dir := filepath.Join(cfg.workdir, ".loopexec")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return failResponse(cmd, cfg.runID, 30, "workspace_invalid", "cannot create state directory")
	}
	if _, err := workflow.SafePath(cfg.workdir, ".loopexec"); err != nil {
		return failResponse(cmd, cfg.runID, 30, "workspace_invalid", "unsafe runtime directory")
	}
	unlock, err := workflow.Lock(dir)
	if err != nil {
		return failResponse(cmd, cfg.runID, 30, "workspace_invalid", err.Error())
	}
	defer unlock()
	var prior loopState
	if w != nil {
		if e = ensureNewRun(dir, cfg.runID, cfg.resume); e != nil {
			return failResponse(cmd, cfg.runID, 30, "workspace_invalid", e.Error())
		}
		if cfg.resume {
			prior, e = readState(filepath.Join(dir, perRunStateName(cfg.runID)))
			if e != nil || prior.RunPolicyHash != runPolicyHash(cfg, hash) {
				return failResponse(cmd, cfg.runID, 30, "workspace_invalid", "resume policy changed or state unreadable")
			}
		}
	}
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	deadline := prior.DeadlineNS
	if cfg.totalTimeout > 0 {
		if deadline == 0 {
			deadline = time.Now().Add(cfg.totalTimeout).UnixNano()
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.Unix(0, deadline))
		defer cancel()
	}
	receiptPath := filepath.Join(dir, "run-"+cfg.runID+".jsonl")
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if cfg.resume {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, e := os.OpenFile(receiptPath, flags, 0600)
	if e != nil {
		return failResponse(cmd, cfg.runID, 30, "workspace_invalid", "cannot open receipt")
	}
	defer f.Close()
	rw := &receiptWriter{f: f, runID: cfg.runID}
	runner := commandRunner{ctx: ctx, dir: cfg.workdir, timeout: cfg.commandTimeout, grace: cfg.grace, receipt: rw}
	st := loopState{SchemaVersion: 1, RunID: cfg.runID, Phase: "running", DeadlineNS: deadline, Workflow: w, WorkflowHash: hash, RunPolicyHash: runPolicyHash(cfg, hash), CommandTimeoutNS: int64(cfg.commandTimeout), GraceNS: int64(cfg.grace)}
	start := 1
	if cfg.resume {
		st = prior
		start = prior.Iteration + 1
		st.Phase = "running"
		st.HaltReason = ""
		st.FailureCause = ""
	}
	st.BestCandidatePath = ""
	if st.Candidate != nil {
		v := *st.Candidate
		v.Exposed = false
		st.Candidate = &v
	}
	st.Check = cfg.check
	st.Exec = cfg.execCmd
	st.Workdir = cfg.workdir
	st.MaxIterations = cfg.maxIterations
	st.FailuresCmd = cfg.failuresCmd
	st.IntegrityCmd = cfg.integrityCmd
	st.NoProgressK = cfg.noProgressK
	st.CostUSD = cfg.costUSD
	if cfg.modelID != "" {
		st.Model = &modelPin{Provider: cfg.modelProvider, ID: cfg.modelID, Version: cfg.modelVersion}
		st.Sampling = &samplingPin{Temperature: cfg.temperature, Seed: cfg.seed, MaxTokens: cfg.maxTokens}
	}
	st.ContextManifest = buildManifest(cfg.workdir, cfg.contextFiles)
	var ext *workflowRuntime
	var candidate *candidateManager
	var tracker *progressTracker
	if cfg.failuresCmd != "" {
		tracker = newProgressTracker(cfg.noProgressK)
	}
	halt := ""

	persist := func() error {
		if rw.err != nil {
			return rw.err
		}
		if ext != nil {
			st.Budget = ext.book
			st.CumulativeUSD = float64(ext.book.MicroUSD) / 1e6
		}
		if candidate != nil {
			v := candidate.state
			st.Candidate = &v
		}
		if e := writeStateFile(dir, perRunStateName(cfg.runID), st); e != nil {
			return e
		}
		return writeStateAtomic(dir, st)
	}
	fail := func(reason, cause string) { halt = reason; st.FailureCause = cause }
	rw.emit(st.Iteration, "run_start", fmt.Sprintf("max=%d resume=%t", cfg.maxIterations, cfg.resume), nil)
	if cfg.resume && prior.Candidate != nil {
		for _, pin := range prior.Candidate.Protected {
			path, err := workflow.SafePath(cfg.workdir, pin.Path)
			if err != nil {
				fail("metric_integrity_violation", "protected_policy_drift")
				break
			}
			hash, _, err := workflow.FileHash(ctx, path)
			if err != nil || hash != pin.SHA256 {
				fail("metric_integrity_violation", "protected_policy_drift")
				break
			}
		}
	}

	if w != nil {
		owned := filepath.Join(dir, cfg.runID)
		if e = os.MkdirAll(owned, 0700); e != nil {
			fail("workspace_invalid", "runtime_directory_failed")
		} else {
			book, err := loadBudget(filepath.Join(owned, "budget.json"), w.Meter.Mode)
			if err != nil {
				fail("cost_anomaly", "saved_usage_invalid")
			} else {
				cap, _ := microUSD(cfg.budgetUSD)
				ext = &workflowRuntime{config: w, hash: hash, dir: owned, workdir: cfg.workdir, book: book, cap: cap, runner: runner}
				if book.Pending != nil && halt == "" {
					if !cfg.resume {
						fail("cost_anomaly", "unresolved_reservation")
					} else if r, c := ext.restorePending(); r != "" {
						fail(r, c)
					}
				}
			}
			if halt == "" && w.Candidate != nil {
				candidate, e = newCandidate(ctx, cfg.workdir, owned, *w.Candidate)
				if e != nil {
					fail("objective_unverified", "candidate_initialization_failed")
				} else if cfg.resume && candidate.state.Status != "accepted" && candidate.state.Status != "restored" && candidate.state.Status != "initial" {
					if e = candidate.restore(); e != nil {
						fail("execution_failure", "restoration_failed")
					}
				}
			}
			if halt == "" && cfg.resume && candidate != nil && candidate.state.Exposed {
				current, err := workflow.Capture(ctx, candidate.root, candidate.policy)
				if err != nil || workflow.JSONHash(current) != candidate.state.Best {
					if err = candidate.restore(); err != nil {
						fail("execution_failure", "restoration_failed")
					}
				}
			}
			if cfg.resume && candidate != nil && candidate.state.Numeric != nil && candidate.state.Status == "accepted" && prior.Phase == "attempting" {
				v := *candidate.state.Numeric
				st.Numeric = &v
			}
			if w.Score != nil && st.Numeric == nil {
				st.Numeric = &numericState{}
			}
		}
	}
	if e = persist(); e != nil {
		fail("internal_error", "state_write_failed")
	}
	integrity := map[string]struct{}(nil)
	if halt == "" && cfg.integrityCmd != "" {
		var order []string
		integrity, order, e = collectorSet(runner.run(0, "integrity", cfg.integrityCmd))
		if e != nil {
			fail("metric_integrity_violation", "integrity_collector_failed")
		} else if cfg.resume && workflow.JSONHash(order) != workflow.JSONHash(st.IntegrityBaseline) {
			fail("metric_integrity_violation", "integrity_baseline_drift")
		} else {
			st.IntegrityBaseline = order
			rw.emit(0, "integrity_baseline", "", nil)
		}
	}
	if halt == "" && candidate != nil && w.Candidate.Baseline && candidate.state.Best == "" {
		if e = verifyBaseline(ext, candidate, &st, cfg.runID); e != nil {
			fail("objective_unverified", "baseline_invalid")
		}
	}
	if halt == "" && cfg.resume && prior.HaltReason == "success_condition_met" {
		halt = prior.HaltReason
	}
	for i := start; i <= cfg.maxIterations && halt == ""; i++ {
		if ctx.Err() != nil {
			fail("execution_failure", causeOfContext(ctx))
			break
		}
		st.Iteration = i
		st.Phase = "attempting"
		if w != nil {
			st.Fingerprint = nil
		}
		rw.emit(i, "iter_start", "", nil)
		writeHeartbeat(dir, i, "iter_start")
		if candidate != nil {
			if e = candidate.intact(cfg.workdir); e != nil {
				fail("metric_integrity_violation", "protected_policy_drift")
				break
			}
			if e = candidate.begin(); e != nil {
				fail("execution_failure", "checkpoint_begin_failed")
				break
			}
		}
		if e = persist(); e != nil {
			fail("internal_error", "state_write_failed")
			break
		}
		if cfg.execCmd != "" || (w != nil && len(w.Exec) > 0) {
			var rc int
			if ext != nil {
				v, r, c := ext.paid(i, "exec", cfg.execCmd, w.Exec)
				rc = v.ExitCode
				if r != "" {
					fail(r, c)
					break
				}
				if v.Cause != "" {
					fail("execution_failure", v.Cause)
					break
				}
			} else {
				v := runner.run(i, "exec", cfg.execCmd)
				rc = v.ExitCode
				if v.Cause != "" {
					fail("execution_failure", v.Cause)
					break
				}
			}
			rw.emit(i, "exec", "", &rc)
			if rc != 0 {
				fail("execution_failure", "work_command_failed")
				break
			}
		}
		if candidate != nil && candidate.intact(cfg.workdir) != nil {
			fail("metric_integrity_violation", "protected_policy_drift")
			break
		}
		if integrity != nil {
			cur, _, err := collectorSet(runner.run(i, "integrity", cfg.integrityCmd))
			if err != nil {
				fail("metric_integrity_violation", "integrity_collector_failed")
				break
			}
			if len(missingMembers(integrity, cur)) > 0 {
				fail("metric_integrity_violation", "integrity_members_removed")
				rw.emit(i, "integrity_violation", "", nil)
				break
			}
		}
		id := fmt.Sprintf("%s/%06d", cfg.runID, i)
		if candidate != nil {
			id, e = candidate.attempted()
			if e != nil {
				fail("objective_unverified", "candidate_snapshot_failed")
				break
			}
			rw.emit(i, "candidate_attempted", id, nil)
		}
		var rc int
		var checkOut string
		if ext != nil {
			// Verification hooks are local read-only operations. Explicitly priced
			// engine/tool work belongs in exec's individual call reservations.
			v := ext.hook(ext.request(i, "check", id), "check", w.Check)
			rc, checkOut = v.ExitCode, v.Stdout
			if v.Cause != "" || v.Truncated {
				cause := v.Cause
				if cause == "" {
					cause = "output_oversized"
				}
				fail("execution_failure", "check_"+cause)
				break
			}
		} else {
			v := runner.run(i, "check", cfg.check)
			rc, checkOut = v.ExitCode, v.Combined
			if v.Cause != "" || v.Truncated {
				cause := v.Cause
				if cause == "" {
					cause = "check_output_oversized"
				}
				fail("execution_failure", cause)
				break
			}
		}
		st.LastCheckExit = &rc
		st.Fingerprint = &checkFingerprint{ExitCode: rc, OutputSHA256: sha256hex([]byte(normalizeOutput(checkOut)))}
		rw.emit(i, "check", "", &rc)
		var F map[string]struct{}
		var order []string
		if cfg.failuresCmd != "" {
			F, order, e = collectorSet(runner.run(i, "failures", cfg.failuresCmd))
			if e != nil {
				fail("objective_unverified", "failures_collector_failed")
				break
			}
			size := len(F)
			rw.emit(i, "progress", sha256hex([]byte(setHash(order))), &size)
		}
		technical, accept, target := rc == 0, rc == 0, rc == 0
		progressHalt := ""
		var currentReviewedScore *float64
		if w != nil && w.Score != nil {
			var sc workflow.Score
			v := ext.hook(ext.request(i, "score", id), "score", w.Score.Command)
			if structuredResult(v, &sc) != nil || sc.SchemaVersion != 1 || sc.RunID != cfg.runID || sc.Iteration != i || sc.CandidateID != id || (rc == 0 && !sc.Technical) {
				fail("objective_unverified", "score_evidence_invalid")
				break
			}
			technical = sc.Technical
			currentReviewedScore = sc.Value
			accept, target, progressHalt = st.Numeric.observe(w.Score, sc)
			if progressHalt == "objective_unverified" {
				fail(progressHalt, "score_evidence_invalid")
				break
			}
			if candidate == nil && st.Numeric.Best != nil && (!technical || !accept) {
				progressHalt = "same_test_regressed"
			}
		} else if tracker != nil && rc != 0 {
			progressHalt = tracker.observe(i, F, order)
			best, initial := tracker.bestSize, tracker.initialSize
			st.BestFailCount = &best
			st.InitialFailCount = &initial
			st.BestIteration = tracker.bestIter
			st.EverImproved = tracker.everImproved
		}
		if candidate != nil {
			if candidate.intact(cfg.workdir) != nil {
				fail("metric_integrity_violation", "protected_policy_drift")
				break
			}
			cur, err := workflow.Capture(ctx, candidate.root, candidate.policy)
			if err != nil || workflow.JSONHash(cur) != id {
				fail("objective_unverified", "candidate_changed_during_verification")
				break
			}
			if technical && accept {
				weaker := false
				if w.Score != nil && candidate.state.Numeric != nil && candidate.state.Numeric.Best != nil && st.Numeric != nil && st.Numeric.Best != nil {
					// observe retains the strict best. Compare the reviewed value with the
					// candidate journal's best by reading the current score result once below.
					weaker = currentReviewedScore != nil && ((w.Score.Direction == "maximize" && *currentReviewedScore < *candidate.state.Numeric.Best) || (w.Score.Direction == "minimize" && *currentReviewedScore > *candidate.state.Numeric.Best))
				}
				if weaker {
					candidate.state.Accepted = id
					if e = candidate.restore(); e != nil {
						fail("execution_failure", "restoration_failed")
						break
					}
					rw.emit(i, "candidate_restored", candidate.state.Restored, nil)
				} else {

					candidate.iteration = i
					if e = candidate.promote(st.Fingerprint, st.Numeric); e != nil {
						if candidate.state.Numeric != nil {
							v := *candidate.state.Numeric
							st.Numeric = &v
						}
						fail("execution_failure", "promotion_failed")

						break
					}
					rw.emit(i, "candidate_accepted", id, nil)
				}
			} else {
				rw.emit(i, "candidate_rejected", id, nil)
				if e = candidate.restore(); e != nil {
					fail("execution_failure", "restoration_failed")
					break
				}
				rw.emit(i, "candidate_restored", candidate.state.Restored, nil)
			}
		}
		if ctx.Err() != nil {
			fail("execution_failure", causeOfContext(ctx))
			break
		}
		if rc == 0 && technical && accept && target {
			halt = "success_condition_met"
		} else if progressHalt != "" {
			halt = progressHalt
		}
		st.Phase = "evaluated"
		if e = persist(); e != nil {
			fail("internal_error", "state_write_failed")
			break
		}
		if halt != "" {
			break
		}
		st.DiffsMergedUnread++
		if cfg.comprehensionEvery > 0 && st.DiffsMergedUnread >= cfg.comprehensionEvery {
			halt = "comprehension_debt_exceeded"
		}
	}
	if halt == "" {
		halt = "max_iterations_reached"
	}
	if candidate != nil && !candidate.state.Exposed && candidate.state.Status != "restored" && st.FailureCause != "restoration_failed" {
		cleanup, cancel := context.WithTimeout(context.Background(), max(cfg.grace, 250*time.Millisecond))
		candidate.ctx = cleanup
		if e = candidate.restore(); e != nil {
			fail("execution_failure", "restoration_failed")
		} else {
			rw.emit(st.Iteration, "candidate_restored", candidate.state.Restored, nil)
		}
		cancel()
	}
	st.HaltReason = halt
	st.Phase = "halted"
	if candidate != nil && candidate.state.Best != "" && candidate.state.Exposed {
		st.BestCandidatePath = filepath.Join(candidate.store, candidate.state.Best, "files")
	}
	rw.emit(st.Iteration, "halt", halt, nil)
	if e = persist(); e != nil {
		return failResponse(cmd, cfg.runID, 50, "internal_error", "cannot persist final state")
	}
	if e = f.Sync(); e != nil {
		return failResponse(cmd, cfg.runID, 50, "internal_error", "cannot flush receipt")
	}
	r := response{Tool: toolName, Version: toolVersion, Status: "halted", RunID: cfg.runID, Iteration: st.Iteration, HaltReason: halt, CheckExit: st.LastCheckExit, Receipt: receiptPath, Errors: []string{}, FailureCause: st.FailureCause, BestCandidate: st.BestCandidatePath}
	if halt == "execution_failure" {
		r.Status = "error"
		r.Errors = []string{"execution failure: " + st.FailureCause}
	}
	if e = printResponse(cmd, r); e != nil {
		return e
	}
	return &cliError{Code: haltExitCode(halt), Message: "halted: " + halt, Silent: true}
}
func verifyBaseline(ext *workflowRuntime, c *candidateManager, st *loopState, runID string) error {
	id, e := c.attempted()
	if e != nil {
		return e
	}
	v := ext.hook(ext.request(0, "check", id), "baseline.check", ext.config.Check)
	if v.Cause != "" || v.Truncated {
		return fmt.Errorf("baseline check unavailable")
	}
	accept := v.ExitCode == 0
	if p := ext.config.Score; p != nil {
		var sc workflow.Score
		r := ext.hook(ext.request(0, "score", id), "baseline.score", p.Command)
		if structuredResult(r, &sc) != nil || sc.SchemaVersion != 1 || sc.RunID != runID || sc.Iteration != 0 || sc.CandidateID != id {
			return fmt.Errorf("baseline score invalid")
		}
		var halt string
		accept, _, halt = st.Numeric.observe(p, sc)
		if halt != "" {
			return fmt.Errorf("baseline score invalid")
		}
		accept = accept && sc.Technical
	}
	if !accept {
		return nil
	}
	fp := &checkFingerprint{v.ExitCode, sha256hex([]byte(normalizeOutput(v.Stdout)))}
	return c.promote(fp, st.Numeric)
}
