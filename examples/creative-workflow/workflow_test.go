package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testCommand(t *testing.T, dir, bin string, args ...string) (int, []byte) {
	t.Helper()
	c := exec.Command(bin, args...)
	c.Dir = dir
	b, e := c.CombinedOutput()
	if e == nil {
		return 0, b
	}
	if ex, ok := e.(*exec.ExitError); ok {
		return ex.ExitCode(), b
	}
	t.Fatal(e)
	return -1, b
}
func buildExamples(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	repo, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	for _, target := range []struct{ name, path string }{{"loopexec", "./cmd/loopexec"}, {"adapter", "./examples/creative-workflow"}} {
		code, b := testCommand(t, repo, "go", "build", "-o", filepath.Join(dir, target.name), target.path)
		if code != 0 {
			t.Fatalf("%s", b)
		}
	}
	return filepath.Join(dir, "loopexec"), filepath.Join(dir, "adapter")
}
func TestOfflineWorkflow(t *testing.T) {
	loop, adapter := buildExamples(t)
	cases := []struct {
		scenario              string
		code, attempts, calls int
		budget                string
	}{
		{"convergence", 10, 3, 9, "3"}, {"budget", 18, 2, 3, "0.9"}, {"regression", 10, 3, 9, "3"},
		{"stale", 13, 1, 3, "3"}, {"technical", 10, 2, 5, "3"}, {"cap", 12, 4, 12, "3"},
	}
	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			parent := t.TempDir()
			parent, _ = filepath.EvalSymlinks(parent)
			root := filepath.Join(parent, "fixture")
			if code, b := testCommand(t, parent, adapter, "init", "--dir", root, "--scenario", tc.scenario); code != 0 {
				t.Fatalf("%d %s", code, b)
			}
			code, b := testCommand(t, parent, loop, "run", "--json", "--workdir", root, "--workflow", "workflow.json", "--run-id", "fixture", "--max-iterations", "4", "--budget-usd", tc.budget, "--timeout", "20s", "--command-timeout", "3s", "--terminate-grace", "100ms")
			if code != tc.code {
				t.Fatalf("exit %d want %d: %s", code, tc.code, b)
			}
			var state struct {
				Iteration  int    `json:"iteration"`
				HaltReason string `json:"halt_reason"`
				Budget     struct {
					Calls    int    `json:"calls"`
					Mode     string `json:"mode"`
					MicroUSD int64  `json:"microusd"`
				} `json:"budget"`
				Candidate struct{ Best, Restored string } `json:"candidate"`
			}
			data, e := os.ReadFile(filepath.Join(root, ".loopexec", "run-fixture.state.json"))
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(data, &state); e != nil {
				t.Fatal(e)
			}
			if state.Iteration != tc.attempts || state.Budget.Calls != tc.calls || state.Budget.Mode != "strict" {
				t.Fatalf("%s", data)
			}
			var times []timing
			invocations := 0
			critics := 0
			for i := 1; i <= tc.attempts; i++ {
				if e := workflow.Read(filepath.Join(attemptDir(root, i), "timing.json"), &times); e != nil {
					if tc.scenario == "budget" && i == 2 {
						continue
					}
					t.Fatal(e)
				}
				seen := map[string]bool{}
				for _, v := range times {
					if seen[v.Phase] {
						t.Fatal("nested/repeated role call")
					}
					seen[v.Phase] = true
					invocations++
					if v.Phase == "critic" {
						critics++
					}
				}
			}
			if invocations != tc.calls {
				t.Fatal(invocations, tc.calls)
			}
			if tc.scenario == "technical" && critics != 1 {
				t.Fatal("technical failure did not skip critic")
			}
			if tc.scenario == "budget" {
				if _, e := os.Stat(attemptDir(root, 2)); !os.IsNotExist(e) {
					t.Fatal("work launched after reservation denial")
				}
			}
			if tc.scenario == "regression" {
				receipt, _ := os.ReadFile(filepath.Join(root, ".loopexec", "run-fixture.jsonl"))
				if !strings.Contains(string(receipt), `"event":"candidate_restored"`) {
					t.Fatal("regression not restored")
				}
				var ev evidence
				if e := workflow.Read(filepath.Join(attemptDir(root, 2), "evidence.json"), &ev); e != nil {
					t.Fatal(e)
				}
				failed := filepath.Join(root, ".loopexec", "fixture", "candidates", ev.CandidateID, "files", "assets", "generated.bin")
				if _, e := os.Stat(failed); e != nil {
					t.Fatal("rejected binary evidence lost", e)
				}
			}
			if tc.code == 10 {
				replayCode, out := testCommand(t, parent, loop, "replay", "--json", "--workdir", root, "--run-id", "fixture")
				if replayCode != 0 {
					t.Fatalf("replay %d: %s", replayCode, out)
				}
			}
			if tc.scenario == "convergence" {
				testVerificationTampering(t, root, loop)
			}
		})
	}
}
func testVerificationTampering(t *testing.T, root, loop string) {
	t.Helper()
	var req workflow.Request
	requestPath := filepath.Join(root, ".loopexec", "fixture", "request-000003-check.json")
	if e := workflow.Read(requestPath, &req); e != nil {
		t.Fatal(e)
	}
	v, e := verify(root, req)
	if e != nil || !v.Accepted || v.Total == nil || *v.Total != 8.6 {
		t.Fatal(v, e)
	}
	before := workflow.JSONHash(v)
	timingPath := filepath.Join(attemptDir(root, 3), "timing.json")
	timingHash, _ := bytesHash(timingPath)
	// Repeated verifier and CLI replay do not need the critic's private key.
	if e = os.Rename(filepath.Join(root, "private"), filepath.Join(root, "private-offline")); e != nil {
		t.Fatal(e)
	}
	defer os.Rename(filepath.Join(root, "private-offline"), filepath.Join(root, "private"))
	for i := 0; i < 2; i++ {
		got, e := verify(root, req)
		if e != nil || workflow.JSONHash(got) != before {
			t.Fatal(got, e)
		}
		if code, b := testCommand(t, root, loop, "replay", "--json", "--workdir", root, "--run-id", "fixture"); code != 0 {
			t.Fatalf("%d %s", code, b)
		}
	}
	if hash, _ := bytesHash(timingPath); hash != timingHash {
		t.Fatal("verification invoked roles")
	}
	for _, path := range []string{"candidate/assets/generated.bin", "policy.json", "engine.json", "reference.txt", "adapter.json", "tools/adapter", "evidence/attempt-000003/capture-front.txt"} {
		full := filepath.Join(root, path)
		original, e := os.ReadFile(full)
		if e != nil {
			t.Fatal(e)
		}
		mode, _ := os.Stat(full)
		os.WriteFile(full, []byte("tampered"), mode.Mode())
		_, e = verify(root, req)
		os.WriteFile(full, original, mode.Mode())
		if e == nil {
			t.Fatalf("accepted tampered %s", path)
		}
	}
	evPath := filepath.Join(attemptDir(root, 3), "evidence.json")
	original, _ := os.ReadFile(evPath)
	for _, kind := range []string{"candidate", "iteration", "test", "review", "builder_forgery", "captures"} {
		var ev evidence
		if e = workflow.Decode(original, &ev); e != nil {
			t.Fatal(e)
		}
		switch kind {
		case "candidate":
			ev.CandidateID = "wrong"
		case "iteration":
			ev.Iteration--
		case "test":
			ev.Tests.IDs = []string{"weakened"}
		case "review":
			ev.Review.Review.Categories[0] = 10
		case "captures":
			ev.Captures[0].Camera = "different"
		case "builder_forgery":
			_, key, _ := ed25519.GenerateKey(rand.Reader)
			b, _ := json.Marshal(ev.Review.Review)
			ev.Review.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, b))
		}
		if e = workflow.Atomic(evPath, ev); e != nil {
			t.Fatal(e)
		}
		_, e = verify(root, req)
		os.WriteFile(evPath, original, 0600)
		if e == nil {
			t.Fatal("accepted", kind)
		}
	}
	// The verifier computes category floors/aggregate, even for an authentic
	// signed critic response; no model-provided aggregate exists in the schema.
	var ev evidence
	workflow.Decode(original, &ev)
	keyBytes, _ := os.ReadFile(filepath.Join(root, "private-offline", "critic.key"))
	key, _ := base64.StdEncoding.DecodeString(string(keyBytes))
	ev.Review.Review.Categories = []float64{10, 10, 10, 10, 6}
	b, _ := json.Marshal(ev.Review.Review)
	ev.Review.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, b))
	workflow.Atomic(evPath, ev)
	got, e := verify(root, req)
	os.WriteFile(evPath, original, 0600)
	if e != nil || got.Accepted || got.Total == nil || *got.Total != 9.4 {
		t.Fatal(got, e)
	}
}
