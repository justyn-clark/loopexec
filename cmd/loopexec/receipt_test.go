package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type receiptJSON struct {
	Status     string `json:"status"`
	HaltReason string `json:"halt_reason"`
	Verified   *bool  `json:"verified"`
	Signature  string `json:"signature"`
}

func parseReceipt(t *testing.T, s string) receiptJSON {
	t.Helper()
	var r receiptJSON
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, s)
	}
	return r
}

func TestNormalizeOutput(t *testing.T) {
	if got := normalizeOutput("a  \nb\t\n\n"); got != "a\nb" {
		t.Fatalf("normalizeOutput = %q, want %q", got, "a\nb")
	}
}

func TestRunRecordsReceiptPins(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ctx.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLI(t, bin, "run", "--json", "--run-id", "p", "--workdir", dir, "--check", "true",
		"--model-id", "claude-opus-4", "--model-provider", "anthropic", "--model-version", "v1",
		"--temperature", "0", "--seed", "0", "--max-tokens", "4096",
		"--context-file", "ctx.txt", "--cost-usd", "0.42")

	data, err := os.ReadFile(filepath.Join(dir, ".loopexec", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]any
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"model", "sampling", "context_manifest", "cost_usd", "fingerprint", "check"} {
		if _, ok := st[k]; !ok {
			t.Fatalf("state missing receipt field %q", k)
		}
	}
	if fp, _ := st["fingerprint"].(map[string]any); fp["output_sha256"] == nil {
		t.Fatal("fingerprint missing output_sha256")
	}
}

func TestReplayVerifiesAndDetectsMismatch(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	ok := filepath.Join(dir, ".ok")
	if err := os.WriteFile(ok, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Converges because .ok exists; fingerprint pins exit 0.
	runCLI(t, bin, "run", "--json", "--run-id", "v", "--workdir", dir, "--check", "test -f .ok")

	exit, stdout, _ := runCLI(t, bin, "replay", "--json", "--workdir", dir)
	if exit != 0 {
		t.Fatalf("replay (match) exit=%d, want 0", exit)
	}
	if r := parseReceipt(t, stdout); r.Status != "verified" || r.Verified == nil || !*r.Verified {
		t.Fatalf("replay match: status=%q verified=%v", r.Status, r.Verified)
	}

	// Remove the file: the recorded check now produces a different verdict.
	os.Remove(ok)
	exit2, stdout2, _ := runCLI(t, bin, "replay", "--json", "--workdir", dir)
	if exit2 != 13 {
		t.Fatalf("replay (mismatch) exit=%d, want 13", exit2)
	}
	if r := parseReceipt(t, stdout2); r.HaltReason != "objective_unverified" {
		t.Fatalf("replay mismatch halt_reason=%q, want objective_unverified", r.HaltReason)
	}
}

func TestAttestSignAndVerify(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	runCLI(t, bin, "run", "--json", "--run-id", "a", "--workdir", dir, "--check", "true")

	exit, stdout, _ := runCLI(t, bin, "attest", "--json", "--workdir", dir, "--key", "k1")
	if exit != 0 {
		t.Fatalf("attest sign exit=%d, want 0", exit)
	}
	if r := parseReceipt(t, stdout); r.Status != "attested" || r.Signature == "" {
		t.Fatalf("attest sign: status=%q sig=%q", r.Status, r.Signature)
	}

	exitV, stdoutV, _ := runCLI(t, bin, "attest", "--json", "--workdir", dir, "--key", "k1", "--verify")
	if exitV != 0 {
		t.Fatalf("attest verify (right key) exit=%d, want 0", exitV)
	}
	if r := parseReceipt(t, stdoutV); r.Verified == nil || !*r.Verified {
		t.Fatalf("attest verify (right key) verified=%v", r.Verified)
	}

	// Wrong key must fail verification.
	exitW, _, _ := runCLI(t, bin, "attest", "--json", "--workdir", dir, "--key", "k2", "--verify")
	if exitW != 13 {
		t.Fatalf("attest verify (wrong key) exit=%d, want 13", exitW)
	}
}
