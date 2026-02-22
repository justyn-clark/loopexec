package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

type contractJSON struct {
	Tool       string   `json:"tool"`
	Version    string   `json:"version"`
	Status     string   `json:"status"`
	RunID      string   `json:"run_id"`
	Iteration  int      `json:"iteration"`
	HaltReason string   `json:"halt_reason"`
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

func TestContractRunBlockedJSON(t *testing.T) {
	bin := buildTestBinary(t)
	exit, stdout, _ := runCLI(t, bin, "run", "--json", "--halt-reason", "blocked")
	if exit != 11 {
		t.Fatalf("run blocked exit=%d, want 11", exit)
	}
	obj := parseSingleJSONObject(t, stdout)
	if obj.HaltReason != "blocked" {
		t.Fatalf("halt_reason=%q, want blocked", obj.HaltReason)
	}
	if obj.Errors == nil {
		t.Fatal("errors must be an array, got null")
	}
}

func TestContractRunHappyPathJSON(t *testing.T) {
	bin := buildTestBinary(t)
	exit, stdout, _ := runCLI(t, bin, "run", "--json")
	if exit != 0 && exit != 10 {
		t.Fatalf("run happy exit=%d, want 0 or 10", exit)
	}
	obj := parseSingleJSONObject(t, stdout)
	if obj.Tool != "loopexec" {
		t.Fatalf("tool=%q, want loopexec", obj.Tool)
	}
	if obj.Errors == nil {
		t.Fatal("errors must be an array, got null")
	}
}
