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
