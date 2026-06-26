package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type opsJSON struct {
	Status     string `json:"status"`
	HaltReason string `json:"halt_reason"`
	Ops        *struct {
		Samples      int            `json:"samples"`
		Converged    int            `json:"converged"`
		Distribution map[string]int `json:"distribution"`
		Packet       string         `json:"packet"`
		Stale        *bool          `json:"stale"`
	} `json:"ops"`
}

func parseOps(t *testing.T, s string) opsJSON {
	t.Helper()
	var o opsJSON
	if err := json.Unmarshal([]byte(s), &o); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, s)
	}
	return o
}

func readStateMap(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".loopexec", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCopyTreeSkipsLoopexec(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(src, ".loopexec"), 0o755)
	os.WriteFile(filepath.Join(src, ".loopexec", "state.json"), []byte("{}"), 0o644)
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Fatal("a.txt not copied")
	}
	if _, err := os.Stat(filepath.Join(dst, ".loopexec")); !os.IsNotExist(err) {
		t.Fatal(".loopexec should be skipped")
	}
}

func TestComprehensionGate(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "run", "--json", "--run-id", "c", "--workdir", dir,
		"--check", "false", "--max-iterations", "10", "--comprehension-every", "2")
	if exit != 19 {
		t.Fatalf("comprehension exit=%d, want 19", exit)
	}
	if o := parseOps(t, stdout); o.HaltReason != "comprehension_debt_exceeded" {
		t.Fatalf("halt_reason=%q, want comprehension_debt_exceeded", o.HaltReason)
	}
}

func TestEscalateAndAck(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "e", "--workdir", dir, "--check", "false", "--max-iterations", "1")

	exit, stdout, _ := runCLI(t, bin, "escalate", "--json", "--workdir", dir, "--channel", "file")
	if exit != 0 {
		t.Fatalf("escalate exit=%d, want 0", exit)
	}
	if o := parseOps(t, stdout); o.Status != "paged" || o.Ops == nil || o.Ops.Packet == "" {
		t.Fatalf("escalate: status=%q ops=%v", o.Status, o.Ops)
	}
	if esc, _ := readStateMap(t, dir)["escalation"].(map[string]any); esc["state"] != "paged" {
		t.Fatalf("escalation state=%v, want paged", readStateMap(t, dir)["escalation"])
	}

	// ack requires a reviewer.
	if exitNo, _, _ := runCLI(t, bin, "ack", "--json", "--workdir", dir); exitNo != 20 {
		t.Fatalf("ack without reviewer exit=%d, want 20", exitNo)
	}
	exitA, stdoutA, _ := runCLI(t, bin, "ack", "--json", "--workdir", dir, "--reviewer", "justyn-clark")
	if exitA != 0 {
		t.Fatalf("ack exit=%d, want 0", exitA)
	}
	if o := parseOps(t, stdoutA); o.Status != "acked" {
		t.Fatalf("ack status=%q, want acked", o.Status)
	}
	esc, _ := readStateMap(t, dir)["escalation"].(map[string]any)
	if esc["state"] != "acked" || esc["acked_by"] != "justyn-clark" {
		t.Fatalf("escalation after ack=%v", esc)
	}
}

func TestWatchAliveAndStale(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "w", "--workdir", dir, "--check", "false", "--max-iterations", "1")

	// Fresh heartbeat -> alive.
	exit, stdout, _ := runCLI(t, bin, "watch", "--json", "--workdir", dir, "--stall-timeout", "600")
	if exit != 0 {
		t.Fatalf("watch alive exit=%d, want 0", exit)
	}
	if o := parseOps(t, stdout); o.Status != "alive" || o.Ops == nil || o.Ops.Stale == nil || *o.Ops.Stale {
		t.Fatalf("watch alive: status=%q stale=%v", o.Status, o.Ops)
	}

	// Force the heartbeat old -> stale.
	hbPath := filepath.Join(dir, ".loopexec", "heartbeat")
	data, _ := os.ReadFile(hbPath)
	var hb map[string]any
	json.Unmarshal(data, &hb)
	hb["ts"] = hb["ts"].(float64) - 1000
	out, _ := json.Marshal(hb)
	os.WriteFile(hbPath, out, 0o644)

	exitS, stdoutS, _ := runCLI(t, bin, "watch", "--json", "--workdir", dir, "--stall-timeout", "10")
	if exitS != 19 {
		t.Fatalf("watch stale exit=%d, want 19", exitS)
	}
	if o := parseOps(t, stdoutS); o.HaltReason != "heartbeat_stale" {
		t.Fatalf("watch stale halt_reason=%q, want heartbeat_stale", o.HaltReason)
	}
}

func TestReexecute(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	// A loop that converges (sets .n so the check passes).
	runCLI(t, bin, "run", "--json", "--run-id", "rx", "--workdir", dir,
		"--exec", "echo 1 > .n", "--check", "test -f .n", "--max-iterations", "3")

	// Refuses without --confirm.
	if exit, _, _ := runCLI(t, bin, "reexecute", "--json", "--workdir", dir, "--samples", "2"); exit != 20 {
		t.Fatalf("reexecute without --confirm exit=%d, want 20", exit)
	}

	exit, stdout, _ := runCLI(t, bin, "reexecute", "--json", "--workdir", dir, "--samples", "3", "--confirm")
	if exit != 0 {
		t.Fatalf("reexecute exit=%d, want 0", exit)
	}
	o := parseOps(t, stdout)
	if o.Ops == nil || o.Ops.Samples != 3 || o.Ops.Converged != 3 {
		t.Fatalf("reexecute ops=%+v, want 3/3 converged", o.Ops)
	}
}
