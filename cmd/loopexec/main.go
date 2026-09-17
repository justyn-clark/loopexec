package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/subprocess"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

const (
	toolName    = "loopexec"
	toolVersion = "0.3.0"
)

// Exit-code classes are the coarse CI-branch buckets defined in SPEC.md section 5.
// The halt_reason string is the stable contract; the exit code is its class.
// Codes 0/10/11/12/20/30/40/50 predate this; classes 13/14/16/17/18/19 all emit
// (18 via inspect-cost, 15 via doctor --mutate-cmd); only 11's task_list reasons remain.
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
// (SPEC.md section 5). This is the single place the mapping lives.
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
	// Silent marks an error whose outcome was already emitted via printResponse
	// (a computed halt or a structured failure). main carries only its exit code
	// and does NOT echo it to stderr, so a converged run never prints "Error:".
	Silent bool
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
	FailureCause  string `json:"failure_cause,omitempty"`
	BestCandidate string `json:"best_candidate,omitempty"`
	Tool          string `json:"tool"`
	Version       string `json:"version"`
	Status        string `json:"status"`
	RunID         string `json:"run_id,omitempty"`
	Iteration     int    `json:"iteration,omitempty"`
	HaltReason    string `json:"halt_reason,omitempty"`
	CheckExit     *int   `json:"check_exit,omitempty"`
	Receipt       string `json:"receipt,omitempty"`

	Probe  *probeReport  `json:"probe,omitempty"`
	Doctor *doctorReport `json:"doctor,omitempty"`

	Verdict string `json:"verdict,omitempty"`
	Why     string `json:"why,omitempty"`

	Verified  *bool  `json:"verified,omitempty"`
	Signature string `json:"signature,omitempty"`

	Ops       *opsReport       `json:"ops,omitempty"`
	Context   *contextReport   `json:"context,omitempty"`
	Isolation *isolationReport `json:"isolation,omitempty"`
	Report    *reportSummary   `json:"report,omitempty"`
	Cost      *costReport      `json:"cost,omitempty"`

	Errors []string `json:"errors"`
}

// isolationReport is the output of the isolate command (SPEC.md section 7).
// It NEVER carries the minted credential value, only its lifecycle metadata.
type isolationReport struct {
	Sandbox       string   `json:"sandbox"`
	Clone         string   `json:"clone"`
	AgentImage    string   `json:"agent_image"`
	ExecImage     string   `json:"exec_image"`
	ExecZoneCmd   string   `json:"exec_zone_cmd"`  // redacted, display-only
	AgentZoneCmd  string   `json:"agent_zone_cmd"` // redacted, display-only
	EgressAllow   []string `json:"egress_allow"`
	KeyEnv        string   `json:"key_env,omitempty"`
	Hardened      bool     `json:"hardened"`
	Minted        bool     `json:"minted"`
	Revoked       bool     `json:"revoked"`
	Executed      bool     `json:"executed"`
	ExecZoneExit  *int     `json:"exec_zone_exit,omitempty"`
	AgentZoneExit *int     `json:"agent_zone_exit,omitempty"`
}

// opsReport carries the output of the reexecute / escalate / watch commands.
type opsReport struct {
	Samples         int            `json:"samples,omitempty"`
	Converged       int            `json:"converged,omitempty"`
	ConvergenceRate float64        `json:"convergence_rate,omitempty"`
	Distribution    map[string]int `json:"distribution,omitempty"`
	Packet          string         `json:"packet,omitempty"`
	HeartbeatAgeS   *int           `json:"heartbeat_age_s,omitempty"`
	Stale           *bool          `json:"stale,omitempty"`
}

// contextReport is the output of build-context (SPEC.md section 4 step 1).
type contextReport struct {
	Path            string   `json:"path"`
	TokensEstimated int      `json:"tokens_estimated"`
	BudgetTokens    int      `json:"budget_tokens"`
	FilesIncluded   []string `json:"files_included"`
	FilesDropped    []string `json:"files_dropped"`
}

var jsonOutput bool

// nowFunc and runShell are seams so the loop is testable without a clock or a
// real subprocess. The defaults are the production implementations.
var nowFunc = time.Now

