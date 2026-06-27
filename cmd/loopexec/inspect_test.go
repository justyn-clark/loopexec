package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type costJSON struct {
	HaltReason string `json:"halt_reason"`
	Cost       *struct {
		Iterations int     `json:"iterations"`
		TotalUSD   float64 `json:"total_usd"`
		OverBudget bool    `json:"over_budget"`
		AnomalyAt  []int   `json:"anomaly_at"`
	} `json:"cost"`
}

func parseCostJSON(t *testing.T, s string) costJSON {
	t.Helper()
	var r costJSON
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, s)
	}
	return r
}

// Under cap with no spike: exit 0, no halt.
func TestInspectCostOK(t *testing.T) {
	bin := buildTestBinary(t)
	code, out, _ := runCLI(t, bin, "inspect-cost", "--json", "--budget-usd", "5",
		"--cost", "0.10", "--cost", "0.12", "--cost", "0.11", "--cost", "0.13", "--cost", "0.12")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; out=%s", code, out)
	}
	r := parseCostJSON(t, out)
	if r.HaltReason != "" || r.Cost == nil || r.Cost.OverBudget || len(r.Cost.AnomalyAt) != 0 {
		t.Fatalf("want clean ok, got halt=%q cost=%+v", r.HaltReason, r.Cost)
	}
}

// Run-total over the cap halts budget_exceeded (exit 18).
func TestInspectCostBudgetExceeded(t *testing.T) {
	bin := buildTestBinary(t)
	code, out, _ := runCLI(t, bin, "inspect-cost", "--json", "--budget-usd", "5",
		"--cost", "2", "--cost", "2", "--cost", "2")
	if code != 18 {
		t.Fatalf("exit = %d, want 18 (budget)", code)
	}
	if r := parseCostJSON(t, out); r.HaltReason != "budget_exceeded" || r.Cost == nil || !r.Cost.OverBudget {
		t.Fatalf("want budget_exceeded/over, got %q %+v", r.HaltReason, r.Cost)
	}
}

// A spike above the rolling sigma bound halts cost_anomaly (exit 18), even under
// the cap.
func TestInspectCostAnomaly(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	led := filepath.Join(dir, "ledger.txt")
	if err := os.WriteFile(led, []byte("0.10\n0.11\n0.10\n0.12\n0.11\n5.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runCLI(t, bin, "inspect-cost", "--json", "--budget-usd", "100", "--ledger", led)
	if code != 18 {
		t.Fatalf("exit = %d, want 18 (anomaly)", code)
	}
	r := parseCostJSON(t, out)
	if r.HaltReason != "cost_anomaly" {
		t.Fatalf("halt = %q, want cost_anomaly", r.HaltReason)
	}
	found := false
	for _, a := range r.Cost.AnomalyAt {
		if a == 6 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the iteration-6 spike flagged, got %v", r.Cost.AnomalyAt)
	}
}

// No costs is a usage error (invariant_failed, exit 20), not a budget halt.
func TestInspectCostNoCostsErrors(t *testing.T) {
	bin := buildTestBinary(t)
	code, _, _ := runCLI(t, bin, "inspect-cost", "--json")
	if code != 20 {
		t.Fatalf("exit = %d, want 20 (invariant)", code)
	}
}

// A negative cost is rejected (a ledger of spend cannot go negative).
func TestInspectCostNegativeRejected(t *testing.T) {
	bin := buildTestBinary(t)
	code, _, _ := runCLI(t, bin, "inspect-cost", "--json", "--cost", "0.1", "--cost", "-3")
	if code != 20 {
		t.Fatalf("exit = %d, want 20 (invariant)", code)
	}
}

// A perfectly flat ledger has no variance, so the sigma detector raises nothing.
func TestInspectCostFlatNoAnomaly(t *testing.T) {
	bin := buildTestBinary(t)
	code, out, _ := runCLI(t, bin, "inspect-cost", "--json",
		"--cost", "1", "--cost", "1", "--cost", "1", "--cost", "1", "--cost", "1")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if r := parseCostJSON(t, out); r.Cost == nil || len(r.Cost.AnomalyAt) != 0 {
		t.Fatalf("flat ledger should raise no anomaly, got %+v", r.Cost)
	}
}
