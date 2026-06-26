package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	toolName    = "loopexec"
	toolVersion = "0.2.0-rc1"
)

// Exit-code classes are the coarse CI-branch buckets defined in SPEC.md §5.
// The halt_reason string is the stable contract; the exit code is its class.
// Existing codes (0/10/11/12/20/30/40/50) are preserved; 13-19 are reserved.
const (
	exitSuccess           = 0  // nominal: loop ran, no halt
	exitConverged         = 10 // success_condition_met
	exitTerminalBlocked   = 11 // no_actionable_tasks, human_required
	exitIterationCap      = 12 // max_iterations_reached
	exitIntegrity         = 13 // blocked_path_modified, reward_hacking_detected, metric_integrity_violation, ...
	exitOracleUntrusted   = 14 // check_flaky, check_not_hermetic, hermeticity_violation, ...
	exitCheckInadequate   = 15 // check_inadequate
	exitResumableJudgment = 16 // escalation_pending, reviewer_rejected
	exitNoConvergence     = 17 // no_progress_detected, oscillation_detected, infeasible_suspected, ...
	exitBudget            = 18 // budget_exceeded, cost_anomaly
	exitLivenessDrift     = 19 // heartbeat_stale, model_drift_detected, comprehension_debt_exceeded, ...
	exitInvariantFailed   = 20 // invariant_failed
	exitWorkspaceInvalid  = 30 // workspace_invalid, isolation_unsatisfiable
	exitExecutionFailure  = 40 // execution_failure
	exitInternalError     = 50 // internal_error
)

// haltExitCode maps a canonical halt_reason string to its exit-code class
// (SPEC.md §5). This is the single place the mapping lives.
func haltExitCode(reason string) int {
	switch reason {
	case "success_condition_met":
		return exitConverged
	case "no_actionable_tasks", "human_required":
		return exitTerminalBlocked
	case "max_iterations_reached":
		return exitIterationCap
	case "blocked_path_modified", "reward_hacking_detected", "metric_integrity_violation",
		"credential_scope_invalid", "objective_unverified":
		return exitIntegrity
	case "check_flaky", "check_has_side_effects", "check_not_hermetic", "hermeticity_violation":
		return exitOracleUntrusted
	case "check_inadequate":
		return exitCheckInadequate
	case "escalation_pending", "reviewer_rejected":
		return exitResumableJudgment
	case "no_progress_detected", "same_failure_repeated", "oscillation_detected",
		"same_test_regressed", "unsatisfiable_constraints", "infeasible_suspected":
		return exitNoConvergence
	case "budget_exceeded", "cost_anomaly":
		return exitBudget
	case "heartbeat_stale", "model_drift_detected", "comprehension_debt_exceeded",
		"context_budget_unsatisfiable":
		return exitLivenessDrift
	case "workspace_invalid", "isolation_unsatisfiable":
		return exitWorkspaceInvalid
	case "execution_failure":
		return exitExecutionFailure
	default:
		return exitInternalError
	}
}

type cliError struct {
	Code    int
	Message string
	Cause   error
}

func (e *cliError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *cliError) Unwrap() error {
	return e.Cause
}

type response struct {
	Tool       string   `json:"tool"`
	Version    string   `json:"version"`
	Status     string   `json:"status"`
	RunID      string   `json:"run_id,omitempty"`
	Iteration  int      `json:"iteration,omitempty"`
	HaltReason string   `json:"halt_reason,omitempty"`
	CheckExit  *int     `json:"check_exit,omitempty"`
	Receipt    string   `json:"receipt,omitempty"`
	Errors     []string `json:"errors"`
}

var jsonOutput bool

// nowFunc and runShell are seams so the loop is testable without a clock or a
// real subprocess. The defaults are the production implementations.
var nowFunc = time.Now

var runShell = func(workdir, command string) (int, string) {
	c := exec.Command("sh", "-c", command)
	if workdir != "" {
		c.Dir = workdir
	}
	out, err := c.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), string(out)
	}
	// Command could not be started (e.g. not found): -1 signals "no verdict".
	return -1, err.Error()
}

func printResponse(cmd *cobra.Command, r response) error {
	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		return enc.Encode(r)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", r.Tool, r.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", r.Status)
	if r.RunID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "run_id: %s\n", r.RunID)
	}
	if r.Iteration > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "iteration: %d\n", r.Iteration)
	}
	if r.HaltReason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "halt_reason: %s\n", r.HaltReason)
	}
	if r.Receipt != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "receipt: %s\n", r.Receipt)
	}
	for _, msg := range r.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "error: %s\n", msg)
	}
	return nil
}

