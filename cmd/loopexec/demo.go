package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	demoRunID      = "demo"
	demoStatusFile = "status.txt"
	demoCheck      = "grep -qx green status.txt"
	demoExec       = "printf 'green\\n' > status.txt"
)

// prepareDemoWorkspace reserves the two paths the proof owns. Refusing an
// existing receipt directory or fixture prevents `demo --workdir` from
// overwriting a real run or an unrelated user file.
func prepareDemoWorkspace(requested string) (string, error) {
	dir := requested
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "loopexec-demo-")
		if err != nil {
			return "", fmt.Errorf("create temporary workdir: %w", err)
		}
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve workdir: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return "", fmt.Errorf("create workdir: %w", err)
	}

	runtimeDir := filepath.Join(absDir, ".loopexec")
	if err := os.Mkdir(runtimeDir, 0o755); err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("refusing to overwrite existing %s", runtimeDir)
		}
		return "", fmt.Errorf("reserve runtime directory: %w", err)
	}

	fixture := filepath.Join(absDir, demoStatusFile)
	f, err := os.OpenFile(fixture, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		_ = os.Remove(runtimeDir) // empty directory created immediately above
		if os.IsExist(err) {
			return "", fmt.Errorf("refusing to overwrite existing %s", fixture)
		}
		return "", fmt.Errorf("create demo fixture: %w", err)
	}
	if _, err := f.WriteString("red\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(fixture)
		_ = os.Remove(runtimeDir)
		return "", fmt.Errorf("initialize demo fixture: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(fixture)
		_ = os.Remove(runtimeDir)
		return "", fmt.Errorf("close demo fixture: %w", err)
	}
	return absDir, nil
}

// newDemoCmd provides a zero-credential deterministic proof that uses the real
// bounded loop and the same fingerprint verifier as `loopexec replay`.
func newDemoCmd() *cobra.Command {
	var workdir string

	cmd := &cobra.Command{
		Use:          "demo",
		Short:        "Prove execute -> external check -> receipt -> replay without credentials",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := prepareDemoWorkspace(workdir)
			if err != nil {
				return failResponse(cmd, demoRunID, exitWorkspaceInvalid, "workspace_invalid", err.Error())
			}

			if rc, _ := runShell(dir, demoCheck); rc == 0 {
				return failResponse(cmd, demoRunID, exitInternalError, "internal_error", "demo fixture must start red")
			}

			// executeRun owns the loop, receipt, state, and computed halt. Its
			// intermediate response is suppressed so demo keeps the global
			// --json contract of exactly one object on stdout.
			inner := &cobra.Command{}
			inner.SetOut(io.Discard)
			inner.SetErr(io.Discard)
			runErr := executeRun(inner, runConfig{
				runID:         demoRunID,
				maxIterations: 3,
				check:         demoCheck,
				execCmd:       demoExec,
				workdir:       dir,
			})
			if exitCode(runErr) != exitConverged {
				return failResponse(cmd, demoRunID, exitInternalError, "internal_error", "demo's real loop did not converge")
			}

			stateFile, err := resolveStatePath(dir, demoRunID)
			if err != nil {
				return failResponse(cmd, demoRunID, exitInternalError, "internal_error", err.Error())
			}
			st, err := readState(stateFile)
			if err != nil {
				return failResponse(cmd, demoRunID, exitInternalError, "internal_error", "demo state is missing after the run")
			}
			_, verified, err := verifyStateFingerprint(st, dir)
			if err != nil {
				return failResponse(cmd, demoRunID, exitInternalError, "internal_error", err.Error())
			}
			if !verified {
				return failResponse(cmd, demoRunID, exitIntegrity, "objective_unverified", "demo replay fingerprint did not match")
			}

			receipt := filepath.Join(dir, ".loopexec", "run-demo.jsonl")
			if info, statErr := os.Stat(receipt); statErr != nil || info.Size() == 0 {
				return failResponse(cmd, demoRunID, exitInternalError, "internal_error", "demo receipt is missing or empty")
			}
			return printResponse(cmd, response{
				Tool:       toolName,
				Version:    toolVersion,
				Status:     "verified",
				RunID:      st.RunID,
				Iteration:  st.Iteration,
				HaltReason: st.HaltReason,
				CheckExit:  st.LastCheckExit,
				Receipt:    receipt,
				Verified:   &verified,
				Errors:     []string{},
			})
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Fresh directory for durable demo artifacts (default: create and retain a temporary directory)")
	return cmd
}
