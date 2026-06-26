package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// copyTree copies src into dst, skipping .loopexec and .git so a reexecute
// sample runs in an isolated workspace without inheriting prior run state.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		top := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
		if top == ".loopexec" || top == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		mode := os.FileMode(0o644)
		if info, ierr := d.Info(); ierr == nil {
			mode = info.Mode().Perm()
		}
		return os.WriteFile(target, data, mode)
	})
}

// newReexecuteCmd re-runs the recorded loop config N times in isolated copies
// and reports the halt-reason distribution. Unlike replay (which only verifies
// the recorded verdict), this is a live, non-deterministic re-run -- so it
// reports a statistical match, never byte identity (SPEC.md section 8).
func newReexecuteCmd() *cobra.Command {
	var workdir string
	var samples int
	var confirm bool
	cmd := &cobra.Command{
		Use:          "reexecute",
		Short:        "Live re-run of the recorded loop config N times; reports a statistical match (--confirm)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			st, err := readState(statePath(workdir))
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "no recorded run state", Cause: err}
			}
			if st.Check == "" {
				return &cliError{Code: exitWorkspaceInvalid, Message: "recorded receipt has no check to re-execute"}
			}
			if !confirm {
				return &cliError{Code: exitInvariantFailed,
					Message: "reexecute is a live, budget-burning re-run; pass --confirm to proceed"}
			}
			if samples < 1 {
				samples = 1
			}
			self, eerr := os.Executable()
			if eerr != nil {
				self = os.Args[0]
			}
			maxIter := st.MaxIterations
			if maxIter < 1 {
				maxIter = 10
			}

			dist := map[string]int{}
			converged := 0
			for s := 0; s < samples; s++ {
				tmp, terr := os.MkdirTemp("", "loopexec-reexec-")
				if terr != nil {
					return &cliError{Code: exitInternalError, Message: "cannot create sample workspace", Cause: terr}
				}
				if cerr := copyTree(workdir, tmp); cerr != nil {
					os.RemoveAll(tmp)
					return &cliError{Code: exitInternalError, Message: "cannot stage sample workspace", Cause: cerr}
				}
				args := []string{"run", "--json", "--workdir", tmp,
					"--run-id", fmt.Sprintf("reexec-%d", s), "--check", st.Check,
					"--max-iterations", strconv.Itoa(maxIter)}
				if st.Exec != "" {
					args = append(args, "--exec", st.Exec)
				}
				if st.FailuresCmd != "" {
					args = append(args, "--failures-cmd", st.FailuresCmd, "--no-progress-k", strconv.Itoa(st.NoProgressK))
				}
				if st.IntegrityCmd != "" {
					args = append(args, "--integrity-cmd", st.IntegrityCmd)
				}
				out, _ := exec.Command(self, args...).Output() //nolint:errcheck // halt is non-zero by design
				var resp struct {
					HaltReason string `json:"halt_reason"`
				}
				_ = json.Unmarshal(out, &resp)
				hr := resp.HaltReason
				if hr == "" {
					hr = "unknown"
				}
				dist[hr]++
				if hr == "success_condition_met" {
					converged++
				}
				os.RemoveAll(tmp)
			}

			rate := float64(converged) / float64(samples)
			r := response{
				Tool: toolName, Version: toolVersion, Status: "ok", RunID: st.RunID, Errors: []string{},
				Ops: &opsReport{Samples: samples, Converged: converged, ConvergenceRate: rate, Distribution: dist},
			}
			if err := printResponse(cmd, r); err != nil {
				return err
			}
			if !jsonOutput {
				fmt.Fprintf(cmd.OutOrStdout(), "reexecute: %d/%d converged (rate %.2f), distribution %v\n",
					converged, samples, rate, dist)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec/state.json (default: current directory)")
	cmd.Flags().IntVar(&samples, "samples", 3, "Number of live re-run samples")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm the budget-burning live re-run")
	return cmd
}

