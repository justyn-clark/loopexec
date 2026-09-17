package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/justyn-clark/loopexec/internal/subprocess"
	"github.com/justyn-clark/loopexec/internal/workflow"
)

type criticInput struct {
	Review review  `json:"review"`
	Score  float64 `json:"score"`
	Key    string  `json:"key"`
}
type timing struct {
	Phase      string `json:"phase"`
	DurationMS int64  `json:"duration_ms"`
	Exit       int    `json:"exit"`
}

func fixtureScore(s string, i int) (float64, bool) {
	switch s {
	case "regression":
		return []float64{8.1, 7.8, 8.6, 8.8}[min(i-1, 3)], true
	case "technical":
		if i == 1 {
			return 0, false
		}
		return 8.6, true
	case "cap":
		return 8 + float64(i)*.21, true
	}
	return []float64{7.8, 8.1, 8.6, 8.8}[min(i-1, 3)], true
}
func reservations(req workflow.Request) workflow.Preflight {
	calls := []workflow.Reservation{}
	for i, phase := range []string{"builder", "test", "critic"} {
		calls = append(calls, workflow.Reservation{ID: workflow.CallID(req.RunID, req.Iteration, req.Phase, phase), Phase: phase, MaxMicroUSD: []int64{400000, 100000, 200000}[i], MaxTokens: []int64{40, 0, 20}[i]})
	}
	return workflow.Preflight{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Enforced: true, Calls: calls}
}
func attempt(root string, req workflow.Request) error {
	a, p, c, e := readConfig(root)
	if e != nil {
		return e
	}
	if req.Iteration < 1 || req.Iteration > 4 {
		return fmt.Errorf("fixture permits at most four attempts")
	}
	if workflow.JSONHash(req.Reservations) != workflow.JSONHash(reservations(req).Calls) {
		return fmt.Errorf("missing or altered per-call allowance")
	}
	var w workflow.Config
	if e = workflow.Read(filepath.Join(root, "workflow.json"), &w); e != nil || workflow.JSONHash(w) != req.ConfigHash {
		return fmt.Errorf("workflow policy drift")
	}
	dir := attemptDir(root, req.Iteration)
	if e = os.Mkdir(dir, 0700); e != nil {
		return fmt.Errorf("attempt already exists or evidence root unavailable")
	}
	score, technical := fixtureScore(a.Scenario, req.Iteration)
	if a.Scenario == "cap" {
		score = 7 + float64(req.Iteration)*.21
	}
	feedback := "none"
	if sum, err := bytesHash(filepath.Join(root, "feedback.json")); err == nil {
		feedback = sum
	}
	input := roleInput{Candidate: filepath.Join(root, "candidate"), Revision: req.Iteration, Score: score, Technical: technical, FeedbackHash: feedback}
	if e = workflow.Read(filepath.Join(root, "feedback.json"), &input.Feedback); e != nil {
		return e
	}
	// Builder receives only synthetic work input, never a signing key or review.

	builderRequest := filepath.Join(root, "candidate", ".builder-request.json")
	if e = workflow.Atomic(builderRequest, input); e != nil {
		return e
	}
	usage := workflow.Usage{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: "exec", Calls: []workflow.Call{}}
	for _, r := range req.Reservations {
		usage.Calls = append(usage.Calls, workflow.Call{ID: r.ID, Phase: r.Phase, Status: "skipped"})
	}
	usagePath := filepath.Join(dir, "usage.json")
	if e = workflow.Atomic(usagePath, usage); e != nil {
		return e
	}
	timings := []timing{}
	invoke := func(index int, argv []string, stdin string) (subprocess.Result, error) {
		phase := []string{"builder", "test", "critic"}[index]
		v := subprocess.Run(context.Background(), subprocess.Options{Dir: root, Argv: argv, Stdin: stdin, Timeout: 2 * time.Second, Grace: 50 * time.Millisecond, JoinGroup: true})
		timings = append(timings, timing{phase, v.Duration.Milliseconds(), v.ExitCode})
		usd, tokens := []int64{300000, 50000, 100000}[index], []int64{30, 0, 10}[index]
		usage.Calls[index].Status = "actual"
		usage.Calls[index].MicroUSD = &usd
		usage.Calls[index].Tokens = &tokens
		if e := workflow.Atomic(usagePath, usage); e != nil {
			return v, e
		}
		if e := workflow.Atomic(filepath.Join(dir, "timing.json"), timings); e != nil {
			return v, e
		}
		if v.Cause != "" || v.Truncated {
			return v, fmt.Errorf("role infrastructure failure")
		}
		return v, nil
	}
	args := append(append([]string{}, a.Builder...), builderRequest)
	v, e := invoke(0, args, "")
	if e != nil || v.ExitCode != 0 {
		return fmt.Errorf("builder infrastructure failure")
	}
	m, e := workflow.Capture(context.Background(), input.Candidate, exampleCandidatePolicy())
	if e != nil {
		return e
	}
	id := workflow.JSONHash(m)
	// The independent test invocation receives candidate and pinned engine config.
	args = append(append([]string{}, a.Test...), builderRequest, filepath.Join(root, "engine.json"))
	v, e = invoke(1, args, "")
	if e != nil {
		return e
	}
	var tests testResult
	if workflow.Decode([]byte(v.Stdout), &tests) != nil || tests.CandidateID != id || (v.ExitCode == 0) != tests.Passed {
		return fmt.Errorf("test infrastructure failure")
	}
	ev := evidence{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, CandidateID: id, WorkflowHash: req.ConfigHash, AdapterHash: workflow.JSONHash(a), PolicyHash: workflow.JSONHash(p), ConfigHash: workflow.JSONHash(c), ReferenceHash: c.ReferenceHash, Manifest: m, Tests: tests, Captures: []capture{}, Synthetic: true}
	if tests.Passed {
		for i, view := range p.Views {
			name := fmt.Sprintf("capture-%s.txt", view)
			data := []byte(fmt.Sprintf("SYNTHETIC fixture capture candidate=%s view=%s camera=%s\n", id, view, c.CameraIDs[i]))
			if e = os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
				return e
			}
			ev.Captures = append(ev.Captures, capture{View: view, Camera: c.CameraIDs[i], Path: name, SHA256: workflow.Hash(data), Synthetic: true})
		}
		if c.RequiresMotion {
			data := []byte("SYNTHETIC motion evidence " + id)
			if e = os.WriteFile(filepath.Join(dir, "motion.txt"), data, 0600); e != nil {
				return e
			}
			ev.Captures = append(ev.Captures, capture{View: "motion", Camera: "motion-fixed", Path: "motion.txt", SHA256: workflow.Hash(data), Synthetic: true})
		}
		// Claim the candidate's single review before launching the critic. A crash
		// leaves an unresolved claim; it never authorizes a second model invocation.
		claim, e := os.OpenFile(filepath.Join(root, "reviews", id+".claim"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return fmt.Errorf("candidate review already claimed")
		}
		if e = claim.Close(); e != nil {
			return e
		}
		r := review{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, CandidateID: id, PolicyHash: ev.PolicyHash, ConfigHash: ev.ConfigHash, ReferenceHash: ev.ReferenceHash, TestHash: workflow.JSONHash(tests), CapturesHash: workflow.JSONHash(ev.Captures), Synthetic: true}
		key, e := os.ReadFile(filepath.Join(root, "private", "critic.key"))
		if e != nil {
			return e
		}
		b, _ := json.Marshal(criticInput{Review: r, Score: score, Key: string(key)})
		v, e = invoke(2, a.Critic, string(b))
		if e != nil || v.ExitCode != 0 {
			return fmt.Errorf("critic infrastructure failure")
		}
		var signed signedReview
		if workflow.Decode([]byte(v.Stdout), &signed) != nil || !checkSignature(a, signed) {
			return fmt.Errorf("critic signature invalid")
		}
		ev.Review = &signed
		if e = workflow.Atomic(filepath.Join(root, "reviews", id+".json"), signed); e != nil {
			return e
		}

		if e = workflow.Atomic(filepath.Join(root, "feedback.json"), signed.Review.Issues); e != nil {
			return e
		}
	} else {
		if e = workflow.Atomic(filepath.Join(root, "feedback.json"), []issue{{1, "technical", "repair synthetic technical failure before review"}}); e != nil {
			return e
		}
	}
	ev.Usage = usage
	if e = workflow.Atomic(filepath.Join(dir, "evidence.json"), ev); e != nil {
		return e
	}
	if a.Scenario == "stale" {
		// Deliberately invalidate the candidate binding after publishing evidence.
		if e = os.WriteFile(filepath.Join(input.Candidate, "assets", "generated.bin"), []byte("tampered fixture"), 0600); e != nil {
			return e
		}
	}
	return nil
}
func builderRole(path string) error {
	var input roleInput
	if e := workflow.Read(path, &input); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Join(input.Candidate, "assets"), 0700); e != nil {
		return e
	}
	p := project{SchemaVersion: 1, Revision: input.Revision, SyntheticScore: input.Score, Technical: input.Technical, FeedbackHash: input.FeedbackHash}
	if e := workflow.Atomic(filepath.Join(input.Candidate, "project.json"), p); e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(input.Candidate, "assets", "generated.bin"), []byte{0, 1, 2, byte(input.Revision), 255}, 0600)
}
func testRole(path, configPath string) (testResult, error) {
	var input roleInput
	var c engineConfig
	var p project
	if e := workflow.Read(path, &input); e != nil {
		return testResult{}, e
	}
	if e := workflow.Read(configPath, &c); e != nil {
		return testResult{}, e
	}
	if e := workflow.Read(filepath.Join(input.Candidate, "project.json"), &p); e != nil {
		return testResult{}, e
	}
	m, e := workflow.Capture(context.Background(), input.Candidate, exampleCandidatePolicy())
	if e != nil {
		return testResult{}, e
	}
	_, e = os.Stat(filepath.Join(input.Candidate, "assets", "generated.bin"))
	return testResult{SchemaVersion: 1, CandidateID: workflow.JSONHash(m), Passed: p.Technical && e == nil, IDs: []string{"asset_exists", "scene_load"}, ConfigHash: workflow.JSONHash(c), Synthetic: true}, nil
}
func criticRole(input criticInput) (signedReview, error) {
	key, e := base64.StdEncoding.DecodeString(input.Key)
	if e != nil || len(key) != ed25519.PrivateKeySize {
		return signedReview{}, fmt.Errorf("critic credential unavailable")
	}
	r := input.Review
	r.Categories = []float64{input.Score, input.Score, input.Score, input.Score, input.Score}
	r.Issues = []issue{}
	if input.Score < 8.5 {
		r.Issues = []issue{{1, "composition", "improve synthetic composition fixture"}, {2, "lighting", "improve synthetic lighting fixture"}}
	}
	r.Model = model{Provider: "fixture", ID: "synthetic-independent-critic", Version: "1"}
	b, _ := json.Marshal(r)
	return signedReview{Review: r, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, b))}, nil
}
