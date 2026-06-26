package main

import (
	"encoding/json"
	"math"
	"testing"
)

func TestResolveProbeRuns(t *testing.T) {
	cases := []struct {
		runsFlag   int
		maxFlake   float64
		wantRuns   int
		wantCapped bool
	}{
		{0, 0, 10, false},       // default floor
		{7, 0, 7, false},        // explicit runs wins
		{0, 0.05, 60, false},    // ceil(3/0.05)
		{0, 0.01, 300, false},   // ceil(3/0.01)
		{0, 0.0001, 500, true},  // would be 30000 -> capped
		{12, 0.0001, 12, false}, // explicit runs overrides derivation
	}
	for _, c := range cases {
		gotRuns, gotCapped := resolveProbeRuns(c.runsFlag, c.maxFlake)
		if gotRuns != c.wantRuns || gotCapped != c.wantCapped {
			t.Errorf("resolveProbeRuns(%d,%v) = (%d,%t), want (%d,%t)",
				c.runsFlag, c.maxFlake, gotRuns, gotCapped, c.wantRuns, c.wantCapped)
		}
	}
}

func TestRuleOfThreeUpper(t *testing.T) {
	if got := ruleOfThreeUpper(0); got != 1.0 {
		t.Fatalf("ruleOfThreeUpper(0) = %v, want 1.0", got)
	}
	if got := ruleOfThreeUpper(60); math.Abs(got-0.05) > 1e-9 {
		t.Fatalf("ruleOfThreeUpper(60) = %v, want 0.05", got)
	}
}

type probeJSON struct {
	Status     string   `json:"status"`
	HaltReason string   `json:"halt_reason"`
	Errors     []string `json:"errors"`
	Probe      *struct {
		Runs            int     `json:"runs"`
		Stable          bool    `json:"stable"`
		FlakeCount      int     `json:"flake_count"`
		FlakeUpperBound float64 `json:"flake_upper_bound"`
		Certified       bool    `json:"certified"`
	} `json:"probe"`
	Doctor *struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	} `json:"doctor"`
}

func parseProbe(t *testing.T, s string) probeJSON {
	t.Helper()
	var p probeJSON
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, s)
	}
	return p
}

func TestProbeCheckRequiresCheck(t *testing.T) {
	bin := buildTestBinary(t)
	exit, stdout, _ := runCLI(t, bin, "probe-check", "--json")
	if exit != 30 {
		t.Fatalf("probe-check without --check exit=%d, want 30", exit)
	}
	if p := parseProbe(t, stdout); p.HaltReason != "workspace_invalid" {
		t.Fatalf("halt_reason=%q, want workspace_invalid", p.HaltReason)
	}
}

func TestProbeCheckStable(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "probe-check", "--json",
		"--workdir", dir, "--check", "true", "--runs", "5")
	if exit != 0 {
		t.Fatalf("probe-check stable exit=%d, want 0", exit)
	}
	p := parseProbe(t, stdout)
	if p.Probe == nil || !p.Probe.Stable || p.Probe.Runs != 5 || p.Probe.FlakeCount != 0 {
		t.Fatalf("unexpected probe report: %+v", p.Probe)
	}
}

func TestProbeCheckFlaky(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	// Alternates pass/fail across runs by toggling a counter file in workdir.
	flaky := `n=$(cat .pc 2>/dev/null || echo 0); n=$((n+1)); printf %s "$n" > .pc; [ $((n%2)) -eq 0 ]`
	exit, stdout, _ := runCLI(t, bin, "probe-check", "--json",
		"--workdir", dir, "--check", flaky, "--runs", "4")
	if exit != 14 {
		t.Fatalf("probe-check flaky exit=%d, want 14", exit)
	}
	p := parseProbe(t, stdout)
	if p.HaltReason != "check_flaky" {
		t.Fatalf("halt_reason=%q, want check_flaky", p.HaltReason)
	}
	if p.Probe == nil || p.Probe.Stable {
		t.Fatalf("expected unstable probe, got %+v", p.Probe)
	}
}

func TestDoctorGreen(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "doctor", "--json",
		"--workdir", dir, "--check", "true", "--runs", "3")
	if exit != 0 {
		t.Fatalf("doctor green exit=%d, want 0", exit)
	}
	p := parseProbe(t, stdout)
	if p.Status != "ok" || p.Doctor == nil {
		t.Fatalf("unexpected doctor response: status=%q doctor=%v", p.Status, p.Doctor)
	}
	// determinism must be a pass; hermeticity/adequacy/isolation reported as planned.
	var detPass, plannedCount int
	for _, c := range p.Doctor.Checks {
		if c.Name == "determinism" && c.Status == "pass" {
			detPass++
		}
		if c.Status == "planned" {
			plannedCount++
		}
	}
	if detPass != 1 || plannedCount < 3 {
		t.Fatalf("doctor checks not as expected: %+v", p.Doctor.Checks)
	}
}

func TestDoctorRequiresCheck(t *testing.T) {
	bin := buildTestBinary(t)
	exit, stdout, _ := runCLI(t, bin, "doctor", "--json")
	if exit != 30 {
		t.Fatalf("doctor without --check exit=%d, want 30", exit)
	}
	if p := parseProbe(t, stdout); p.HaltReason != "workspace_invalid" {
		t.Fatalf("halt_reason=%q, want workspace_invalid", p.HaltReason)
	}
}

func TestDoctorFlakyFails(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	flaky := `n=$(cat .pc 2>/dev/null || echo 0); n=$((n+1)); printf %s "$n" > .pc; [ $((n%2)) -eq 0 ]`
	exit, stdout, _ := runCLI(t, bin, "doctor", "--json",
		"--workdir", dir, "--check", flaky, "--runs", "4")
	if exit != 14 {
		t.Fatalf("doctor flaky exit=%d, want 14", exit)
	}
	if p := parseProbe(t, stdout); p.HaltReason != "check_flaky" {
		t.Fatalf("halt_reason=%q, want check_flaky", p.HaltReason)
	}
}
