// creative-workflow is a one-attempt fixture adapter, never an outer loop.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"io"
	"os"
	"path/filepath"
)

func initialize(requested, scenario string) error {
	switch scenario {
	case "convergence", "budget", "regression", "stale", "technical", "cap":
	default:
		return fmt.Errorf("unknown fixture scenario")
	}
	abs, e := filepath.Abs(requested)
	if e != nil {
		return e
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(abs))
	if e != nil {
		return e
	}
	root := filepath.Join(parent, filepath.Base(abs))
	if e = os.Mkdir(root, 0700); e != nil {
		return fmt.Errorf("fixture directory must be new: %w", e)
	}
	for _, p := range []string{"candidate", "candidate/assets", "evidence", "private", "tools", "reviews"} {
		if e = os.MkdirAll(filepath.Join(root, p), 0700); e != nil {
			return e
		}
	}
	self, e := os.Executable()
	if e != nil {
		return e
	}
	data, e := os.ReadFile(self)
	if e != nil {
		return e
	}
	executable := filepath.Join(root, "tools", "adapter")
	if e = os.WriteFile(executable, data, 0700); e != nil {
		return e
	}
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(root, "private", "critic.key"), []byte(base64.StdEncoding.EncodeToString(key)), 0600); e != nil {
		return e
	}
	ref := []byte("SYNTHETIC reference fixture. No rendered image or engine performance claim.\n")
	if e = os.WriteFile(filepath.Join(root, "reference.txt"), ref, 0600); e != nil {
		return e
	}
	p := policy{SchemaVersion: 1, Views: []string{"front", "left", "right", "back", "top", "detail"}, ExploratoryMax: 2, Weights: []int{25, 25, 20, 15, 15}, Floor: 7, Target: 8.5}
	c := engineConfig{SchemaVersion: 1, CameraIDs: []string{"fixed-front", "fixed-left", "fixed-right", "fixed-back", "fixed-top", "fixed-detail"}, Renderer: "synthetic", ReferenceHash: workflow.Hash(ref)}
	a := adapterConfig{SchemaVersion: 1, Synthetic: true, Scenario: scenario, Builder: []string{executable, "builder-role"}, Test: []string{executable, "test-role"}, Critic: []string{executable, "critic-role"}, PublicKey: base64.StdEncoding.EncodeToString(pub), BinaryHash: workflow.Hash(data)}
	candidate := exampleCandidatePolicy()
	candidate.Protected = append(candidate.Protected, "tools/adapter")
	w := workflow.Config{SchemaVersion: 1, Exec: []string{executable, "attempt"}, Check: []string{executable, "verify"}, Meter: workflow.MeterPolicy{Mode: "strict", Reserve: []string{executable, "reserve"}, Usage: []string{executable, "usage"}, MaxCalls: 12, MaxTokens: 240}, Candidate: &candidate, Score: &workflow.ScorePolicy{Command: []string{executable, "score"}, Direction: "maximize", Min: 0, Max: 10, Target: 8.5, MinImprovement: .2, Tolerance: 0, Patience: 2}}
	for path, obj := range map[string]any{"policy.json": p, "engine.json": c, "adapter.json": a, "workflow.json": w, "candidate/project.json": project{SchemaVersion: 1}, "feedback.json": []issue{}} {
		if e = workflow.Atomic(filepath.Join(root, path), obj); e != nil {
			return e
		}
	}
	fmt.Println(root)
	return nil
}
func output(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
func command(args []string) (int, error) {
	if len(args) == 0 {
		return 2, fmt.Errorf("use init, attempt, verify, score, reserve, or usage")
	}
	if args[0] == "init" {
		fs := flag.NewFlagSet("init", flag.ContinueOnError)
		dir := fs.String("dir", "", "New fixture directory")
		scenario := fs.String("scenario", "convergence", "convergence|budget|regression|stale|technical|cap")
		if e := fs.Parse(args[1:]); e != nil {
			return 2, e
		}
		if *dir == "" {
			return 2, fmt.Errorf("--dir required")
		}
		return 0, initialize(*dir, *scenario)
	}
	if args[0] == "critic-role" {
		b, e := io.ReadAll(io.LimitReader(os.Stdin, workflow.MaxEvidence+1))
		if e != nil {
			return 2, e
		}
		var input criticInput
		if e = workflow.Decode(b, &input); e != nil {
			return 2, e
		}
		r, e := criticRole(input)
		if e != nil {
			return 2, e
		}
		return 0, output(r)
	}
	if args[0] == "builder-role" {
		if len(args) != 2 {
			return 2, fmt.Errorf("builder input required")
		}
		return 0, builderRole(args[1])
	}
	if args[0] == "test-role" {
		if len(args) != 3 {
			return 2, fmt.Errorf("test input and config required")
		}
		r, e := testRole(args[1], args[2])
		if e != nil {
			return 2, e
		}
		code := 0
		if !r.Passed {
			code = 1
		}
		return code, output(r)
	}
	if len(args) != 2 {
		return 2, fmt.Errorf("one governor request file is required")
	}
	var req workflow.Request
	if e := workflow.Read(args[1], &req); e != nil {
		return 2, e
	}
	if req.SchemaVersion != 1 || req.RunID == "" || req.Iteration < 0 {
		return 2, fmt.Errorf("invalid request")
	}
	root, e := os.Getwd()
	if e != nil {
		return 2, e
	}
	switch args[0] {
	case "attempt":
		return 0, attempt(root, req)
	case "reserve":
		return 0, output(reservations(req))
	case "usage":
		var usage workflow.Usage
		if e := workflow.Read(filepath.Join(attemptDir(root, req.Iteration), "usage.json"), &usage); e != nil {
			return 2, e
		}
		return 0, output(usage)
	case "verify", "score":
		v, e := verify(root, req)
		if e != nil {
			return 1, e
		}
		if args[0] == "score" {
			return 0, output(workflow.Score{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, CandidateID: req.CandidateID, Reviewed: v.Reviewed, Technical: v.Technical, Value: v.Total, Veto: v.Veto})
		}
		code := 1
		if v.Accepted {
			code = 0
		}
		return code, output(v)
	default:
		return 2, fmt.Errorf("unknown operation")
	}
}
func main() {
	code, e := command(os.Args[1:])
	if e != nil {
		fmt.Fprintln(os.Stderr, "creative-workflow: evidence or infrastructure error:", e)
		if code == 0 {
			code = 2
		}
	}
	os.Exit(code)
}
