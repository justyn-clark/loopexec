package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// reportSummary is the rendered digest of a recorded receipt. report re-runs
// nothing (unlike replay / reexecute): it reads the durable state plus the
// typed JSONL event log and presents them. It is the read-only audit view.
type reportSummary struct {
	Budget            *budgetBook       `json:"budget,omitempty"`
	Numeric           *numericState     `json:"numeric,omitempty"`
	Candidate         *candidateState   `json:"candidate,omitempty"`
	BestCandidate     string            `json:"best_candidate,omitempty"`
	Phase             string            `json:"phase"`
	ExitCode          int               `json:"exit_code"`
	Iterations        int               `json:"iterations"`
	Check             string            `json:"check,omitempty"`
	Workdir           string            `json:"workdir,omitempty"`
	CostUSD           float64           `json:"cost_usd,omitempty"`
	Model             *modelPin         `json:"model,omitempty"`
	Sampling          *samplingPin      `json:"sampling,omitempty"`
	ContextFiles      int               `json:"context_files,omitempty"`
	Fingerprint       *checkFingerprint `json:"fingerprint,omitempty"`
	Events            int               `json:"events"`
	Receipt           string            `json:"receipt,omitempty"`
	ReceiptStatus     string            `json:"receipt_status"`
	Warnings          []string          `json:"warnings"`
	Timeline          []receiptEvent    `json:"timeline"`
	ScoreTarget       *float64          `json:"score_target,omitempty"`
	AttestationStatus string            `json:"attestation_status"`
	// Attested preserves the legacy signature-file-presence flag, not verification.
	Attested bool `json:"attested"`
}

// Keep readable evidence from a damaged receipt, but never conceal the gaps.
// "readable" describes parsing only; it is not an integrity verification.
func readReceiptEvents(path, runID string) ([]receiptEvent, []string, string) {
	evs := []receiptEvent{}
	warnings := []string{}
	f, err := os.Open(path)
	if err != nil {
		status := "unreadable"
		if os.IsNotExist(err) {
			status = "missing"
		}
		return evs, []string{fmt.Sprintf("receipt %s; timeline unavailable: %v", status, err)}, status
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return evs, []string{"receipt is not a readable regular file; timeline unavailable"}, "unreadable"
	}
	invalid, lineNumber := 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		lineNumber++
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e receiptEvent
		if err := json.Unmarshal(line, &e); err != nil || e.RunID != runID || strings.TrimSpace(e.Event) == "" {
			invalid++
			if invalid <= 16 {
				warnings = append(warnings, fmt.Sprintf("receipt line %d is malformed or has an invalid event/run identity; excluded from timeline", lineNumber))
			}
			continue
		}
		evs = append(evs, e)
	}
	if invalid > 16 {
		warnings = append(warnings, fmt.Sprintf("%d additional invalid receipt lines excluded", invalid-16))
	}
	if err := sc.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("receipt read stopped after line %d: %v", lineNumber, err))
	}
	if len(evs) == 0 {
		warnings = append(warnings, "receipt contains no readable events")
	}
	status := "readable"
	if len(warnings) > 0 {
		status = "incomplete"
	}
	return evs, warnings, status
}

