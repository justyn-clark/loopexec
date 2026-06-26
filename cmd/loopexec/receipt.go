package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	return json.Marshal(rcpt{
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

// newReplayCmd VERIFIES a recorded receipt: re-run the recorded check against
// the current end-state and confirm the fingerprint matches. Agent-free and
// budget-free (SPEC.md section 8) -- it never re-runs the agent. This is the
// "verify a verdict without re-running the agent" half; reexecute is the live,
// non-deterministic re-run (Planned).
func newReplayCmd() *cobra.Command {
	var workdir string
	cmd := &cobra.Command{
		Use:          "replay",
		Short:        "VERIFY a recorded receipt: re-run the check and confirm the fingerprint (agent-free)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			st, err := readState(statePath(workdir))
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "no recorded run state", Cause: err}
			}
			if st.Check == "" || st.Fingerprint == nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "receipt has no check fingerprint to replay"}
			}
			runDir := st.Workdir
			if runDir == "" {
				runDir = workdir
			}
			rc, out := runShell(runDir, st.Check)
			got := checkFingerprint{ExitCode: rc, OutputSHA256: sha256hex([]byte(normalizeOutput(out)))}
			match := got.ExitCode == st.Fingerprint.ExitCode && got.OutputSHA256 == st.Fingerprint.OutputSHA256

			r := response{Tool: toolName, Version: toolVersion, RunID: st.RunID, Verified: &match, Errors: []string{}}
			if match {
				r.Status = "verified"
				return printResponse(cmd, r)
			}
			r.Status = "error"
			r.HaltReason = "objective_unverified"
			r.Errors = []string{fmt.Sprintf("receipt fingerprint mismatch: recorded exit=%d hash=%s, got exit=%d hash=%s",
				st.Fingerprint.ExitCode, st.Fingerprint.OutputSHA256[:12], got.ExitCode, got.OutputSHA256[:12])}
			if err := printResponse(cmd, r); err != nil {
				return err
			}
			return &cliError{Code: exitIntegrity, Message: "objective_unverified"}
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec/state.json (default: current directory)")
	return cmd
}

// newAttestCmd signs the receipt (HMAC-SHA256) so provenance is checkable, or
// verifies an existing signature with --verify.
func newAttestCmd() *cobra.Command {
	var workdir, key string
	var verify bool
	cmd := &cobra.Command{
		Use:          "attest",
		Short:        "Sign the receipt (HMAC) so provenance is verifiable; --verify to check a signature",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			st, err := readState(statePath(workdir))
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "no recorded run state", Cause: err}
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
				return &cliError{Code: exitIntegrity, Message: "attestation mismatch"}
			}

			if err := os.WriteFile(sigPath, []byte(sig+"\n"), 0o644); err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: "cannot write signature", Cause: err}
			}
			r := response{Tool: toolName, Version: toolVersion, Status: "attested", RunID: st.RunID, Signature: sig, Errors: []string{}}
			return printResponse(cmd, r)
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec/state.json (default: current directory)")
	cmd.Flags().StringVar(&key, "key", "", "Attestation key (else $LOOPEXEC_ATTEST_KEY, else a dev default)")
	cmd.Flags().BoolVar(&verify, "verify", false, "Verify the stored signature instead of creating one")
	return cmd
}
