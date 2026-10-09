package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type reportJSON struct {
	RunID        string `json:"run_id"`
	FailureCause string `json:"failure_cause"`
	Report       *struct {
		Phase             string         `json:"phase"`
		ExitCode          int            `json:"exit_code"`
		Iterations        int            `json:"iterations"`
		Check             string         `json:"check"`
		Events            int            `json:"events"`
		ReceiptStatus     string         `json:"receipt_status"`
		Warnings          []string       `json:"warnings"`
		Timeline          []receiptEvent `json:"timeline"`
		AttestationStatus string         `json:"attestation_status"`
		Fingerprint       *struct {
			ExitCode int `json:"exit_code"`
		} `json:"fingerprint"`
	} `json:"report"`
}

func parseReportJSON(t *testing.T, s string) reportJSON {
	t.Helper()
	var r reportJSON
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, s)
	}
	return r
}

// report renders a recorded run as a structured digest: outcome, pins, and a
// receipt event count - re-running nothing, exiting 0 even for a halted run.
func TestReportRendersRecordedRun(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "r", "--workdir", dir, "--check", "true")
	code, out, errOut := runCLI(t, bin, "report", "--json", "--workdir", dir)
	if code != 0 {
		t.Fatalf("report exit = %d, want 0; stderr=%q", code, errOut)
	}
	r := parseReportJSON(t, out)
	if r.Report == nil {
		t.Fatal("response missing report object")
	}
	if r.Report.Phase != "halted" || r.Report.ExitCode != 10 {
		t.Fatalf("report phase/exit = %s/%d, want halted/10", r.Report.Phase, r.Report.ExitCode)
	}
	if r.Report.Events == 0 {
		t.Fatal("report should count receipt events")
	}
	if r.Report.ReceiptStatus != "readable" || len(r.Report.Warnings) != 0 || r.Report.Events != len(r.Report.Timeline) {
		t.Fatalf("healthy receipt incorrectly summarized: %s", out)
	}
	if r.Report.Fingerprint == nil {
		t.Fatal("report should surface the check fingerprint")
	}
}

// report addresses any recorded run by --run-id, like replay / explain-halt /
// attest; without it, it reports the latest run.
func TestReportTargetsRunByID(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "first", "--workdir", dir, "--check", "true")
	runCLI(t, bin, "run", "--json", "--run-id", "second", "--workdir", dir, "--check", "false", "--max-iterations", "1")

	_, latest, _ := runCLI(t, bin, "report", "--json", "--workdir", dir)
	if r := parseReportJSON(t, latest); r.RunID != "second" {
		t.Fatalf("default report run_id = %q, want second (latest)", r.RunID)
	}
	_, first, _ := runCLI(t, bin, "report", "--json", "--workdir", dir, "--run-id", "first")
	if r := parseReportJSON(t, first); r.RunID != "first" || r.Report.ExitCode != 10 {
		t.Fatalf("report --run-id first = %q/%d, want first/10", r.RunID, r.Report.ExitCode)
	}
}

// The human render carries the iteration timeline and the halt.
func TestReportHumanTimeline(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "h", "--workdir", dir, "--check", "true")
	_, out, _ := runCLI(t, bin, "report", "--workdir", dir)
	for _, want := range []string{"timeline:", "success_condition_met", "halt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("human report missing %q\n%s", want, out)
		}
	}
}

func TestReportNoStateErrors(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	code, _, _ := runCLI(t, bin, "report", "--workdir", dir)
	if code != 30 {
		t.Fatalf("report with no state exit = %d, want 30", code)
	}
}

func TestReportWarnsOnDamagedReceipt(t *testing.T) {
	bin := buildTestBinary(t)
	for _, tc := range []struct {
		name, data, status string
		missing            bool
	}{
		{name: "malformed", data: "{broken\n", status: "incomplete"},
		{name: "null", data: "null\n", status: "incomplete"},
		{name: "empty object", data: "{}\n", status: "incomplete"},
		{name: "wrong run", data: `{"run_id":"other","event":"halt"}` + "\n", status: "incomplete"},
		{name: "missing", missing: true, status: "missing"},
		{name: "empty", status: "incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			runCLI(t, bin, "run", "--json", "--run-id", "r", "--workdir", dir, "--check", "true")
			path := filepath.Join(dir, ".loopexec", "run-r.jsonl")
			if tc.missing {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				data := tc.data
				if data != "" {
					data = `{"run_id":"r","iteration":1,"event":"check","exit_code":0}` + "\n" + data
				}
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			code, out, errOut := runCLI(t, bin, "report", "--json", "--workdir", dir)
			if code != 0 {
				t.Fatalf("report exit %d: %s", code, errOut)
			}
			r := parseReportJSON(t, out)
			if r.Report.ReceiptStatus != tc.status || len(r.Report.Warnings) == 0 {
				t.Fatalf("damaged receipt hidden: %s", out)
			}
			if tc.data != "" && (len(r.Report.Timeline) != 1 || r.Report.Timeline[0].Event != "check") {
				t.Fatalf("valid evidence not retained separately: %s", out)
			}
			_, human, _ := runCLI(t, bin, "report", "--workdir", dir)
			if !strings.Contains(human, "warning:") {
				t.Fatalf("human report hides warning: %s", human)
			}
		})
	}
}