// newEscalateCmd writes a structured escalation packet and marks the run paged
// (SPEC.md section 9). `loopexec ack` clears it.
func newEscalateCmd() *cobra.Command {
	var workdir, channel string
	cmd := &cobra.Command{
		Use:          "escalate",
		Short:        "Emit a structured escalation packet and mark the run paged (cleared by `ack`)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			if channel == "" {
				channel = "file"
			}
			st, err := readState(statePath(workdir))
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "no recorded run state", Cause: err}
			}

			packet := map[string]any{
				"run_id":              st.RunID,
				"halt_reason":         st.HaltReason,
				"iteration":           st.Iteration,
				"best_fail_count":     st.BestFailCount,
				"cost_usd":            st.CostUSD,
				"fingerprint":         st.Fingerprint,
				"diffs_merged_unread": st.DiffsMergedUnread,
			}
			data, _ := json.MarshalIndent(packet, "", "  ")

			ref := ""
			switch channel {
			case "stdout":
				fmt.Fprintln(cmd.ErrOrStderr(), string(data))
				ref = "stdout"
			default: // file
				ref = filepath.Join(workdir, ".loopexec", "escalation-"+st.RunID+".json")
				if werr := os.WriteFile(ref, append(data, '\n'), 0o644); werr != nil {
					return &cliError{Code: exitWorkspaceInvalid, Message: "cannot write escalation packet", Cause: werr}
				}
			}

			st.Escalation = &escalationState{State: "paged", Channel: channel, Ref: ref}
			if werr := writeStateAtomic(filepath.Join(workdir, ".loopexec"), st); werr != nil {
				return &cliError{Code: exitInternalError, Message: "cannot update state", Cause: werr}
			}

			r := response{Tool: toolName, Version: toolVersion, Status: "paged", RunID: st.RunID,
				HaltReason: "escalation_pending", Ops: &opsReport{Packet: ref}, Errors: []string{}}
			return printResponse(cmd, r)
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec/state.json (default: current directory)")
	cmd.Flags().StringVar(&channel, "channel", "file", "Escalation channel: file | stdout (github/slack are Planned)")
	return cmd
}

// newWatchCmd reads the heartbeat and reports whether the run is alive or
// stale (SPEC.md section 9). The kill-the-wedged-PID actuator is Planned.
func newWatchCmd() *cobra.Command {
	var workdir string
	var stallTimeout int
	cmd := &cobra.Command{
		Use:          "watch",
		Short:        "Read the heartbeat and report alive vs heartbeat_stale",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			data, err := os.ReadFile(filepath.Join(workdir, ".loopexec", "heartbeat"))
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "no heartbeat to watch", Cause: err}
			}
			var hb heartbeat
			if uerr := json.Unmarshal(data, &hb); uerr != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "heartbeat is unparseable", Cause: uerr}
			}
			age := int(nowFunc().Unix() - hb.TS)
			stale := age > stallTimeout
			r := response{Tool: toolName, Version: toolVersion, RunID: "", Errors: []string{},
				Ops: &opsReport{HeartbeatAgeS: &age, Stale: &stale}}
			if !stale {
				r.Status = "alive"
				return printResponse(cmd, r)
			}
			r.Status = "error"
			r.HaltReason = "heartbeat_stale"
			r.Errors = []string{fmt.Sprintf("heartbeat is %ds old (> %ds); worker pid %d may be wedged", age, stallTimeout, hb.PID)}
			if perr := printResponse(cmd, r); perr != nil {
				return perr
			}
			return &cliError{Code: exitLivenessDrift, Message: "heartbeat_stale"}
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec/heartbeat (default: current directory)")
	cmd.Flags().IntVar(&stallTimeout, "stall-timeout", 600, "Seconds before the heartbeat is considered stale")
	return cmd
}

// newAckCmd clears the comprehension debt and any paged escalation, recording
// the reviewer (SPEC.md section 9). A forcing/visibility gate, not proof of
// comprehension.
func newAckCmd() *cobra.Command {
	var workdir, reviewer string
	cmd := &cobra.Command{
		Use:          "ack",
		Short:        "Clear comprehension debt and any paged escalation (records the reviewer)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			if strings.TrimSpace(reviewer) == "" {
				return &cliError{Code: exitInvariantFailed, Message: "ack requires --reviewer"}
			}
			st, err := readState(statePath(workdir))
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "no recorded run state", Cause: err}
			}
			st.DiffsMergedUnread = 0
			if st.Escalation != nil && st.Escalation.State == "paged" {
				st.Escalation.State = "acked"
				st.Escalation.AckedBy = reviewer
			}
			if werr := writeStateAtomic(filepath.Join(workdir, ".loopexec"), st); werr != nil {
				return &cliError{Code: exitInternalError, Message: "cannot update state", Cause: werr}
			}
			r := response{Tool: toolName, Version: toolVersion, Status: "acked", RunID: st.RunID, Errors: []string{}}
			return printResponse(cmd, r)
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec/state.json (default: current directory)")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "Reviewer identity recorded in the ack (required)")
	return cmd
}
