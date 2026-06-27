package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type doctorJSON struct {
	HaltReason string `json:"halt_reason"`
	Doctor     *struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	} `json:"doctor"`
}

func parseDoctorJSON(t *testing.T, out string) doctorJSON {
	t.Helper()
	var d doctorJSON
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, out)
	}
	if d.Doctor == nil {
		t.Fatalf("response has no doctor report: %s", out)
	}
	return d
}

func adequacyStatus(t *testing.T, d doctorJSON) string {
	t.Helper()
	for _, c := range d.Doctor.Checks {
		if c.Name == "adequacy" {
			return c.Status
		}
	}
	t.Fatal("no adequacy check in the doctor report")
	return ""
}

// workdirWithF makes a workdir holding f="ok" so `grep -q ok f` is a green,
// deterministic check.
func workdirWithF(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A mutation that truly breaks the check turns it red: the check exercises the
// change, so it is adequate (doctor green).
func TestDoctorAdequacyPass(t *testing.T) {
	bin := buildTestBinary(t)
	dir := workdirWithF(t)
	code, out, _ := runCLI(t, bin, "doctor", "--json", "--workdir", dir,
		"--check", "grep -q ok f", "--mutate-cmd", "echo xyz > f")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (adequate); out=%s", code, out)
	}
	if s := adequacyStatus(t, parseDoctorJSON(t, out)); s != "pass" {
		t.Fatalf("adequacy = %q, want pass", s)
	}
}

// A mutation the check fails to catch (it touches unrelated code) means the
// check does not exercise the change: check_inadequate (exit 15).
func TestDoctorAdequacyInadequate(t *testing.T) {
	bin := buildTestBinary(t)
	dir := workdirWithF(t)
	code, out, _ := runCLI(t, bin, "doctor", "--json", "--workdir", dir,
		"--check", "grep -q ok f", "--mutate-cmd", "echo noise > unrelated.txt")
	if code != 15 {
		t.Fatalf("exit = %d, want 15 (check_inadequate)", code)
	}
	d := parseDoctorJSON(t, out)
	if d.HaltReason != "check_inadequate" {
		t.Fatalf("halt = %q, want check_inadequate", d.HaltReason)
	}
	if s := adequacyStatus(t, d); s != "fail" {
		t.Fatalf("adequacy = %q, want fail", s)
	}
}

// Without --mutate-cmd the adequacy gate stays planned and does not fail doctor.
func TestDoctorAdequacyPlannedWithoutFlag(t *testing.T) {
	bin := buildTestBinary(t)
	dir := workdirWithF(t)
	code, out, _ := runCLI(t, bin, "doctor", "--json", "--workdir", dir, "--check", "grep -q ok f")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if s := adequacyStatus(t, parseDoctorJSON(t, out)); s != "planned" {
		t.Fatalf("adequacy = %q, want planned", s)
	}
}

// A mutate-cmd that cannot apply is a setup error (execution_failure, exit 40),
// not an adequacy verdict.
func TestDoctorAdequacyMutateFailure(t *testing.T) {
	bin := buildTestBinary(t)
	dir := workdirWithF(t)
	code, _, _ := runCLI(t, bin, "doctor", "--json", "--workdir", dir,
		"--check", "grep -q ok f", "--mutate-cmd", "exit 1")
	if code != 40 {
		t.Fatalf("exit = %d, want 40 (execution_failure)", code)
	}
}

// The canary runs in a copy: the real workdir is never mutated.
func TestDoctorAdequacyLeavesWorkdirUntouched(t *testing.T) {
	bin := buildTestBinary(t)
	dir := workdirWithF(t)
	runCLI(t, bin, "doctor", "--json", "--workdir", dir,
		"--check", "grep -q ok f", "--mutate-cmd", "echo xyz > f")
	got, err := os.ReadFile(filepath.Join(dir, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok\n" {
		t.Fatalf("workdir f = %q, want unchanged \"ok\\n\"", got)
	}
}
