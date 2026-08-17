package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type demoJSON struct {
	Tool       string   `json:"tool"`
	Status     string   `json:"status"`
	RunID      string   `json:"run_id"`
	HaltReason string   `json:"halt_reason"`
	Receipt    string   `json:"receipt"`
	Verified   *bool    `json:"verified"`
	Errors     []string `json:"errors"`
}

func TestContractDemoJSON(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	exit, stdout, stderr := runCLI(t, bin, "demo", "--json", "--workdir", dir)
	if exit != 0 {
		t.Fatalf("demo --json exit=%d, want 0\nstderr=%s", exit, stderr)
	}
	var got demoJSON
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("demo stdout is not one JSON object: %v\nstdout=%q", err, stdout)
	}
	if got.Tool != toolName || got.Status != "verified" || got.RunID != demoRunID {
		t.Fatalf("unexpected demo response: %+v", got)
	}
	if got.HaltReason != "success_condition_met" || got.Verified == nil || !*got.Verified {
		t.Fatalf("demo did not prove convergence and replay: %+v", got)
	}
	if got.Errors == nil || got.Receipt == "" {
		t.Fatalf("demo response has invalid errors/receipt fields: %+v", got)
	}

	receipt, err := os.ReadFile(got.Receipt)
	if err != nil {
		t.Fatalf("read demo receipt: %v", err)
	}
	seen := map[string]bool{}
	for i, line := range bytes.Split(bytes.TrimSpace(receipt), []byte("\n")) {
		var event receiptEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("receipt line %d is not typed JSON: %v", i+1, err)
		}
		seen[event.Event] = true
	}
	for _, event := range []string{"run_start", "exec", "check", "halt"} {
		if !seen[event] {
			t.Fatalf("demo receipt missing %q event", event)
		}
	}

	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep\n" {
		t.Fatalf("unrelated workdir file changed: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, demoStatusFile)); err != nil || string(data) != "green\n" {
		t.Fatalf("demo fixture not repaired: data=%q err=%v", data, err)
	}

	replayExit, replayOut, replayErr := runCLI(t, bin, "replay", "--json", "--workdir", dir, "--run-id", demoRunID)
	if replayExit != 0 {
		t.Fatalf("replay demo exit=%d, want 0\nstderr=%s", replayExit, replayErr)
	}
	if replay := parseReceipt(t, replayOut); replay.Verified == nil || !*replay.Verified {
		t.Fatalf("replay did not verify demo receipt: %+v", replay)
	}

	before := append([]byte(nil), receipt...)
	secondExit, _, _ := runCLI(t, bin, "demo", "--json", "--workdir", dir)
	if secondExit != exitWorkspaceInvalid {
		t.Fatalf("second demo exit=%d, want %d", secondExit, exitWorkspaceInvalid)
	}
	after, err := os.ReadFile(got.Receipt)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("second demo overwrote the first receipt: err=%v", err)
	}
}

func TestPrepareDemoWorkspacePreservesReservedPaths(t *testing.T) {
	t.Run("runtime directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".loopexec"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareDemoWorkspace(dir); err == nil {
			t.Fatal("prepareDemoWorkspace accepted an existing .loopexec directory")
		}
		if _, err := os.Stat(filepath.Join(dir, demoStatusFile)); !os.IsNotExist(err) {
			t.Fatalf("fixture was created despite collision: %v", err)
		}
	})

	t.Run("fixture", func(t *testing.T) {
		dir := t.TempDir()
		fixture := filepath.Join(dir, demoStatusFile)
		if err := os.WriteFile(fixture, []byte("mine\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareDemoWorkspace(dir); err == nil {
			t.Fatal("prepareDemoWorkspace accepted an existing fixture")
		}
		data, err := os.ReadFile(fixture)
		if err != nil || string(data) != "mine\n" {
			t.Fatalf("existing fixture changed: data=%q err=%v", data, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".loopexec")); !os.IsNotExist(err) {
			t.Fatalf("failed preparation left runtime directory: %v", err)
		}
	})
}
