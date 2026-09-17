package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/subprocess"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// normalizeOutput strips trailing whitespace/newlines so a stable check's
// fingerprint is robust to trivial output noise.
func normalizeOutput(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// buildManifest hashes each context file (path + sha256) for the receipt so the
// prompt context is reconstructable (SPEC.md section 8).
func buildManifest(workdir string, files []string) []manifestEntry {
	var man []manifestEntry
	for _, f := range files {
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(workdir, f)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			man = append(man, manifestEntry{Path: f, SHA256: "missing"})
			continue
		}
		man = append(man, manifestEntry{Path: f, SHA256: sha256hex(data)})
	}
	return man
}

// canonicalReceipt is the deterministic byte form signed by attest and
// reconstructed for verification: only the pinned, output-determining fields.
func canonicalReceipt(st loopState) ([]byte, error) {
	type rcpt struct {
		Workflow     *workflow.Config `json:"workflow,omitempty"`
		WorkflowHash string           `json:"workflow_hash,omitempty"`
		Budget       *budgetBook      `json:"budget,omitempty"`
		Numeric      *numericState    `json:"numeric,omitempty"`
		Candidate    *candidateState  `json:"candidate,omitempty"`

		RunID           string            `json:"run_id"`
		HaltReason      string            `json:"halt_reason"`
		Iteration       int               `json:"iteration"`
		Check           string            `json:"check"`
		Model           *modelPin         `json:"model,omitempty"`
		Sampling        *samplingPin      `json:"sampling,omitempty"`
		ContextManifest []manifestEntry   `json:"context_manifest,omitempty"`
		CostUSD         float64           `json:"cost_usd"`
		Fingerprint     *checkFingerprint `json:"fingerprint,omitempty"`
	}
	return json.Marshal(rcpt{Workflow: st.Workflow, WorkflowHash: st.WorkflowHash, Budget: st.Budget, Numeric: st.Numeric, Candidate: st.Candidate,
		RunID:           st.RunID,
		HaltReason:      st.HaltReason,
		Iteration:       st.Iteration,
		Check:           st.Check,
		Model:           st.Model,
		Sampling:        st.Sampling,
		ContextManifest: st.ContextManifest,
		CostUSD:         st.CostUSD,
		Fingerprint:     st.Fingerprint,
	})
}

func attestKey(flagKey string) []byte {
	if flagKey != "" {
		return []byte(flagKey)
	}
	if env := os.Getenv("LOOPEXEC_ATTEST_KEY"); env != "" {
		return []byte(env)
	}
	return []byte("loopexec-dev-attest-key")
}

func signReceipt(st loopState, key []byte) (string, error) {
	canon, err := canonicalReceipt(st)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(canon)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func statePath(workdir string) string {
	return filepath.Join(workdir, ".loopexec", "state.json")
}

// perRunStateName is the per-run state snapshot written alongside run-<id>.jsonl
// so a specific run can be verified by --run-id after later runs overwrite
// state.json (the latest-run pointer).
func perRunStateName(runID string) string {
	return "run-" + runID + ".state.json"
}

// resolveStatePath maps an optional --run-id to the state file to read. Empty
// run-id => the latest run (state.json). A run-id => its per-run snapshot, after
// validating the id (runIDRe) so it cannot escape .loopexec via path traversal.
func resolveStatePath(workdir, runID string) (string, error) {
	if runID == "" {
		return statePath(workdir), nil
	}
	if !validRunID(runID) {
		return "", &cliError{Code: exitWorkspaceInvalid,
			Message: "invalid --run-id (allowed: letters, digits, '.', '_', '-'; max 64 chars)"}
	}
	return filepath.Join(workdir, ".loopexec", perRunStateName(runID)), nil
}

// noStateMsg names the run in a not-found error so a typo'd --run-id is obvious.
func noStateMsg(runID string) string {
	if runID != "" {
		return "no recorded state for run-id '" + runID + "' (did this run complete in this workdir?)"
	}
	return "no recorded run state"
}

// verifyStateFingerprint is the shared replay verifier. Both `replay` and the
// built-in demo use this path so the first-run proof cannot drift from the
// receipt-verification contract it is intended to demonstrate.
func verifyStateFingerprint(st loopState, fallbackWorkdir string) (checkFingerprint, bool, error) {
	if st.Workflow != nil {
		return verifyWorkflowFingerprint(st, fallbackWorkdir)
	}
	if st.Check == "" || st.Fingerprint == nil {
		return checkFingerprint{}, false, fmt.Errorf("receipt has no check fingerprint to replay")
	}
	runDir := st.Workdir
	if runDir == "" {
		runDir = fallbackWorkdir
	}
	rc, out := runShell(runDir, st.Check)
	got := checkFingerprint{ExitCode: rc, OutputSHA256: sha256hex([]byte(normalizeOutput(out)))}
	match := got.ExitCode == st.Fingerprint.ExitCode && got.OutputSHA256 == st.Fingerprint.OutputSHA256
	return got, match, nil
}

// newReplayCmd VERIFIES a recorded receipt: re-run the recorded check against
// the current end-state and confirm the fingerprint matches. Agent-free and
// budget-free (SPEC.md section 8) -- it never re-runs the agent. This is the
// "verify a verdict without re-running the agent" half; reexecute is the live,
// non-deterministic re-run.
func newReplayCmd() *cobra.Command {
	var workdir, runID string
	cmd := &cobra.Command{
		Use:          "replay",
		Short:        "VERIFY a recorded receipt: re-run the check and confirm the fingerprint (agent-free)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			sp, perr := resolveStatePath(workdir, runID)
			if perr != nil {
				return perr
			}
			st, err := readState(sp)
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: noStateMsg(runID), Cause: err}
			}
			got, match, verifyErr := verifyStateFingerprint(st, workdir)
			if verifyErr != nil {
				return failResponse(cmd, st.RunID, exitIntegrity, "objective_unverified", verifyErr.Error())
			}

			r := response{Tool: toolName, Version: toolVersion, RunID: st.RunID, Verified: &match, Errors: []string{}}
			if match {
				r.Status = "verified"
				return printResponse(cmd, r)
			}
			r.Status = "error"
			r.HaltReason = "objective_unverified"
			r.Errors = []string{fmt.Sprintf("receipt fingerprint mismatch: got exit=%d hash=%s", got.ExitCode, shortHash(got.OutputSHA256))}
			if err := printResponse(cmd, r); err != nil {
				return err
			}
			return &cliError{Code: exitIntegrity, Message: "objective_unverified", Silent: true}
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec (default: current directory)")
	cmd.Flags().StringVar(&runID, "run-id", "", "Verify a specific recorded run by id (default: the latest run)")
	return cmd
}

