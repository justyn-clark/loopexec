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
	toolVersion = "0.2.0"
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
	Tool       string `json:"tool"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	RunID      string `json:"run_id,omitempty"`
	Iteration  int    `json:"iteration,omitempty"`
	HaltReason string `json:"halt_reason,omitempty"`
	CheckExit  *int   `json:"check_exit,omitempty"`
	Receipt    string `json:"receipt,omitempty"`

	Probe  *probeReport  `json:"probe,omitempty"`
	Doctor *doctorReport `json:"doctor,omitempty"`

	Verdict string `json:"verdict,omitempty"`
	Why     string `json:"why,omitempty"`

	Verified  *bool  `json:"verified,omitempty"`
	Signature string `json:"signature,omitempty"`

	Ops       *opsReport       `json:"ops,omitempty"`
	Context   *contextReport   `json:"context,omitempty"`
	Isolation *isolationReport `json:"isolation,omitempty"`

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

// runArgv runs a program with an explicit argv (no host shell), so untrusted
// command strings passed into a container or git cannot inject on the host.
var runArgv = func(workdir, name string, args ...string) (int, string) {
	c := exec.Command(name, args...)
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
// the receipt (SPEC.md §8; the bug this fixes is in UPDATES/ref-cross-exam.md).
type receiptEvent struct {
	TS        int64  `json:"ts"`
	RunID     string `json:"run_id"`
	Iteration int    `json:"iteration"`
	Event     string `json:"event"`
	Detail    string `json:"detail,omitempty"`
	ExitCode  *int   `json:"exit_code,omitempty"`
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
	failuresCmd   string
	noProgressK   int
	integrityCmd  string

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
	var lastCheckOut string
	haltReason := ""
	diffsUnread := 0

	var tracker *progressTracker
	if strings.TrimSpace(cfg.failuresCmd) != "" {
		tracker = newProgressTracker(cfg.noProgressK)
	}

	// Metric-integrity baseline captured at t0 (SPEC.md section 6): the
	// test-determining surface MUST NOT lose a member during the run.
	var integrityBaseline map[string]struct{}
	if strings.TrimSpace(cfg.integrityCmd) != "" {
		_, bout := runShell(cfg.workdir, cfg.integrityCmd)
		integrityBaseline, _ = parseFailures(bout)
		n := len(integrityBaseline)
		rw.emit(0, "integrity_baseline", "", &n)
	}

	for i := 1; i <= cfg.maxIterations; i++ {
		st.Iteration = i
		rw.emit(i, "iter_start", "", nil)
		writeHeartbeat(dir, i, "iter_start")

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

		// Guards dominate success (SPEC.md section 6): the metric-integrity gate
		// is evaluated BEFORE the check can declare green, so a suite that went
		// green by weakening the test surface halts here instead of succeeding.
		if integrityBaseline != nil {
			_, cout := runShell(cfg.workdir, cfg.integrityCmd)
			cur, _ := parseFailures(cout)
			if missing := missingMembers(integrityBaseline, cur); len(missing) > 0 {
				n := len(missing)
				rw.emit(i, "integrity_violation", strings.Join(missing, ","), &n)
				haltReason = "metric_integrity_violation"
				break
			}
		}

		rc, checkOut := runShell(cfg.workdir, cfg.check)
		rcCopy := rc
		lastCheckExit = &rcCopy
		lastCheckOut = checkOut
		rw.emit(i, "check", "", &rcCopy)
		if rc == 0 {
			haltReason = "success_condition_met"
			break
		}

		// Set-based progress + ratchet (SPEC.md section 3.2): track the failing
		// set, halt on regression, oscillation, or no strict decrease over K.
		if tracker != nil {
			_, fout := runShell(cfg.workdir, cfg.failuresCmd)
			F, order := parseFailures(fout)
			sz := len(F)
			rw.emit(i, "progress", setHash(order), &sz)
			pr := tracker.observe(i, F, order)
			best := tracker.bestSize
			init := tracker.initialSize
			st.BestFailCount = &best
			st.InitialFailCount = &init
			st.BestIteration = tracker.bestIter
			st.EverImproved = tracker.everImproved
			if pr != "" {
				haltReason = pr
				break
			}
		}

		// Comprehension gate (SPEC.md section 9): after N merged-but-unread
		// iterations, halt to force a human read (cleared by `loopexec ack`).
		diffsUnread++
		if cfg.comprehensionEvery > 0 && diffsUnread >= cfg.comprehensionEvery {
			haltReason = "comprehension_debt_exceeded"
			break
		}
	}

	if haltReason == "" {
		haltReason = "max_iterations_reached"
	}

	st.HaltReason = haltReason
	st.LastCheckExit = lastCheckExit
	st.Phase = "halted"

	// Receipt pinning (SPEC.md section 8): everything needed to verify offline.
	st.Check = cfg.check
	st.Workdir = cfg.workdir
	st.CostUSD = cfg.costUSD
	st.Exec = cfg.execCmd
	st.MaxIterations = cfg.maxIterations
	st.FailuresCmd = cfg.failuresCmd
	st.IntegrityCmd = cfg.integrityCmd
	st.NoProgressK = cfg.noProgressK
	st.DiffsMergedUnread = diffsUnread
	if cfg.modelID != "" {
		st.Model = &modelPin{Provider: cfg.modelProvider, ID: cfg.modelID, Version: cfg.modelVersion}
		st.Sampling = &samplingPin{Temperature: cfg.temperature, Seed: cfg.seed, MaxTokens: cfg.maxTokens}
	}
	if man := buildManifest(cfg.workdir, cfg.contextFiles); len(man) > 0 {
		st.ContextManifest = man
	}
	if lastCheckExit != nil {
		st.Fingerprint = &checkFingerprint{
			ExitCode:     *lastCheckExit,
			OutputSHA256: sha256hex([]byte(normalizeOutput(lastCheckOut))),
		}
	}

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
	cmd.Flags().StringVar(&cfg.failuresCmd, "failures-cmd", "", "Command printing current open failures (one identity per line); enables set-based progress and the no-regression ratchet")
	cmd.Flags().IntVar(&cfg.noProgressK, "no-progress-k", 3, "Halt no_progress_detected after K iterations with no new best failing-set size")
	cmd.Flags().StringVar(&cfg.integrityCmd, "integrity-cmd", "", "Command printing the test-determining surface (one identity per line); its t0 set MUST NOT lose a member (metric-integrity gate)")
	cmd.Flags().StringVar(&cfg.modelProvider, "model-provider", "", "Pin the model provider into the receipt (section 8)")
	cmd.Flags().StringVar(&cfg.modelID, "model-id", "", "Pin the model id into the receipt; enables the model/sampling pin")
	cmd.Flags().StringVar(&cfg.modelVersion, "model-version", "", "Pin the model version/build into the receipt")
	cmd.Flags().Float64Var(&cfg.temperature, "temperature", 0, "Recorded sampling temperature")
	cmd.Flags().IntVar(&cfg.seed, "seed", 0, "Recorded sampling seed")
	cmd.Flags().IntVar(&cfg.maxTokens, "max-tokens", 0, "Recorded sampling max_tokens")
	cmd.Flags().StringArrayVar(&cfg.contextFiles, "context-file", nil, "File to include in the receipt context manifest (path + sha256); repeatable")
	cmd.Flags().Float64Var(&cfg.costUSD, "cost-usd", 0, "Recorded run cost in USD (live metering is Planned)")
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
	cmd.AddCommand(newProbeCheckCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newExplainHaltCmd())
	cmd.AddCommand(newReplayCmd())
	cmd.AddCommand(newAttestCmd())
	cmd.AddCommand(newReexecuteCmd())
	cmd.AddCommand(newEscalateCmd())
	cmd.AddCommand(newWatchCmd())
	cmd.AddCommand(newAckCmd())
	cmd.AddCommand(newBuildContextCmd())
	cmd.AddCommand(newIsolateCmd())
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
