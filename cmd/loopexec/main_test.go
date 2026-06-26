package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

func TestExitCodeMappings(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, exitSuccess},
		{"invariant", &cliError{Code: exitInvariantFailed, Message: "x"}, exitInvariantFailed},
		{"internal", errors.New("boom"), exitInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exitCode(tt.err)
			if got != tt.want {
				t.Fatalf("exitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHaltExitCodeMap(t *testing.T) {
	// Locks the canonical halt_reason -> exit_code map from SPEC.md §5.
	cases := map[string]int{
		"success_condition_met":      exitConverged,
		"human_required":             exitTerminalBlocked,
		"max_iterations_reached":     exitIterationCap,
		"metric_integrity_violation": exitIntegrity,
		"reward_hacking_detected":    exitIntegrity,
		"check_flaky":                exitOracleUntrusted,
		"check_inadequate":           exitCheckInadequate,
		"escalation_pending":         exitResumableJudgment,
		"oscillation_detected":       exitNoConvergence,
		"budget_exceeded":            exitBudget,
		"model_drift_detected":       exitLivenessDrift,
		"workspace_invalid":          exitWorkspaceInvalid,
		"execution_failure":          exitExecutionFailure,
		"definitely_not_a_reason":    exitInternalError,
	}
	for reason, want := range cases {
		if got := haltExitCode(reason); got != want {
			t.Errorf("haltExitCode(%q) = %d, want %d", reason, got, want)
		}
	}
}

func TestPrintResponseJSON(t *testing.T) {
	jsonOutput = true
	defer func() { jsonOutput = false }()

	buf := bytes.NewBuffer(nil)
	cmd := &cobra.Command{}
	cmd.SetOut(buf)

	r := response{
		Tool:      toolName,
		Version:   toolVersion,
		Status:    "ok",
		RunID:     "r1",
		Iteration: 1,
		Errors:    []string{},
	}

	if err := printResponse(cmd, r); err != nil {
		t.Fatalf("printResponse() error = %v", err)
	}

	var got response
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if got.Tool != toolName || got.Status != "ok" || got.RunID != "r1" {
		t.Fatalf("unexpected response payload: %+v", got)
	}
}
