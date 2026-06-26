package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type contractJSON struct {
	Tool       string   `json:"tool"`
	Version    string   `json:"version"`
	Status     string   `json:"status"`
	RunID      string   `json:"run_id"`
	Iteration  int      `json:"iteration"`
	HaltReason string   `json:"halt_reason"`
	CheckExit  *int     `json:"check_exit"`
	Receipt    string   `json:"receipt"`
	Errors     []string `json:"errors"`
}

func buildTestBinary(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "loopexec-test")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/loopexec")
	cmd.Dir = filepath.Clean(filepath.Join("..", ".."))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build test binary: %v\n%s", err, string(out))
	}
	return bin
}

func runCLI(t *testing.T, bin string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), stdout.String(), stderr.String()
	}
	t.Fatalf("failed to run cli: %v", err)
	return 0, "", ""
}

func parseSingleJSONObject(t *testing.T, s string) contractJSON {
	t.Helper()
	var obj contractJSON
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\nstdout=%q", err, s)
	}
	return obj
}

func TestContractInitJSON(t *testing.T) {
	bin := buildTestBinary(t)
	exit, stdout, _ := runCLI(t, bin, "init", "--json")
	if exit != 0 {
		t.Fatalf("init --json exit=%d, want 0", exit)
	}
	obj := parseSingleJSONObject(t, stdout)
	if obj.Tool != "loopexec" {
		t.Fatalf("tool=%q, want loopexec", obj.Tool)
	}
	if obj.Errors == nil {
		t.Fatal("errors must be an array, got null")
	}
}

// No check, no loop (SPEC.md O1): `run` without --check is a precondition
// failure, not a silent ok. This replaces the old flag-forced --halt-reason
// contract, which the spec demotes to a hidden test fixture.
func TestContractRunRequiresCheck(t *testing.T) {
	bin := buildTestBinary(t)
	exit, stdout, _ := runCLI(t, bin, "run", "--json")
	if exit != 30 {
		t.Fatalf("run without --check exit=%d, want 30", exit)
	}
	obj := parseSingleJSONObject(t, stdout)
	if obj.HaltReason != "workspace_invalid" {
		t.Fatalf("halt_reason=%q, want workspace_invalid", obj.HaltReason)
	}
	if len(obj.Errors) == 0 {
		t.Fatal("expected a non-empty errors array")
	}
}

func TestContractRunSuccessJSON(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "run", "--json",
		"--run-id", "s1", "--workdir", dir, "--check", "true")
	if exit != 10 {
		t.Fatalf("run success exit=%d, want 10", exit)
	}
	obj := parseSingleJSONObject(t, stdout)
	if obj.HaltReason != "success_condition_met" {
		t.Fatalf("halt_reason=%q, want success_condition_met", obj.HaltReason)
	}
	if obj.Status != "halted" {
		t.Fatalf("status=%q, want halted", obj.Status)
	}
	if obj.Errors == nil {
		t.Fatal("errors must be an array, got null")
	}
}

func TestContractRunMaxIterationsJSON(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "run", "--json",
		"--run-id", "m1", "--workdir", dir, "--check", "false", "--max-iterations", "2")
	if exit != 12 {
		t.Fatalf("run max-iterations exit=%d, want 12", exit)
	}
	obj := parseSingleJSONObject(t, stdout)
	if obj.HaltReason != "max_iterations_reached" {
		t.Fatalf("halt_reason=%q, want max_iterations_reached", obj.HaltReason)
	}
	if obj.Iteration != 2 {
		t.Fatalf("iteration=%d, want 2", obj.Iteration)
	}
}

func TestContractRunExecFailureJSON(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "run", "--json",
		"--run-id", "e1", "--workdir", dir, "--check", "true", "--exec", "exit 3")
	if exit != 40 {
		t.Fatalf("run exec-failure exit=%d, want 40", exit)
	}
	obj := parseSingleJSONObject(t, stdout)
	if obj.HaltReason != "execution_failure" {
		t.Fatalf("halt_reason=%q, want execution_failure", obj.HaltReason)
	}
	if obj.Status != "error" {
		t.Fatalf("status=%q, want error", obj.Status)
	}
}

// The receipt MUST be valid JSONL even when failure text contains quotes,
// backslashes, or newlines (SPEC.md §8). A check whose output is hostile to
// naive string interpolation must still produce a parseable receipt.
func TestRunReceiptIsValidJSONL(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	hostile := `printf 'FAIL "x"\\n\tbad\n'; exit 1`
	exit, _, _ := runCLI(t, bin, "run", "--json",
		"--run-id", "r1", "--workdir", dir, "--check", hostile, "--max-iterations", "1")
	if exit != 12 {
		t.Fatalf("exit=%d, want 12", exit)
	}

	receipt := filepath.Join(dir, ".loopexec", "run-r1.jsonl")
	data, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatalf("reading receipt: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least run_start/iter_start/check/halt events, got %d lines", len(lines))
	}
	for i, ln := range lines {
		var ev map[string]any
		if err := json.Unmarshal([]byte(ln), &ev); err != nil {
			t.Fatalf("receipt line %d is not valid JSON: %v\nline=%q", i, err, ln)
		}
	}

	state := filepath.Join(dir, ".loopexec", "state.json")
	sdata, err := os.ReadFile(state)
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	var st map[string]any
	if err := json.Unmarshal(sdata, &st); err != nil {
		t.Fatalf("state.json is not valid JSON: %v", err)
	}
	if st["halt_reason"] != "max_iterations_reached" {
		t.Fatalf("state halt_reason=%v, want max_iterations_reached", st["halt_reason"])
	}
}
