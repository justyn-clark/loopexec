package main

import (
	"bufio"
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
	Phase        string            `json:"phase"`
	ExitCode     int               `json:"exit_code"`
	Iterations   int               `json:"iterations"`
	Check        string            `json:"check,omitempty"`
	Workdir      string            `json:"workdir,omitempty"`
	CostUSD      float64           `json:"cost_usd,omitempty"`
	Model        *modelPin         `json:"model,omitempty"`
	Sampling     *samplingPin      `json:"sampling,omitempty"`
	ContextFiles int               `json:"context_files,omitempty"`
	Fingerprint  *checkFingerprint `json:"fingerprint,omitempty"`
	Events       int               `json:"events"`
	Receipt      string            `json:"receipt,omitempty"`
	Attested     bool              `json:"attested"`
}

// readReceiptEvents parses the typed JSONL receipt. A missing receipt is not
// fatal (report still renders the recorded state); an unparseable line is
// skipped rather than failing the whole report.
func readReceiptEvents(path string) ([]receiptEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var evs []receiptEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e receiptEvent
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		evs = append(evs, e)
	}
	return evs, sc.Err()
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
			events, _ := readReceiptEvents(receiptPath)
			displayReceipt := receiptPath
			if _, statErr := os.Stat(receiptPath); statErr != nil {
				displayReceipt = ""
			}
			_, attestErr := os.Stat(filepath.Join(workdir, ".loopexec", "attest-"+st.RunID+".sig"))

			sum := &reportSummary{
				Phase:        st.Phase,
				ExitCode:     haltExitCode(st.HaltReason),
				Iterations:   st.Iteration,
				Check:        st.Check,
				Workdir:      st.Workdir,
				CostUSD:      st.CostUSD,
				Model:        st.Model,
				Sampling:     st.Sampling,
				ContextFiles: len(st.ContextManifest),
				Fingerprint:  st.Fingerprint,
				Events:       len(events),
				Receipt:      displayReceipt,
				Attested:     attestErr == nil,
			}

			r := response{
				Tool:       toolName,
				Version:    toolVersion,
				Status:     "ok",
				RunID:      st.RunID,
				Iteration:  st.Iteration,
				HaltReason: st.HaltReason,
				Receipt:    displayReceipt,
				Report:     sum,
				Errors:     []string{},
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
	fmt.Fprintf(w, "cost: $%.2f\n", st.CostUSD)
	if st.Model != nil {
		fmt.Fprintf(w, "model: %s/%s %s\n", st.Model.Provider, st.Model.ID, st.Model.Version)
	}
	if sum.ContextFiles > 0 {
		fmt.Fprintf(w, "context files: %d\n", sum.ContextFiles)
	}
	fmt.Fprintf(w, "attested: %s\n", yesNo(sum.Attested))

	if sum.Receipt == "" {
		fmt.Fprintln(w, "receipt: (none on disk)")
		return
	}
	fmt.Fprintf(w, "receipt: %s (%d events)\n", sum.Receipt, sum.Events)
	fmt.Fprintln(w, "\ntimeline:")
	for _, e := range events {
		line := fmt.Sprintf("  [iter %d] %-10s", e.Iteration, e.Event)
		if e.ExitCode != nil {
			line += fmt.Sprintf(" exit %d", *e.ExitCode)
		}
		if e.Event == "halt" && e.Detail != "" {
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

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
