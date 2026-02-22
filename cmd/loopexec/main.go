package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	toolName    = "loopexec"
	toolVersion = "0.1.0-rc1"
)

const (
	exitSuccess                = 0
	exitHaltedSuccessCondition = 10
	exitHaltedBlocked          = 11
	exitHaltedMaxIterations    = 12
	exitInvariantFailed        = 20
	exitWorkspaceInvalid       = 30
	exitExecutionFailure       = 40
	exitInternalError          = 50
)

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
	Errors     []string `json:"errors"`
}

var jsonOutput bool

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
	for _, msg := range r.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "error: %s\n", msg)
	}
	return nil
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
	var runID string
	var maxIterations int
	var haltReason string

	cmd := &cobra.Command{
		Use:          "run",
		Short:        "Run one bounded execution loop",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if maxIterations <= 0 {
				return &cliError{Code: exitInvariantFailed, Message: "invariant failed: max-iterations must be >= 1"}
			}

			if runID == "" {
				runID = "local"
			}

			r := response{
				Tool:      toolName,
				Version:   toolVersion,
				Status:    "ok",
				RunID:     runID,
				Iteration: 1,
				Errors:    []string{},
			}

			switch haltReason {
			case "":
				return printResponse(cmd, r)
			case "success":
				r.Status = "halted"
				r.HaltReason = "success_condition_met"
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return &cliError{Code: exitHaltedSuccessCondition, Message: "halted: success condition met"}
			case "blocked":
				r.Status = "halted"
				r.HaltReason = "blocked"
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return &cliError{Code: exitHaltedBlocked, Message: "halted: blocked"}
			case "max-iterations":
				r.Status = "halted"
				r.HaltReason = "max_iterations_reached"
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return &cliError{Code: exitHaltedMaxIterations, Message: "halted: max iterations reached"}
			case "exec-fail":
				r.Status = "error"
				r.HaltReason = "execution_failure"
				r.Errors = []string{"execution failure"}
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return &cliError{Code: exitExecutionFailure, Message: "execution failure"}
			default:
				return &cliError{Code: exitInvariantFailed, Message: "invariant failed: unsupported halt-reason"}
			}
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "Run identifier")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 1, "Maximum iterations")
	cmd.Flags().StringVar(&haltReason, "halt-reason", "", "Force halt behavior: success|blocked|max-iterations|exec-fail")
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
		Short:        "Validate loop invariants",
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
		Aliases:      []string{"loopexec"},
		Short:        "loopexec CLI",
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
