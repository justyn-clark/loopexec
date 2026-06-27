package main

import (
	"encoding/json"
	"strings"
	"testing"
)

type uResp struct {
	Status     string `json:"status"`
	RunID      string `json:"run_id"`
	HaltReason string `json:"halt_reason"`
	Verdict    string `json:"verdict"`
	Verified   *bool  `json:"verified"`
}

func parseUResp(t *testing.T, s string) uResp {
	t.Helper()
	var r uResp
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, s)
	}
	return r
}

// A converged loop is a success, not an error: it MUST exit 10 and print
// nothing to stderr. Regression test for the "Error: halted:
// success_condition_met" line a converged run used to emit.
func TestConvergedRunIsSilentOnStderr(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	code, out, errOut := runCLI(t, bin, "run", "--json", "--run-id", "ok", "--workdir", dir, "--check", "true")
	if code != 10 {
		t.Fatalf("exit = %d, want 10 (converged)", code)
	}
	if strings.TrimSpace(errOut) != "" {
		t.Fatalf("stderr must be empty on a converged run, got: %q", errOut)
	}
	if r := parseUResp(t, out); r.HaltReason != "success_condition_met" {
		t.Fatalf("halt_reason = %q, want success_condition_met", r.HaltReason)
	}
}

// A computed halt (here the iteration cap) is an outcome, not an error: exit
// with the class code but never echo "Error:" to stderr.
func TestComputedHaltIsSilentOnStderr(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	code, _, errOut := runCLI(t, bin, "run", "--json", "--run-id", "cap", "--workdir", dir, "--check", "false", "--max-iterations", "1")
	if code != 12 {
		t.Fatalf("exit = %d, want 12 (max_iterations_reached)", code)
	}
	if strings.Contains(errOut, "Error:") {
		t.Fatalf("stderr must not contain 'Error:' for a computed halt, got: %q", errOut)
	}
}

// Silencing outcomes MUST NOT silence genuine errors: a command with no
// recorded state still surfaces its message on stderr.
func TestGenuineErrorStillPrintsToStderr(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	code, _, errOut := runCLI(t, bin, "replay", "--json", "--workdir", dir)
	if code != 30 {
		t.Fatalf("exit = %d, want 30", code)
	}
	if !strings.Contains(errOut, "no recorded run state") {
		t.Fatalf("a genuine error must print to stderr, got: %q", errOut)
	}
}

// replay / explain-halt / attest must target a specific run by --run-id, not
// just the latest run that state.json happens to point at.
func TestRunIDTargetsSpecificRun(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "good", "--workdir", dir, "--check", "true")
	runCLI(t, bin, "run", "--json", "--run-id", "capped", "--workdir", dir, "--check", "false", "--max-iterations", "1")

	// No --run-id resolves to the latest run (capped).
	_, latest, _ := runCLI(t, bin, "explain-halt", "--json", "--workdir", dir)
	if r := parseUResp(t, latest); r.RunID != "capped" {
		t.Fatalf("default explain-halt run_id = %q, want capped (latest)", r.RunID)
	}

	// --run-id reaches back to the earlier converged run.
	_, good, _ := runCLI(t, bin, "explain-halt", "--json", "--workdir", dir, "--run-id", "good")
	if rg := parseUResp(t, good); rg.RunID != "good" || rg.HaltReason != "success_condition_met" {
		t.Fatalf("explain-halt --run-id good = %+v, want good/success_condition_met", rg)
	}

	// replay verifies the targeted run's fingerprint offline.
	code, rep, _ := runCLI(t, bin, "replay", "--json", "--workdir", dir, "--run-id", "good")
	if rr := parseUResp(t, rep); code != 0 || rr.Status != "verified" || rr.RunID != "good" {
		t.Fatalf("replay --run-id good = code %d %+v, want 0/verified/good", code, rr)
	}

	// attest the targeted run, then verify that signature.
	runCLI(t, bin, "attest", "--json", "--workdir", dir, "--run-id", "good", "--key", "k")
	_, av, _ := runCLI(t, bin, "attest", "--json", "--workdir", dir, "--run-id", "good", "--key", "k", "--verify")
	if rv := parseUResp(t, av); rv.Verified == nil || !*rv.Verified {
		t.Fatalf("attest --verify --run-id good = %+v, want verified true", rv)
	}
}

// A traversal-y --run-id MUST be rejected, never resolved into a path that
// escapes .loopexec.
func TestRunIDInvalidRejected(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "x", "--workdir", dir, "--check", "true")
	code, _, errOut := runCLI(t, bin, "replay", "--json", "--workdir", dir, "--run-id", "../../etc/passwd")
	if code != 30 {
		t.Fatalf("exit = %d, want 30 for an invalid run-id", code)
	}
	if !strings.Contains(errOut, "invalid --run-id") {
		t.Fatalf("want 'invalid --run-id' message, got: %q", errOut)
	}
}
