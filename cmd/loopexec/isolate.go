package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

// runIDRe and imageRe fail closed against path traversal and docker
// argument-injection: a run-id becomes filesystem paths, and an image that
// starts with '-' would be parsed by docker as a flag (--privileged, etc.).
var runIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
var imageRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]*$`)

// detachedClone makes a real, separate clone (NOT a git worktree, which shares
// the object DB/refs/hooks/credentials and is therefore not a boundary) and
// hardens it -- failing CLOSED if any hardening step fails (SPEC.md section 7).
func detachedClone(repo, branch, into string) error {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	args := []string{"clone", "--no-local", "--no-hardlinks", "--single-branch"}
	if strings.TrimSpace(branch) != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "file://"+abs, into)
	if rc, out := runArgv("", "git", args...); rc != 0 {
		return fmt.Errorf("git clone failed (%d): %s", rc, strings.TrimSpace(out))
	}
	// Remove every remote so the sandbox has no route to origin.
	rc, out := runArgv(into, "git", "remote")
	if rc != 0 {
		return fmt.Errorf("git remote list failed (%d): %s", rc, strings.TrimSpace(out))
	}
	for _, name := range strings.Fields(out) {
		if rc, o := runArgv(into, "git", "remote", "remove", name); rc != 0 {
			return fmt.Errorf("git remote remove %s failed (%d): %s", name, rc, strings.TrimSpace(o))
		}
	}
	if rc, o := runArgv(into, "git", "remote"); rc != 0 || strings.TrimSpace(o) != "" {
		return fmt.Errorf("sandbox still has a remote after hardening: %q", strings.TrimSpace(o))
	}
	// No host hooks, no inherited credentials -- checked, not swallowed.
	if rc, o := runArgv(into, "git", "config", "core.hooksPath", "/dev/null"); rc != 0 {
		return fmt.Errorf("git config core.hooksPath failed (%d): %s", rc, strings.TrimSpace(o))
	}
	if rc, o := runArgv(into, "git", "config", "credential.helper", ""); rc != 0 {
		return fmt.Errorf("git config credential.helper failed (%d): %s", rc, strings.TrimSpace(o))
	}
	return nil
}

// execZoneArgv: the untrusted check/build runs with NO network, read-only root,
// ephemeral tmpfs, sharing only the work volume. The "--" stops docker flag
// parsing so a hostile image name cannot inject docker flags.
func execZoneArgv(cloneAbs, image, check string) []string {
	a := []string{"run", "--rm", "--network", "none", "--read-only", "--tmpfs", "/tmp",
		"-v", cloneAbs + ":/work:rw", "-w", "/work", "--", image}
	if strings.TrimSpace(check) != "" {
		a = append(a, "sh", "-c", check)
	}
	return a
}

// agentZoneArgv: the reasoning agent reaches ONLY the model endpoint via the
// egress proxy, with its credential injected through a 0600 --env-file (never
// on the argv / process list), and no $HOME on disk. "--" stops flag parsing.
func agentZoneArgv(cloneAbs, image, agentNet, envFile, exec string) []string {
	a := []string{"run", "--rm", "--network", agentNet, "--read-only", "--tmpfs", "/tmp",
		"-v", cloneAbs + ":/work:rw", "--env-file", envFile, "-w", "/work", "--", image}
	if strings.TrimSpace(exec) != "" {
		a = append(a, "sh", "-c", exec)
	}
	return a
}

func under(base, p string) bool {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func newIsolateCmd() *cobra.Command {
	var repo, branch, workdir, runID, agentImage, execImage, agentNet, proxy, keyEnv string
	var execCmd, checkCmd, mintCmd, revokeCmd, runtime string
	var egressAllow []string
	var execute, confirm bool

	cmd := &cobra.Command{
		Use:          "isolate",
		Short:        "Two-zone isolation: detached clone + per-run minted key + exec/agent zone launch plan",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			if repo == "" {
				repo = workdir
			}
			if runID == "" {
				runID = "local"
			}
			if agentImage == "" {
				agentImage = "loopexec/agent"
			}
			if execImage == "" {
				execImage = "loopexec/exec"
			}
			if agentNet == "" {
				agentNet = "loopexec-agent-net"
			}
			if proxy == "" {
				proxy = "http://egress-proxy:8080"
			}
			if keyEnv == "" {
				keyEnv = "ANTHROPIC_API_KEY"
			}
			if runtime == "" {
				runtime = "docker"
			}
			if len(egressAllow) == 0 {
				egressAllow = []string{"api.anthropic.com:443"}
			}

			// Fail closed on inputs that become paths or docker flags.
			if !runIDRe.MatchString(runID) || strings.Contains(runID, "..") {
				return failResponse(cmd, runID, exitWorkspaceInvalid, "workspace_invalid",
					"invalid --run-id: must match [A-Za-z0-9._-]{1,64} and contain no '/', '\\', or '..'")
			}
			for _, img := range []string{execImage, agentImage} {
				if !imageRe.MatchString(img) {
					return failResponse(cmd, runID, exitWorkspaceInvalid, "isolation_unsatisfiable",
						fmt.Sprintf("invalid image %q (must not start with '-')", img))
				}
			}
			if strings.HasPrefix(runtime, "-") || strings.ContainsAny(runtime, " \t\n") {
				return failResponse(cmd, runID, exitWorkspaceInvalid, "isolation_unsatisfiable",
					"invalid --runtime")
			}

			lxDir := filepath.Join(workdir, ".loopexec")
			sandbox := filepath.Join(lxDir, "sandbox-"+runID)
			cloneDir := filepath.Join(sandbox, "work")
			envFile := filepath.Join(sandbox, "agent.env")

			// Containment: the sandbox MUST live under .loopexec before any rm.
			absLx, _ := filepath.Abs(lxDir)
			absSandbox, _ := filepath.Abs(sandbox)
			if !under(absLx, absSandbox) {
				return failResponse(cmd, runID, exitWorkspaceInvalid, "workspace_invalid",
					"sandbox path escapes .loopexec")
			}

			if err := os.RemoveAll(sandbox); err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "cannot reset sandbox", Cause: err}
			}
			if err := os.MkdirAll(sandbox, 0o755); err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "cannot create sandbox", Cause: err}
			}
			if err := detachedClone(repo, branch, cloneDir); err != nil {
				os.RemoveAll(sandbox) // no partial sandbox on failure
				return &cliError{Code: exitWorkspaceInvalid, Message: "detached clone failed", Cause: err}
			}
			cloneAbs, _ := filepath.Abs(cloneDir)

			execArgv := execZoneArgv(cloneAbs, execImage, checkCmd)
			agentArgv := agentZoneArgv(cloneAbs, agentImage, agentNet, envFile, execCmd)

			report := &isolationReport{
				Sandbox:      sandbox,
				Clone:        cloneAbs,
				AgentImage:   agentImage,
				ExecImage:    execImage,
				ExecZoneCmd:  runtime + " " + strings.Join(execArgv, " "),
				AgentZoneCmd: runtime + " " + strings.Join(agentArgv, " "),
				EgressAllow:  egressAllow,
				KeyEnv:       keyEnv,
				Hardened:     true,
			}

			rcExec, rcAgent := 0, 0
			if execute {
				if !confirm {
					return &cliError{Code: exitInvariantFailed,
						Message: "isolate --execute launches containers; pass --confirm to proceed"}
				}
				// A minted key with no way to revoke it is fail-open.
				if strings.TrimSpace(mintCmd) != "" && strings.TrimSpace(revokeCmd) == "" {
					return failResponse(cmd, runID, exitWorkspaceInvalid, "isolation_unsatisfiable",
						"--mint-cmd requires --revoke-cmd (a minted key must be revocable)")
				}

				key := ""
				if strings.TrimSpace(mintCmd) != "" {
					keyOut, merr := exec.Command("sh", "-c", mintCmd).Output() // stdout only
					if merr != nil {
						return failResponse(cmd, runID, exitWorkspaceInvalid, "isolation_unsatisfiable", "credential mint failed")
					}
					key = strings.TrimSpace(string(keyOut))
					report.Minted = key != ""
				}

				// Register revoke + cleanup BEFORE writing the key to disk, so
				// every post-mint error path still revokes and scrubs.
				revoke := func() {
					if report.Minted && !report.Revoked && strings.TrimSpace(revokeCmd) != "" {
						rc := exec.Command("sh", "-c", revokeCmd)
						rc.Env = append(os.Environ(), keyEnv+"="+key) // via env, not argv
						_ = rc.Run()
						report.Revoked = true
					}
				}
				cleanup := func() { _ = os.Remove(envFile); revoke() }
				defer cleanup()

				// Catchable signals (Ctrl-C, watch's SIGTERM) must still scrub +
				// revoke; SIGKILL can't be caught (short-TTL keys are the backstop).
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				defer signal.Stop(sigCh)
				go func() {
					s, ok := <-sigCh
					if !ok {
						return
					}
					cancel()
					cleanup()
					if p, e := os.FindProcess(os.Getpid()); e == nil {
						_ = p.Signal(s)
					}
				}()

				// A newline in the minted key would inject extra env-file lines.
				if strings.ContainsAny(key, "\r\n") {
					return failResponse(cmd, runID, exitWorkspaceInvalid, "isolation_unsatisfiable",
						"minted key contains a newline")
				}
				envBody := keyEnv + "=" + key + "\nHTTPS_PROXY=" + proxy + "\nHTTP_PROXY=" + proxy + "\nNO_PROXY=\n"
				if werr := os.WriteFile(envFile, []byte(envBody), 0o600); werr != nil {
					return &cliError{Code: exitWorkspaceInvalid, Message: "cannot write agent env-file", Cause: werr}
				}

				rcExec = runRuntime(ctx, runtime, execArgv)
				rcAgent = runRuntime(ctx, runtime, agentArgv)
				report.ExecZoneExit = &rcExec
				report.AgentZoneExit = &rcAgent
				report.Executed = true

				if rerr := os.Remove(envFile); rerr != nil && !os.IsNotExist(rerr) {
					revoke()
					return &cliError{Code: exitWorkspaceInvalid, Message: "could not remove agent env-file (credential may be on disk)", Cause: rerr}
				}
				revoke()
			}

			if rerr := writeIsolationReceipt(lxDir, runID, report); rerr != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "cannot write isolation receipt", Cause: rerr}
			}

			// Surface a failed zone instead of swallowing it.
			status, halt, code := "ok", "", 0
			if report.Executed {
				switch {
				case rcExec == -1 || rcAgent == -1:
					status, halt, code = "error", "isolation_unsatisfiable", exitWorkspaceInvalid
				case rcExec != 0 || rcAgent != 0:
					status, halt, code = "error", "execution_failure", exitExecutionFailure
				}
			}
			r := response{Tool: toolName, Version: toolVersion, Status: status, RunID: runID,
				HaltReason: halt, Errors: []string{}, Isolation: report}
			if code != 0 {
				r.Errors = []string{fmt.Sprintf("isolation zone failed: exec_zone_exit=%d agent_zone_exit=%d", rcExec, rcAgent)}
			}
			if perr := printResponse(cmd, r); perr != nil {
				return perr
			}
			if !jsonOutput {
				mode := "plan"
				if report.Executed {
					mode = "executed"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "isolate (%s): clone=%s hardened=%t minted=%t revoked=%t\n",
					mode, cloneAbs, report.Hardened, report.Minted, report.Revoked)
				fmt.Fprintf(cmd.OutOrStdout(), "  exec-zone:  %s\n", report.ExecZoneCmd)
				fmt.Fprintf(cmd.OutOrStdout(), "  agent-zone: %s\n", report.AgentZoneCmd)
				if !report.Executed {
					fmt.Fprintf(cmd.ErrOrStderr(), "note: plan only; on --execute, loopexec mints the key into a 0600 %s and revokes it after. egress confinement requires an operator-provisioned internal network + allowlist proxy.\n", envFile)
				}
			}
			if code != 0 {
				return &cliError{Code: code, Message: "halted: " + halt}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository to clone into the sandbox (default: --workdir)")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch to check out (default: the repo default)")
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory holding .loopexec (default: current directory)")
	cmd.Flags().StringVar(&runID, "run-id", "", "Run identifier (slug)")
	cmd.Flags().StringVar(&execCmd, "exec", "", "Agent/work command to run in the agent zone")
	cmd.Flags().StringVar(&checkCmd, "check", "", "Check command to run in the exec zone")
	cmd.Flags().StringVar(&agentImage, "agent-image", "loopexec/agent", "Agent-zone container image")
	cmd.Flags().StringVar(&execImage, "exec-image", "loopexec/exec", "Exec-zone container image")
	cmd.Flags().StringVar(&agentNet, "agent-network", "loopexec-agent-net", "Agent-zone network (operator-provisioned internal net; egress via the proxy)")
	cmd.Flags().StringVar(&proxy, "egress-proxy", "http://egress-proxy:8080", "Auditing forward proxy the agent zone routes through")
	cmd.Flags().StringArrayVar(&egressAllow, "egress-allow", nil, "Allowed egress host:port (default api.anthropic.com:443); RECORDED in the receipt, enforced by the operator's allowlist proxy")
	cmd.Flags().StringVar(&keyEnv, "key-env", "ANTHROPIC_API_KEY", "Env var the minted credential is injected as")
	cmd.Flags().StringVar(&mintCmd, "mint-cmd", "", "Operator hook that prints a per-run scoped, spend-capped key to stdout (requires --revoke-cmd)")
	cmd.Flags().StringVar(&revokeCmd, "revoke-cmd", "", "Operator hook that revokes the key (receives it via the key-env env var)")
	cmd.Flags().StringVar(&runtime, "runtime", "docker", "Container runtime used to launch the zones")
	cmd.Flags().BoolVar(&execute, "execute", false, "Launch the zones (default: render the plan only)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm launching containers (required with --execute)")
	return cmd
}

// runRuntime launches a zone via the container runtime under a context so the
// child is killed if loopexec is signalled (it was handed the credential).
func runRuntime(ctx context.Context, name string, args []string) int {
	c := exec.CommandContext(ctx, name, args...)
	_ = c.Run()
	if c.ProcessState != nil {
		return c.ProcessState.ExitCode()
	}
	return -1
}

// writeIsolationReceipt persists the redacted isolation report (never the key).
func writeIsolationReceipt(lxDir, runID string, report *isolationReport) error {
	if err := os.MkdirAll(lxDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(lxDir, "isolation-"+runID+".json"), append(data, '\n'), 0o644)
}
