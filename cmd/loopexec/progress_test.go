package main

import (
	"sort"
	"testing"
)

func mkset(ids ...string) (map[string]struct{}, []string) {
	m := map[string]struct{}{}
	for _, id := range ids {
		m[id] = struct{}{}
	}
	ord := append([]string{}, ids...)
	sort.Strings(ord)
	return m, ord
}

func obs(t *progressTracker, iter int, ids ...string) string {
	F, ord := mkset(ids...)
	return t.observe(iter, F, ord)
}

func TestProgressImprovingNoHalt(t *testing.T) {
	tr := newProgressTracker(3)
	if r := obs(tr, 1, "a", "b", "c"); r != "" {
		t.Fatalf("iter1 = %q, want continue", r)
	}
	if r := obs(tr, 2, "a", "b"); r != "" { // c resolved -> improving
		t.Fatalf("iter2 = %q, want continue", r)
	}
	if r := obs(tr, 3, "a"); r != "" { // b resolved -> improving
		t.Fatalf("iter3 = %q, want continue", r)
	}
	if !tr.everImproved || tr.bestSize != 1 || tr.bestIter != 3 {
		t.Fatalf("tracker = everImproved:%t best:%d@%d", tr.everImproved, tr.bestSize, tr.bestIter)
	}
}

func TestProgressNoProgressDetected(t *testing.T) {
	tr := newProgressTracker(3)
	// Distinct sets of the same size: no strict decrease, no exact repeat.
	_ = obs(tr, 1, "a", "b")
	_ = obs(tr, 2, "a", "c")
	_ = obs(tr, 3, "a", "d")
	if r := obs(tr, 4, "a", "e"); r != "no_progress_detected" { // 4 - bestIter(1) >= 3
		t.Fatalf("iter4 = %q, want no_progress_detected", r)
	}
}

func TestProgressOscillationDetected(t *testing.T) {
	tr := newProgressTracker(5)
	_ = obs(tr, 1, "a", "b")
	_ = obs(tr, 2, "a")                                         // improve
	if r := obs(tr, 3, "a", "b"); r != "oscillation_detected" { // {a,b} repeats iter1
		t.Fatalf("iter3 = %q, want oscillation_detected", r)
	}
}

func TestProgressRegressionDetected(t *testing.T) {
	tr := newProgressTracker(5)
	_ = obs(tr, 1, "a", "b")
	_ = obs(tr, 2, "a")            // b resolved
	r := obs(tr, 3, "a", "b", "c") // b returns (regression); new set so not oscillation
	if r != "same_test_regressed" {
		t.Fatalf("iter3 = %q, want same_test_regressed", r)
	}
}

func TestExplainHaltVerdicts(t *testing.T) {
	best := 2
	cases := []struct {
		st         loopState
		wantPrefix string
	}{
		{loopState{HaltReason: "success_condition_met"}, "done"},
		{loopState{HaltReason: "max_iterations_reached", EverImproved: true, BestFailCount: &best}, "raise-the-limit"},
		{loopState{HaltReason: "max_iterations_reached", EverImproved: false, BestFailCount: &best}, "do-not-retry"},
		{loopState{HaltReason: "oscillation_detected"}, "do-not-retry"},
		{loopState{HaltReason: "same_test_regressed"}, "do-not-retry"},
		{loopState{HaltReason: "no_progress_detected", EverImproved: false}, "do-not-retry"},
	}
	for _, c := range cases {
		got, _ := explainHalt(c.st)
		if got != c.wantPrefix {
			t.Errorf("explainHalt(%q, improved=%t) verdict=%q, want %q",
				c.st.HaltReason, c.st.EverImproved, got, c.wantPrefix)
		}
	}
}

// End-to-end: a loop whose failing set never shrinks halts in the no-convergence
// class, and explain-halt reads the recorded state.
func TestContractRunOscillationJSON(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	// failures-cmd reports a constant set every iteration -> exact repeat at iter2.
	exit, stdout, _ := runCLI(t, bin, "run", "--json",
		"--run-id", "osc", "--workdir", dir,
		"--check", "false", "--failures-cmd", "printf 'fail-a\\nfail-b\\n'",
		"--max-iterations", "10")
	if exit != 17 {
		t.Fatalf("oscillation exit=%d, want 17 (no-convergence)", exit)
	}
	obj := parseSingleJSONObject(t, stdout)
	if obj.HaltReason != "oscillation_detected" {
		t.Fatalf("halt_reason=%q, want oscillation_detected", obj.HaltReason)
	}

	ex, exStdout, _ := runCLI(t, bin, "explain-halt", "--json", "--workdir", dir)
	if ex != 0 {
		t.Fatalf("explain-halt exit=%d, want 0", ex)
	}
	exObj := parseSingleJSONObject(t, exStdout)
	if exObj.HaltReason != "oscillation_detected" {
		t.Fatalf("explain-halt halt_reason=%q, want oscillation_detected", exObj.HaltReason)
	}
}