var runShell = func(workdir, command string) (int, string) {
	r := subprocess.Run(context.Background(), subprocess.Options{Dir: workdir, Argv: []string{"sh", "-c", command}})
	return r.ExitCode, r.Combined
}
var runArgv = func(workdir, name string, args ...string) (int, string) {
	r := subprocess.Run(context.Background(), subprocess.Options{Dir: workdir, Argv: append([]string{name}, args...)})
	return r.ExitCode, r.Combined
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
	if r.Probe != nil {
		p := r.Probe
		fmt.Fprintf(cmd.OutOrStdout(),
			"probe: %d runs, stable=%t, flake_upper_bound=%.4f (95%%), certified=%t\n",
			p.Runs, p.Stable, p.FlakeUpperBound, p.Certified)
	}
	if r.Doctor != nil {
		for _, c := range r.Doctor.Checks {
			fmt.Fprintf(cmd.OutOrStdout(), "  [%-7s] %s: %s\n", c.Status, c.Name, c.Detail)
		}
	}
	if r.Verified != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "verified: %t\n", *r.Verified)
	}
	if r.Signature != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "signature: %s\n", r.Signature)
	}
	for _, msg := range r.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "error: %s\n", msg)
	}
	return nil
}

// receiptEvent is one typed JSONL line. It is serialized with encoding/json so
// failure text containing quotes, backslashes, or newlines can never corrupt
// the receipt (SPEC.md section 8).
type receiptEvent struct {
	OutputTruncated bool   `json:"output_truncated,omitempty"`
	Phase           string `json:"phase,omitempty"`
	DurationMS      int64  `json:"duration_ms"`
	Cause           string `json:"cause,omitempty"`
	TS              int64  `json:"ts"`
	RunID           string `json:"run_id"`
	Iteration       int    `json:"iteration"`
	Event           string `json:"event"`
	Detail          string `json:"detail,omitempty"`
	ExitCode        *int   `json:"exit_code,omitempty"`
}

// Receipt-pin types (SPEC.md section 8): everything that determines the output,
// so a receipt can be verified offline.
type modelPin struct {
	Provider string `json:"provider,omitempty"`
	ID       string `json:"id"`
	Version  string `json:"version,omitempty"`
}