func TestReportIncludesFailureEvidence(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	code, _, _ := runCLI(t, bin, "run", "--json", "--run-id", "timed", "--workdir", dir,
		"--exec", "sleep 2", "--check", "true", "--command-timeout", "40ms", "--terminate-grace", "10ms")
	if code != 40 {
		t.Fatalf("timeout exit %d, want 40", code)
	}
	code, out, errOut := runCLI(t, bin, "report", "--json", "--workdir", dir)
	if code != 0 {
		t.Fatalf("report exit %d: %s", code, errOut)
	}
	r := parseReportJSON(t, out)
	if r.FailureCause == "" || r.Report.ReceiptStatus != "readable" || r.Report.Events != len(r.Report.Timeline) {
		t.Fatalf("failure context missing: %s", out)
	}
	found := false
	for _, event := range r.Report.Timeline {
		if event.Event == "command_timeout" && event.Phase == "exec" && event.Cause == "deadline_exceeded" && event.ExitCode != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("timeout evidence missing: %s", out)
	}
	_, human, _ := runCLI(t, bin, "report", "--workdir", dir)
	for _, want := range []string{"failure cause:", "exec", "deadline_exceeded", "ms", "attestation: absent"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human report missing %q: %s", want, human)
		}
	}
}

func TestReportDoesNotClaimSignatureVerification(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "r", "--workdir", dir, "--check", "true")
	if err := os.WriteFile(filepath.Join(dir, ".loopexec", "attest-r.sig"), []byte("not a valid signature"), 0600); err != nil {
		t.Fatal(err)
	}
	_, out, _ := runCLI(t, bin, "report", "--json", "--workdir", dir)
	if r := parseReportJSON(t, out); r.Report.AttestationStatus != "present_unverified" {
		t.Fatalf("signature status missing: %s", out)
	}
	_, human, _ := runCLI(t, bin, "report", "--workdir", dir)
	if !strings.Contains(human, "signature present (not verified)") || strings.Contains(human, "attested: yes") {
		t.Fatalf("report implies verification: %s", human)
	}
}

func TestReportReceiptReadFailurePreservesEarlierEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt.jsonl")
	data := `{"run_id":"r","event":"check"}` + "\n" + strings.Repeat("x", 4*1024*1024) + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	events, warnings, status := readReceiptEvents(path, "r")
	if status != "incomplete" || len(events) != 1 || len(warnings) != 1 || !strings.Contains(warnings[0], "read stopped") {
		t.Fatalf("read failure hidden: events=%d warnings=%v status=%s", len(events), warnings, status)
	}
	events, warnings, status = readReceiptEvents(t.TempDir(), "r")
	if status != "unreadable" || len(events) != 0 || len(warnings) == 0 {
		t.Fatalf("nonregular receipt accepted: %s %v", status, warnings)
	}
}

func TestReportHumanShowsRetentionAndCommandDiagnostics(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	score, target := 8.1, 8.5
	exit := 1
	st := loopState{RunID: "r", Phase: "halted", HaltReason: "max_iterations_reached"}
	sum := &reportSummary{ExitCode: 12, Receipt: "receipt.jsonl", ReceiptStatus: "readable", AttestationStatus: "absent",
		Numeric: &numericState{Best: &score, Reviewed: 1}, ScoreTarget: &target, Candidate: &candidateState{Status: "accepted"}}
	renderReport(cmd, st, sum, []receiptEvent{{Event: "command_end", Phase: "check", ExitCode: &exit, DurationMS: 123, Cause: "nonzero_exit", OutputTruncated: true}})
	for _, want := range []string{"best retained score (this run): 8.100", "score target: 8.500", "candidate retention: accepted", "run did not converge", "candidate retention does not establish acceptance", "phase=check", "duration=123ms", "cause=nonzero_exit", "output_truncated=true"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out.String())
		}
	}
}