// newAttestCmd signs the receipt (HMAC-SHA256) so provenance is checkable, or
// verifies an existing signature with --verify.
func newAttestCmd() *cobra.Command {
	var workdir, key, runID string
	var verify bool
	cmd := &cobra.Command{
		Use:          "attest",
		Short:        "Sign the receipt (HMAC) so provenance is verifiable; --verify to check a signature",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			sp, perr := resolveStatePath(workdir, runID)
			if perr != nil {
				return perr
			}
			st, err := readState(sp)
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: noStateMsg(runID), Cause: err}
			}
			sig, err := signReceipt(st, attestKey(key))
			if err != nil {
				return &cliError{Code: exitInternalError, Message: "cannot sign receipt", Cause: err}
			}
			sigPath := filepath.Join(workdir, ".loopexec", "attest-"+st.RunID+".sig")

			if verify {
				stored, rerr := os.ReadFile(sigPath)
				ok := rerr == nil && hmac.Equal([]byte(strings.TrimSpace(string(stored))), []byte(sig))
				r := response{Tool: toolName, Version: toolVersion, RunID: st.RunID, Verified: &ok, Errors: []string{}}
				if ok {
					r.Status = "verified"
					return printResponse(cmd, r)
				}
				r.Status = "error"
				r.HaltReason = "objective_unverified"
				r.Errors = []string{"attestation signature does not match (wrong key or tampered receipt)"}
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return &cliError{Code: exitIntegrity, Message: "attestation mismatch", Silent: true}
			}

			if err := os.WriteFile(sigPath, []byte(sig+"\n"), 0o644); err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "cannot write signature", Cause: err}
			}
			r := response{Tool: toolName, Version: toolVersion, Status: "attested", RunID: st.RunID, Signature: sig, Errors: []string{}}
			return printResponse(cmd, r)
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec (default: current directory)")
	cmd.Flags().StringVar(&key, "key", "", "Attestation key (else $LOOPEXEC_ATTEST_KEY, else a dev default)")
	cmd.Flags().StringVar(&runID, "run-id", "", "Attest a specific recorded run by id (default: the latest run)")
	cmd.Flags().BoolVar(&verify, "verify", false, "Verify the stored signature instead of creating one")
	return cmd
}

func verifyWorkflowFingerprint(st loopState, fallback string) (checkFingerprint, bool, error) {
	root := st.Workdir
	if root == "" {
		root = fallback
	}
	fp := st.Fingerprint
	iter := st.Iteration
	if st.Candidate != nil {
		if st.Candidate.Best == "" || !st.Candidate.Exposed {
			return checkFingerprint{}, false, fmt.Errorf("no best candidate is exposed")
		}
		fp = st.Candidate.BestFingerprint
		iter = st.Candidate.BestIteration
		for _, pin := range st.Candidate.Protected {
			p, e := workflow.SafePath(root, pin.Path)
			if e != nil {
				return checkFingerprint{}, false, e
			}
			hash, _, e := workflow.FileHash(context.Background(), p)
			if e != nil || hash != pin.SHA256 {
				return checkFingerprint{}, false, fmt.Errorf("protected policy drift")
			}
		}
	}
	if fp == nil || len(st.Workflow.Check) == 0 {
		return checkFingerprint{}, false, fmt.Errorf("no completed workflow check")
	}
	requestPath := filepath.Join(root, ".loopexec", st.RunID, fmt.Sprintf("request-%06d-check.json", iter))
	var req workflow.Request
	if e := workflow.Read(requestPath, &req); e != nil {
		return checkFingerprint{}, false, e
	}
	if req.RunID != st.RunID || req.Iteration != iter || req.ConfigHash != st.WorkflowHash || workflow.JSONHash(st.Workflow) != st.WorkflowHash {
		return checkFingerprint{}, false, fmt.Errorf("replay request drift")
	}
	if st.Candidate != nil && req.CandidateID != st.Candidate.Best {
		return checkFingerprint{}, false, fmt.Errorf("replay candidate drift")
	}
	timeout := time.Duration(st.CommandTimeoutNS)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	args := append(append([]string{}, st.Workflow.Check...), requestPath)
	r := subprocess.Run(context.Background(), subprocess.Options{Dir: root, Argv: args, Timeout: timeout, Grace: time.Duration(st.GraceNS)})
	got := checkFingerprint{r.ExitCode, sha256hex([]byte(normalizeOutput(r.Stdout)))}
	if r.Cause != "" || r.Truncated {
		return got, false, fmt.Errorf("replay command unavailable")
	}
	return got, got == *fp, nil
}