// newReportCmd renders a recorded run: its outcome, its receipt pins, and the
// per-iteration timeline. Agent-free and side-effect-free; it always exits 0
// when a run exists, regardless of how that run halted.
func newReportCmd() *cobra.Command {
	var workdir, runID string
	cmd := &cobra.Command{
		Use:          "report",
		Short:        "Render a recorded receipt (state + JSONL event log); re-runs nothing",
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

			receiptPath := filepath.Join(workdir, ".loopexec", "run-"+st.RunID+".jsonl")
			events, warnings, receiptStatus := readReceiptEvents(receiptPath, st.RunID)
			displayReceipt := receiptPath
			if _, statErr := os.Stat(receiptPath); statErr != nil {
				displayReceipt = ""
			}
			attestInfo, attestErr := os.Stat(filepath.Join(workdir, ".loopexec", "attest-"+st.RunID+".sig"))
			attestationStatus := "absent"
			if attestErr == nil && attestInfo.Mode().IsRegular() {
				attestationStatus = "present_unverified"
			} else if !os.IsNotExist(attestErr) {
				attestationStatus = "unreadable"
				warnings = append(warnings, "attestation is not a readable signature file")
			}

			sum := &reportSummary{Budget: st.Budget, Numeric: st.Numeric, Candidate: st.Candidate, BestCandidate: st.BestCandidatePath,
				Phase:             st.Phase,
				ExitCode:          haltExitCode(st.HaltReason),
				Iterations:        st.Iteration,
				Check:             st.Check,
				Workdir:           st.Workdir,
				CostUSD:           st.CostUSD,
				Model:             st.Model,
				Sampling:          st.Sampling,
				ContextFiles:      len(st.ContextManifest),
				Fingerprint:       st.Fingerprint,
				Events:            len(events),
				Receipt:           displayReceipt,
				ReceiptStatus:     receiptStatus,
				Warnings:          warnings,
				Timeline:          events,
				AttestationStatus: attestationStatus,
				Attested:          attestErr == nil,
			}
			if st.Workflow != nil && st.Workflow.Score != nil {
				target := st.Workflow.Score.Target
				sum.ScoreTarget = &target
			}

			r := response{
				FailureCause: st.FailureCause,
				Tool:         toolName,
				Version:      toolVersion,
				Status:       "ok",
				RunID:        st.RunID,
				Iteration:    st.Iteration,
				HaltReason:   st.HaltReason,
				Receipt:      displayReceipt,
				Report:       sum,
				Errors:       []string{},
			}
			if jsonOutput {
				return printResponse(cmd, r)
			}
			renderReport(cmd, st, sum, events)
			return nil
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec (default: current directory)")
	cmd.Flags().StringVar(&runID, "run-id", "", "Report a specific recorded run by id (default: the latest run)")
	return cmd
}

// renderReport writes the human digest: header, pins, then the iteration timeline.
func renderReport(cmd *cobra.Command, st loopState, sum *reportSummary, events []receiptEvent) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s %s\n", toolName, toolVersion)
	fmt.Fprintf(w, "report: run %q\n", st.RunID)

	halt := st.HaltReason
	if halt == "" {
		halt = "(none)"
	}
	fmt.Fprintf(w, "phase: %s (%s, exit %d)\n", st.Phase, halt, sum.ExitCode)
	if st.FailureCause != "" {
		fmt.Fprintf(w, "failure cause: %s\n", st.FailureCause)
	}
	fmt.Fprintf(w, "iterations: %d\n", st.Iteration)
	if st.Check != "" {
		fmt.Fprintf(w, "check: %s\n", st.Check)
	}
	if st.Workdir != "" {
		fmt.Fprintf(w, "workdir: %s\n", st.Workdir)
	}
	if st.Fingerprint != nil {
		fmt.Fprintf(w, "fingerprint: exit %d, sha256 %s\n", st.Fingerprint.ExitCode, shortHash(st.Fingerprint.OutputSHA256))
	}
	if st.Budget == nil {
		fmt.Fprintf(w, "cost: $%.2f\n", st.CostUSD)
	} else {
		if st.Budget.MonetaryKnown {
			fmt.Fprintf(w, "metered cost: $%.6f (%s)\n", float64(st.Budget.MicroUSD)/1e6, st.Budget.Mode)
		} else {
			fmt.Fprintf(w, "monetary usage: unknown (%s)\n", st.Budget.Mode)
		}
		fmt.Fprintf(w, "calls: %d, tokens: %d, pending usage: %t\n", st.Budget.Calls, st.Budget.Tokens, st.Budget.Pending != nil)
	}
	if st.BestCandidatePath != "" {
		fmt.Fprintf(w, "best candidate: %s\n", st.BestCandidatePath)
	}
	if sum.Numeric != nil {
		if sum.Numeric.Best != nil {
			fmt.Fprintf(w, "best retained score (this run): %.3f\n", *sum.Numeric.Best)
		}
		if sum.ScoreTarget != nil {
			fmt.Fprintf(w, "score target: %.3f\n", *sum.ScoreTarget)
		}
		fmt.Fprintf(w, "reviews: %d; reviews since improvement: %d\n", sum.Numeric.Reviewed, sum.Numeric.Stale)
	}
	if sum.Candidate != nil {
		fmt.Fprintf(w, "candidate retention: %s\n", sum.Candidate.Status)
		if st.HaltReason != "success_condition_met" {
			fmt.Fprintln(w, "acceptance: run did not converge; candidate retention does not establish acceptance")
		}
	}

	if st.Model != nil {
		fmt.Fprintf(w, "model: %s/%s %s\n", st.Model.Provider, st.Model.ID, st.Model.Version)
	}
	if sum.ContextFiles > 0 {
		fmt.Fprintf(w, "context files: %d\n", sum.ContextFiles)
	}
	attestation := sum.AttestationStatus
	if attestation == "present_unverified" {
		attestation = "signature present (not verified)"
	}
	fmt.Fprintf(w, "attestation: %s\n", attestation)
	fmt.Fprintf(w, "receipt status: %s (parsing only; not integrity verification)\n", sum.ReceiptStatus)
	for _, warning := range sum.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}

	if sum.Receipt == "" {
		fmt.Fprintln(w, "receipt: (none on disk)")
		return
	}
	fmt.Fprintf(w, "receipt: %s (%d events)\n", sum.Receipt, sum.Events)
	fmt.Fprintln(w, "\ntimeline:")
	for _, e := range events {
		line := fmt.Sprintf("  [iter %d] %-10s", e.Iteration, e.Event)
		if e.Phase != "" {
			line += " phase=" + e.Phase
		}
		if e.ExitCode != nil {
			line += fmt.Sprintf(" exit %d", *e.ExitCode)
		}
		if e.Event != "command_start" && strings.HasPrefix(e.Event, "command_") {
			line += fmt.Sprintf(" duration=%dms", e.DurationMS)
		}
		if e.Cause != "" {
			line += " cause=" + e.Cause
		}
		if e.OutputTruncated {
			line += " output_truncated=true"
		}
		if e.Detail != "" {
			line += "  " + e.Detail
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