// receiptEvent is one typed JSONL line. It is serialized with encoding/json so
// failure text containing quotes, backslashes, or newlines can never corrupt
// the receipt (SPEC.md §8; the bug this fixes is in UPDATES/ref-cross-exam.md).
type receiptEvent struct {
	TS        int64  `json:"ts"`
	RunID     string `json:"run_id"`
	Iteration int    `json:"iteration"`
	Event     string `json:"event"`
	Detail    string `json:"detail,omitempty"`
	ExitCode  *int   `json:"exit_code,omitempty"`
}

// loopState is the durable, resumable machine state (SPEC.md §8). Slice 0 holds
// the subset needed to record a run; later slices add the audit fields.
type loopState struct {
	SchemaVersion int     `json:"schema_version"`
	RunID         string  `json:"run_id"`
	Phase         string  `json:"phase"`
	Iteration     int     `json:"iteration"`
	LastCheckExit *int    `json:"last_check_exit,omitempty"`
	HaltReason    string  `json:"halt_reason,omitempty"`
	CumulativeUSD float64 `json:"cumulative_usd"`
	UpdatedTS     int64   `json:"updated_ts"`
}

type receiptWriter struct {
	f     *os.File
	runID string
}

func (w *receiptWriter) emit(iteration int, event, detail string, exitCode *int) {
	if w == nil || w.f == nil {
		return
	}
	ev := receiptEvent{
		TS:        nowFunc().Unix(),
		RunID:     w.runID,
		Iteration: iteration,
		Event:     event,
		Detail:    detail,
		ExitCode:  exitCode,
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_, _ = w.f.Write(append(line, '\n'))
}

// writeStateAtomic writes state via a temp file + rename so a crash mid-write
// can never leave a half-written, unparseable state file.
func writeStateAtomic(dir string, st loopState) error {
	st.UpdatedTS = nowFunc().Unix()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".state.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "state.json"))
}

type runConfig struct {
	runID         string
	maxIterations int
	check         string
	execCmd       string
	budgetUSD     float64
	workdir       string
}

// executeRun is the real check_fixpoint loop (SPEC.md §4, Slice 0 subset):
// each iteration runs the optional work command then the external check once,
// derives a COMPUTED halt reason, and records a typed receipt + durable state.
func executeRun(cmd *cobra.Command, cfg runConfig) error {
	if cfg.maxIterations < 1 {
		return failResponse(cmd, cfg.runID, exitInvariantFailed,
			"invariant_failed", "invariant failed: max-iterations must be >= 1")
	}
	if cfg.budgetUSD < 0 {
		return failResponse(cmd, cfg.runID, exitInvariantFailed,
			"invariant_failed", "invariant failed: budget-usd must be >= 0")
	}
	// No check, no loop (SPEC.md O1). This is the brand-defining precondition.
	if strings.TrimSpace(cfg.check) == "" {
		return failResponse(cmd, cfg.runID, exitWorkspaceInvalid,
			"workspace_invalid", "a loop requires an external check (--check). no check, no loop")
	}

	dir := filepath.Join(cfg.workdir, ".loopexec")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &cliError{Code: exitWorkspaceInvalid, Message: "workspace invalid or missing", Cause: err}
	}

	receiptPath := filepath.Join(dir, "run-"+cfg.runID+".jsonl")
	f, err := os.Create(receiptPath)
	if err != nil {
		return &cliError{Code: exitWorkspaceInvalid, Message: "cannot open receipt", Cause: err}
	}
	defer f.Close()
	rw := &receiptWriter{f: f, runID: cfg.runID}

	st := loopState{SchemaVersion: 1, RunID: cfg.runID, Phase: "running"}
	rw.emit(0, "run_start", fmt.Sprintf("check=%q exec=%q max=%d", cfg.check, cfg.execCmd, cfg.maxIterations), nil)

	var lastCheckExit *int
	haltReason := ""

	for i := 1; i <= cfg.maxIterations; i++ {
		st.Iteration = i
		rw.emit(i, "iter_start", "", nil)

		if strings.TrimSpace(cfg.execCmd) != "" {
			rc, _ := runShell(cfg.workdir, cfg.execCmd)
			rcCopy := rc
			rw.emit(i, "exec", "", &rcCopy)
			if rc != 0 {
				// A non-zero work step is an execution failure, not a blocked
				// task: the substrate/agent itself failed (SPEC.md §5, exit 40).
				haltReason = "execution_failure"
				break
			}
		}

		rc, _ := runShell(cfg.workdir, cfg.check)
		rcCopy := rc
		lastCheckExit = &rcCopy
		rw.emit(i, "check", "", &rcCopy)
		if rc == 0 {
			haltReason = "success_condition_met"
			break
		}
	}

	if haltReason == "" {
		haltReason = "max_iterations_reached"
	}

	st.HaltReason = haltReason
	st.LastCheckExit = lastCheckExit
	st.Phase = "halted"
	rw.emit(st.Iteration, "halt", haltReason, nil)
	if err := writeStateAtomic(dir, st); err != nil {
		return &cliError{Code: exitInternalError, Message: "cannot write state", Cause: err}
	}

	code := haltExitCode(haltReason)
	r := response{
		Tool:       toolName,
		Version:    toolVersion,
		Status:     "halted",
		RunID:      cfg.runID,
		Iteration:  st.Iteration,
		HaltReason: haltReason,
		CheckExit:  lastCheckExit,
		Receipt:    receiptPath,
		Errors:     []string{},
	}
	if haltReason == "execution_failure" {
		r.Status = "error"
		r.Errors = []string{"execution failure during work command"}
	}
	if err := printResponse(cmd, r); err != nil {
		return err
	}
	return &cliError{Code: code, Message: "halted: " + haltReason}
}

