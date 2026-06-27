package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type isolateJSON struct {
	Status    string `json:"status"`
	Isolation *struct {
		Clone         string `json:"clone"`
		ExecZoneCmd   string `json:"exec_zone_cmd"`
		AgentZoneCmd  string `json:"agent_zone_cmd"`
		Minted        bool   `json:"minted"`
		Revoked       bool   `json:"revoked"`
		Executed      bool   `json:"executed"`
		ExecZoneExit  *int   `json:"exec_zone_exit"`
		AgentZoneExit *int   `json:"agent_zone_exit"`
	} `json:"isolation"`
}

func parseIsolate(t *testing.T, s string) isolateJSON {
	t.Helper()
	var i isolateJSON
	if err := json.Unmarshal([]byte(s), &i); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, s)
	}
	return i
}

func makeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "init"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestDetachedCloneHardened(t *testing.T) {
	repo := makeRepo(t)
	into := filepath.Join(t.TempDir(), "work")
	if err := detachedClone(repo, "", into); err != nil {
		t.Fatalf("detachedClone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(into, "a.txt")); err != nil {
		t.Fatal("clone is missing repo files")
	}
	get := func(args ...string) string {
		c := exec.Command("git", args...)
		c.Dir = into
		out, _ := c.Output()
		return strings.TrimSpace(string(out))
	}
	if r := get("remote"); r != "" {
		t.Fatalf("clone still has a remote: %q", r)
	}
	if h := get("config", "core.hooksPath"); h != "/dev/null" {
		t.Fatalf("core.hooksPath=%q, want /dev/null", h)
	}
}

func sepBeforeImage(argv []string, image string) bool {
	for i, a := range argv {
		if a == image && i > 0 && argv[i-1] == "--" {
			return true
		}
	}
	return false
}

func TestZoneArgv(t *testing.T) {
	ex := execZoneArgv("/w", "img", "go test")
	if !strings.Contains(strings.Join(ex, " "), "--network none") {
		t.Fatalf("exec zone must be --network none: %v", ex)
	}
	if !sepBeforeImage(ex, "img") {
		t.Fatalf("exec zone needs -- before image (flag-injection guard): %v", ex)
	}
	ag := agentZoneArgv("/w", "img", "agent-net", "/w/agent.env", "make")
	ags := strings.Join(ag, " ")
	if !strings.Contains(ags, "--env-file /w/agent.env") || strings.Contains(ags, "--network none") {
		t.Fatalf("agent zone wrong: %s", ags)
	}
	if !sepBeforeImage(ag, "img") {
		t.Fatalf("agent zone needs -- before image: %v", ag)
	}
}

func TestIsolateRejectsHostileImage(t *testing.T) {
	bin := buildTestBinary(t)
	repo := makeRepo(t)
	exit, stdout, _ := runCLI(t, bin, "isolate", "--json", "--workdir", repo, "--repo", repo,
		"--run-id", "h", "--exec-image=--privileged")
	if exit != 30 {
		t.Fatalf("hostile image exit=%d, want 30", exit)
	}
	if !strings.Contains(stdout, "isolation_unsatisfiable") {
		t.Fatalf("want isolation_unsatisfiable, got %s", stdout)
	}
}

func TestIsolateRejectsBadRunID(t *testing.T) {
	bin := buildTestBinary(t)
	repo := makeRepo(t)
	exit, _, _ := runCLI(t, bin, "isolate", "--json", "--workdir", repo, "--repo", repo, "--run-id", "../evil")
	if exit != 30 {
		t.Fatalf("bad run-id exit=%d, want 30", exit)
	}
	// No sandbox dir should have been created outside .loopexec/.
	if _, err := os.Stat(filepath.Join(repo, "evil")); err == nil {
		t.Fatal("run-id traversal created a path outside .loopexec")
	}
}

func TestIsolateMintRequiresRevoke(t *testing.T) {
	bin := buildTestBinary(t)
	repo := makeRepo(t)
	exit, stdout, _ := runCLI(t, bin, "isolate", "--json", "--workdir", repo, "--repo", repo,
		"--run-id", "mr", "--mint-cmd", "echo K", "--execute", "--confirm")
	if exit != 30 {
		t.Fatalf("mint without revoke exit=%d, want 30", exit)
	}
	if !strings.Contains(stdout, "isolation_unsatisfiable") {
		t.Fatalf("want isolation_unsatisfiable, got %s", stdout)
	}
}

func TestIsolateZoneFailureSurfaced(t *testing.T) {
	bin := buildTestBinary(t)
	repo := makeRepo(t)
	runtime := filepath.Join(t.TempDir(), "fail-runtime")
	os.WriteFile(runtime, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	exit, stdout, _ := runCLI(t, bin, "isolate", "--json", "--workdir", repo, "--repo", repo,
		"--run-id", "zf", "--check", "true", "--exec", "true", "--runtime", runtime, "--execute", "--confirm")
	if exit != 40 {
		t.Fatalf("failed zone exit=%d, want 40 (execution_failure)", exit)
	}
	if !strings.Contains(stdout, "execution_failure") {
		t.Fatalf("want execution_failure, got %s", stdout)
	}
}

func TestIsolatePlan(t *testing.T) {
	bin := buildTestBinary(t)
	repo := makeRepo(t)
	exit, stdout, _ := runCLI(t, bin, "isolate", "--json", "--workdir", repo, "--repo", repo,
		"--run-id", "p", "--check", "go test ./...", "--exec", "make fix")
	if exit != 0 {
		t.Fatalf("isolate plan exit=%d, want 0", exit)
	}
	iso := parseIsolate(t, stdout)
	if iso.Isolation == nil || iso.Isolation.Executed || iso.Isolation.Minted {
		t.Fatalf("plan should not mint/execute: %+v", iso.Isolation)
	}
	if !strings.Contains(iso.Isolation.ExecZoneCmd, "--network none") {
		t.Fatalf("exec zone not network:none: %s", iso.Isolation.ExecZoneCmd)
	}
	if !strings.Contains(iso.Isolation.AgentZoneCmd, "--env-file") {
		t.Fatalf("agent zone missing --env-file: %s", iso.Isolation.AgentZoneCmd)
	}
}

func TestIsolateExecuteRequiresConfirm(t *testing.T) {
	bin := buildTestBinary(t)
	repo := makeRepo(t)
	exit, _, _ := runCLI(t, bin, "isolate", "--json", "--workdir", repo, "--repo", repo, "--run-id", "nc", "--execute")
	if exit != 20 {
		t.Fatalf("--execute without --confirm exit=%d, want 20", exit)
	}
}

// Critical security regression: on --execute, the minted key must never appear
// in the JSON output, the receipt, or the runtime argv (process list), the
// env-file must be removed, and revoke must run.
func TestIsolateExecuteRedactsCredential(t *testing.T) {
	bin := buildTestBinary(t)
	repo := makeRepo(t)
	rtLog := filepath.Join(t.TempDir(), "runtime.log")
	runtime := filepath.Join(t.TempDir(), "stub-runtime")
	os.WriteFile(runtime, []byte("#!/bin/sh\necho \"$@\" >> "+rtLog+"\n"), 0o755)
	marker := filepath.Join(t.TempDir(), "revoked.marker")
	const secret = "SUPER-SECRET-KEY-XYZ"

	exit, stdout, _ := runCLI(t, bin, "isolate", "--json", "--workdir", repo, "--repo", repo,
		"--run-id", "ex", "--check", "true", "--exec", "true",
		"--runtime", runtime, "--mint-cmd", "echo "+secret, "--revoke-cmd", "touch "+marker,
		"--execute", "--confirm")
	if exit != 0 {
		t.Fatalf("isolate execute exit=%d, want 0", exit)
	}
	iso := parseIsolate(t, stdout)
	if iso.Isolation == nil || !iso.Isolation.Minted || !iso.Isolation.Revoked || !iso.Isolation.Executed {
		t.Fatalf("lifecycle wrong: %+v", iso.Isolation)
	}
	if strings.Contains(stdout, secret) {
		t.Fatal("credential leaked into JSON output")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("revoke-cmd did not run")
	}
	if log, _ := os.ReadFile(rtLog); strings.Contains(string(log), secret) {
		t.Fatal("credential leaked into runtime argv (process list)")
	}
	if rcpt, _ := os.ReadFile(filepath.Join(repo, ".loopexec", "isolation-ex.json")); strings.Contains(string(rcpt), secret) {
		t.Fatal("credential leaked into isolation receipt")
	}
	if _, err := os.Stat(filepath.Join(repo, ".loopexec", "sandbox-ex", "agent.env")); err == nil {
		t.Fatal("agent env-file not removed")
	}
}
