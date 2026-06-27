package main

import (
	"encoding/json"
	"strings"
	"testing"
)

type reportJSON struct {
	RunID  string `json:"run_id"`
	Report *struct {
		Phase       string `json:"phase"`
		ExitCode    int    `json:"exit_code"`
		Iterations  int    `json:"iterations"`
		Check       string `json:"check"`
		Events      int    `json:"events"`
		Fingerprint *struct {
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