// failResponse prints one JSON object describing a precondition failure and
// returns the matching cliError, so even error paths emit a single object.
func failResponse(cmd *cobra.Command, runID string, code int, haltReason, msg string) error {
	r := response{
		Tool:       toolName,
		Version:    toolVersion,
		Status:     "error",
		RunID:      runID,
		HaltReason: haltReason,
		Errors:     []string{msg},
	}
	if err := printResponse(cmd, r); err != nil {
		return err
	}
	return &cliError{Code: code, Message: msg}
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "init",
		Short:        "Initialize loopexec workspace metadata",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := os.MkdirAll(".loopexec", 0o755)
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "workspace invalid or missing", Cause: err}
			}

			return printResponse(cmd, response{
				Tool:    toolName,
				Version: toolVersion,
				Status:  "initialized",
				Errors:  []string{},
			})
		},
	}
}

func newRunCmd() *cobra.Command {
	cfg := runConfig{}

	cmd := &cobra.Command{
		Use:          "run",
		Short:        "Run a bounded check_fixpoint loop until the check passes or a bound trips",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cfg.runID == "" {
				cfg.runID = "local"
			}
			if cfg.workdir == "" {
				cfg.workdir = "."
			}
			return executeRun(cmd, cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.runID, "run-id", "", "Run identifier")
	cmd.Flags().IntVar(&cfg.maxIterations, "max-iterations", 10, "Maximum iterations (fuse)")
	cmd.Flags().StringVar(&cfg.check, "check", "", "External check command; exit 0 means converged (required)")
	cmd.Flags().StringVar(&cfg.execCmd, "exec", "", "Work command run each iteration before the check (e.g. an agent invocation)")
	cmd.Flags().Float64Var(&cfg.budgetUSD, "budget-usd", 0, "Total run budget cap in USD (recorded; metering lands with agent execution)")
	cmd.Flags().StringVar(&cfg.workdir, "workdir", "", "Directory to run commands in (default: current directory)")
	return cmd
}

func newStatusCmd() *cobra.Command {
	var runID string
	var iteration int
	var haltReason string

	cmd := &cobra.Command{
		Use:          "status",
		Short:        "Show loop status",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if iteration < 0 {
				return &cliError{Code: exitInvariantFailed, Message: "invariant failed: iteration must be >= 0"}
			}
			return printResponse(cmd, response{
				Tool:       toolName,
				Version:    toolVersion,
				Status:     "ok",
				RunID:      runID,
				Iteration:  iteration,
				HaltReason: haltReason,
				Errors:     []string{},
			})
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "Run identifier")
	cmd.Flags().IntVar(&iteration, "iteration", 0, "Current iteration")
	cmd.Flags().StringVar(&haltReason, "halt-reason", "", "Current halt reason")
	return cmd
}

func newCheckCmd() *cobra.Command {
	var failInvariant bool

	cmd := &cobra.Command{
		Use:          "check",
		Short:        "Validate loop invariants (state hygiene, not the application oracle)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if failInvariant {
				r := response{
					Tool:    toolName,
					Version: toolVersion,
					Status:  "error",
					Errors:  []string{"invariant failed"},
				}
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return &cliError{Code: exitInvariantFailed, Message: "invariant failed"}
			}
			return printResponse(cmd, response{
				Tool:    toolName,
				Version: toolVersion,
				Status:  "ok",
				Errors:  []string{},
			})
		},
	}

	cmd.Flags().BoolVar(&failInvariant, "fail-invariant", false, "Force invariant failure")
	return cmd
}

func newStepCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "step",
		Short:        "Execute a single loop step",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printResponse(cmd, response{
				Tool:      toolName,
				Version:   toolVersion,
				Status:    "ok",
				RunID:     "local",
				Iteration: 1,
				Errors:    []string{},
			})
		},
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "loopexec",
		Short:        "loopexec — deterministic runtime for loop engineering",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newCheckCmd())
	cmd.AddCommand(newStepCmd())
	return cmd
}

func exitCode(err error) int {
	if err == nil {
		return exitSuccess
	}

	var ce *cliError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return exitInternalError
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(exitCode(err))
	}
}
