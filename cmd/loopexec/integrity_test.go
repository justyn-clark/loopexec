package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingMembers(t *testing.T) {
	base, _ := mkset("t1", "t2", "t3")
	cur, _ := mkset("t1", "t3") // t2 lost
	got := missingMembers(base, cur)
	if len(got) != 1 || got[0] != "t2" {
		t.Fatalf("missingMembers = %v, want [t2]", got)
	}
	full, _ := mkset("t1", "t2", "t3", "t4") // superset: nothing missing
	if got := missingMembers(base, full); len(got) != 0 {
		t.Fatalf("missingMembers(superset) = %v, want empty", got)
	}
}

// The metric-integrity gate halts before a suite can go green by testing less:
// when the work command removes a member of the t0 surface, the run halts
// metric_integrity_violation (exit 13) instead of ever reaching the check.
func TestContractRunMetricIntegrityViolation(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".tests"), []byte("t1\nt2\nt3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exit, stdout, _ := runCLI(t, bin, "run", "--json",
		"--run-id", "mi", "--workdir", dir,
		"--check", "false",
		"--exec", "grep -v t2 .tests > .t && mv .t .tests",
		"--integrity-cmd", "cat .tests",
		"--max-iterations", "5")
	if exit != 13 {
		t.Fatalf("metric-integrity exit=%d, want 13", exit)
	}
	if obj := parseSingleJSONObject(t, stdout); obj.HaltReason != "metric_integrity_violation" {
		t.Fatalf("halt_reason=%q, want metric_integrity_violation", obj.HaltReason)
	}
}

// A stable surface does not trip the gate; the loop converges normally.
func TestContractRunMetricIntegrityStable(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "run", "--json",
		"--run-id", "mok", "--workdir", dir,
		"--check", "true", "--integrity-cmd", "printf 't1\\nt2\\n'",
		"--max-iterations", "3")
	if exit != 10 {
		t.Fatalf("stable-integrity exit=%d, want 10 (converged)", exit)
	}
	if obj := parseSingleJSONObject(t, stdout); obj.HaltReason != "success_condition_met" {
		t.Fatalf("halt_reason=%q, want success_condition_met", obj.HaltReason)
	}
}

func TestDoctorBindClaudeHome(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "doctor", "--json",
		"--workdir", dir, "--check", "true", "--runs", "2", "--bind-claude-home")
	if exit != 13 {
		t.Fatalf("bind-claude-home exit=%d, want 13", exit)
	}
	if p := parseProbe(t, stdout); p.HaltReason != "credential_scope_invalid" {
		t.Fatalf("halt_reason=%q, want credential_scope_invalid", p.HaltReason)
	}
}

func TestDoctorExecNetworkUnsafe(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "doctor", "--json",
		"--workdir", dir, "--check", "true", "--runs", "2", "--exec-network", "bridge")
	if exit != 30 {
		t.Fatalf("exec-network bridge exit=%d, want 30", exit)
	}
	if p := parseProbe(t, stdout); p.HaltReason != "isolation_unsatisfiable" {
		t.Fatalf("halt_reason=%q, want isolation_unsatisfiable", p.HaltReason)
	}
}

func TestDoctorIsolationGreen(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "doctor", "--json",
		"--workdir", dir, "--check", "true", "--runs", "2", "--exec-network", "none")
	if exit != 0 {
		t.Fatalf("isolation-green exit=%d, want 0", exit)
	}
	p := parseProbe(t, stdout)
	if p.Status != "ok" || p.Doctor == nil {
		t.Fatalf("unexpected doctor response: status=%q", p.Status)
	}
	var credPass, netPass bool
	for _, c := range p.Doctor.Checks {
		if c.Name == "isolation-credentials" && c.Status == "pass" {
			credPass = true
		}
		if c.Name == "isolation-exec-network" && c.Status == "pass" {
			netPass = true
		}
	}
	if !credPass || !netPass {
		t.Fatalf("isolation checks not both pass: %+v", p.Doctor.Checks)
	}
}