type samplingPin struct {
	Temperature float64 `json:"temperature"`
	Seed        int     `json:"seed"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

type manifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// checkFingerprint is the verifiable verdict: the recorded check's exit code and
// a hash of its normalized output. `replay` reproduces and compares it.
type checkFingerprint struct {
	ExitCode     int    `json:"exit_code"`
	OutputSHA256 string `json:"output_sha256"`
}

// escalationState is the resumable human-handoff record (SPEC.md section 9).
type escalationState struct {
	State   string `json:"state"` // none | paged | acked
	Channel string `json:"channel,omitempty"`
	Ref     string `json:"ref,omitempty"`
	AckedBy string `json:"acked_by,omitempty"`
}

// loopState is the durable, resumable machine state (SPEC.md section 8). Slice 0 holds
// the subset needed to record a run; later slices add the audit fields.
type loopState struct {
	Workflow          *workflow.Config `json:"workflow,omitempty"`
	WorkflowHash      string           `json:"workflow_hash,omitempty"`
	RunPolicyHash     string           `json:"run_policy_hash,omitempty"`
	Budget            *budgetBook      `json:"budget,omitempty"`
	Numeric           *numericState    `json:"numeric,omitempty"`
	Candidate         *candidateState  `json:"candidate,omitempty"`
	BestCandidatePath string           `json:"best_candidate_path,omitempty"`
	IntegrityBaseline []string         `json:"integrity_baseline,omitempty"`
	DeadlineNS        int64            `json:"deadline_ns,omitempty"`
	CommandTimeoutNS  int64            `json:"command_timeout_ns,omitempty"`
	GraceNS           int64            `json:"grace_ns,omitempty"`

	FailureCause  string  `json:"failure_cause,omitempty"`
	SchemaVersion int     `json:"schema_version"`
	RunID         string  `json:"run_id"`
	Phase         string  `json:"phase"`
	Iteration     int     `json:"iteration"`
	LastCheckExit *int    `json:"last_check_exit,omitempty"`
	HaltReason    string  `json:"halt_reason,omitempty"`
	CumulativeUSD float64 `json:"cumulative_usd"`

	// Set-based progress (SPEC.md section 3.2), populated when --failures-cmd is set.
	InitialFailCount *int `json:"initial_fail_count,omitempty"`
	BestFailCount    *int `json:"best_fail_count,omitempty"`
	BestIteration    int  `json:"best_iteration,omitempty"`
	EverImproved     bool `json:"ever_improved,omitempty"`

	// Receipt pinning (SPEC.md section 8) for replay / attest.
	Check           string            `json:"check,omitempty"`
	Workdir         string            `json:"workdir,omitempty"`
	Model           *modelPin         `json:"model,omitempty"`
	Sampling        *samplingPin      `json:"sampling,omitempty"`
	ContextManifest []manifestEntry   `json:"context_manifest,omitempty"`
	CostUSD         float64           `json:"cost_usd,omitempty"`
	Fingerprint     *checkFingerprint `json:"fingerprint,omitempty"`

	// Recorded config for reexecute (SPEC.md section 8) + ops state (section 9).
	Exec              string           `json:"exec,omitempty"`
	MaxIterations     int              `json:"max_iterations,omitempty"`
	FailuresCmd       string           `json:"failures_cmd,omitempty"`
	IntegrityCmd      string           `json:"integrity_cmd,omitempty"`
	NoProgressK       int              `json:"no_progress_k,omitempty"`
	DiffsMergedUnread int              `json:"diffs_merged_unread,omitempty"`
	Escalation        *escalationState `json:"escalation,omitempty"`

	UpdatedTS int64 `json:"updated_ts"`
}

// readState loads a recorded run state for explain-halt / resume.
func readState(path string) (loopState, error) {
	var st loopState
	data, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	return st, nil
}

type receiptWriter struct {
	err   error
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
	if _, e := w.f.Write(append(line, '\n')); e != nil {
		w.err = e
	}
}

// heartbeat is the liveness marker an external `watch` reads (SPEC.md section 9).
type heartbeat struct {
	TS        int64  `json:"ts"`
	PID       int    `json:"pid"`
	Iteration int    `json:"iteration"`
	Phase     string `json:"phase"`
}

func writeHeartbeat(dir string, iteration int, phase string) {
	hb := heartbeat{TS: nowFunc().Unix(), PID: os.Getpid(), Iteration: iteration, Phase: phase}
	data, err := json.Marshal(hb)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "heartbeat"), data, 0o644)
}

// writeStateAtomic writes the latest-run state (state.json) via a temp file +
// rename so a crash mid-write can never leave a half-written, unparseable file.
func writeStateAtomic(dir string, st loopState) error {
	return writeStateFile(dir, "state.json", st)
}

// writeStateFile atomically writes state to dir/name (temp file + rename). It
// backs both state.json (the latest run) and the per-run snapshot that lets
// replay / explain-halt / attest target a specific run by --run-id.
func writeStateFile(dir, name string, st loopState) error {
	st.UpdatedTS = nowFunc().Unix()
	return workflow.Atomic(filepath.Join(dir, name), st)
}

type runConfig struct {
	workflowFile                        string
	resume                              bool
	totalTimeout, commandTimeout, grace time.Duration
	runID                               string
	maxIterations                       int
	check                               string
	execCmd                             string
	budgetUSD                           float64
	workdir                             string
	failuresCmd                         string
	noProgressK                         int
	integrityCmd                        string

	// Receipt pinning (SPEC.md section 8).
	modelProvider string
	modelID       string
	modelVersion  string
	temperature   float64
	seed          int
	maxTokens     int
	contextFiles  []string
	costUSD       float64

	comprehensionEvery int
	once               bool
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
	return &cliError{Code: code, Message: msg, Silent: true}
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
			// --once is the single-iteration debug form (it absorbs the legacy
			// `step` stub): run exactly one iteration, then halt on the computed
			// outcome (success_condition_met if the check passes, else
			// max_iterations_reached). It overrides --max-iterations.
			if cfg.once {
				cfg.maxIterations = 1
			}
			return executeRun(cmd, cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.workflowFile, "workflow", "", "Pinned workflow JSON (argv hooks, metering, candidates, numeric policy)")
	cmd.Flags().BoolVar(&cfg.resume, "resume", false, "Resume a workflow run with identical policy and cumulative bounds")
	cmd.Flags().DurationVar(&cfg.totalTimeout, "timeout", 0, "Total run deadline (0 = disabled)")
	cmd.Flags().DurationVar(&cfg.commandTimeout, "command-timeout", 0, "Deadline for every command and collector (0 = disabled)")
	cmd.Flags().DurationVar(&cfg.grace, "terminate-grace", 250*time.Millisecond, "Grace between process-group TERM and KILL")
	cmd.Flags().StringVar(&cfg.runID, "run-id", "", "Run identifier")
	cmd.Flags().IntVar(&cfg.maxIterations, "max-iterations", 10, "Maximum iterations (fuse)")
	cmd.Flags().BoolVar(&cfg.once, "once", false, "Run exactly one iteration (debug single-step); overrides --max-iterations")
	cmd.Flags().StringVar(&cfg.check, "check", "", "External check command; exit 0 means converged (required)")
	cmd.Flags().StringVar(&cfg.execCmd, "exec", "", "Work command run each iteration before the check (e.g. an agent invocation)")
	cmd.Flags().Float64Var(&cfg.budgetUSD, "budget-usd", 0, "Run-total USD allowance; requires workflow strict or observed metering")
	cmd.Flags().StringVar(&cfg.workdir, "workdir", "", "Directory to run commands in (default: current directory)")
	cmd.Flags().StringVar(&cfg.failuresCmd, "failures-cmd", "", "Command printing current open failures (one identity per line); enables set-based progress and the no-regression ratchet")
	cmd.Flags().IntVar(&cfg.noProgressK, "no-progress-k", 3, "Halt no_progress_detected after K iterations with no new best failing-set size")
	cmd.Flags().StringVar(&cfg.integrityCmd, "integrity-cmd", "", "Command printing the test-determining surface (one identity per line); its t0 set MUST NOT lose a member (metric-integrity gate)")
	cmd.Flags().StringVar(&cfg.modelProvider, "model-provider", "", "Record the model provider in the receipt (records only; does NOT select the model - --exec does)")
	cmd.Flags().StringVar(&cfg.modelID, "model-id", "", "Record the model id in the receipt (records only; the call lives in --exec); enables the model/sampling pin")
	cmd.Flags().StringVar(&cfg.modelVersion, "model-version", "", "Record the model version/build in the receipt")
	cmd.Flags().Float64Var(&cfg.temperature, "temperature", 0, "Recorded sampling temperature")
	cmd.Flags().IntVar(&cfg.seed, "seed", 0, "Recorded sampling seed")
	cmd.Flags().IntVar(&cfg.maxTokens, "max-tokens", 0, "Recorded sampling max_tokens")
	cmd.Flags().StringArrayVar(&cfg.contextFiles, "context-file", nil, "File to include in the receipt context manifest (path + sha256); repeatable")
	cmd.Flags().Float64Var(&cfg.costUSD, "cost-usd", 0, "Legacy recorded cost metadata; workflow actuals are metered separately")
	cmd.Flags().IntVar(&cfg.comprehensionEvery, "comprehension-every", 0, "Halt comprehension_debt_exceeded after N iterations without a `loopexec ack` (0 = off)")
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
				return &cliError{Code: exitInvariantFailed, Message: "invariant failed", Silent: true}
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
		Use:     "loopexec",
		Version: toolVersion,
		Short:   "loopexec - deterministic runtime for loop engineering",
		// We render every outcome ourselves (printResponse + exit code). Cobra
		// must not also print "Error: ..." - that double-printed and labeled a
		// converged halt as an error.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	cmd.AddCommand(&cobra.Command{
		Use: "version", Short: "Print the CLI identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printResponse(cmd, response{Tool: toolName, Version: toolVersion, Status: "ok", Errors: []string{}})
		},
	})
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newDemoCmd())
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newCheckCmd())
	cmd.AddCommand(newStepCmd())
	cmd.AddCommand(newProbeCheckCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newExplainHaltCmd())
	cmd.AddCommand(newReplayCmd())
	cmd.AddCommand(newAttestCmd())
	cmd.AddCommand(newReportCmd())
	cmd.AddCommand(newReexecuteCmd())
	cmd.AddCommand(newEscalateCmd())
	cmd.AddCommand(newWatchCmd())
	cmd.AddCommand(newAckCmd())
	cmd.AddCommand(newBuildContextCmd())
	cmd.AddCommand(newIsolateCmd())
	cmd.AddCommand(newInspectCostCmd())
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
		var ce *cliError
		if errors.As(err, &ce) {
			// A Silent cliError already emitted its outcome (the JSON object or
			// human summary) and only carries the exit code - do not echo it,
			// so a converged loop exits 10 without an "Error:" line.
			if !ce.Silent {
				fmt.Fprintln(os.Stderr, ce.Error())
			}
			os.Exit(ce.Code)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(exitInternalError)
	}
}
