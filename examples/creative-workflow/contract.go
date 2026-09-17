package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
)

type policy struct {
	SchemaVersion  int      `json:"schema_version"`
	Views          []string `json:"fixed_views"`
	ExploratoryMax int      `json:"exploratory_max"`
	Weights        []int    `json:"weights"`
	Floor          float64  `json:"category_floor"`
	Target         float64  `json:"target"`
}
type engineConfig struct {
	SchemaVersion  int      `json:"schema_version"`
	CameraIDs      []string `json:"camera_ids"`
	RequiresMotion bool     `json:"requires_motion"`
	Renderer       string   `json:"renderer"`
	ReferenceHash  string   `json:"reference_hash"`
}
type adapterConfig struct {
	BinaryHash    string   `json:"binary_hash"`
	SchemaVersion int      `json:"schema_version"`
	Synthetic     bool     `json:"synthetic"`
	Scenario      string   `json:"scenario"`
	Builder       []string `json:"builder"`
	Test          []string `json:"test"`
	Critic        []string `json:"critic"`
	PublicKey     string   `json:"critic_public_key"`
}
type project struct {
	SchemaVersion  int     `json:"schema_version"`
	Revision       int     `json:"revision"`
	SyntheticScore float64 `json:"synthetic_score"`
	Technical      bool    `json:"technical"`
	FeedbackHash   string  `json:"feedback_hash"`
}
type roleInput struct {
	Feedback     []issue `json:"ranked_feedback"`
	Candidate    string  `json:"candidate"`
	Revision     int     `json:"revision"`
	Score        float64 `json:"score"`
	Technical    bool    `json:"technical"`
	FeedbackHash string  `json:"feedback_hash"`
}
type testResult struct {
	SchemaVersion int      `json:"schema_version"`
	CandidateID   string   `json:"candidate_id"`
	Passed        bool     `json:"passed"`
	IDs           []string `json:"ids"`
	ConfigHash    string   `json:"config_hash"`
	Synthetic     bool     `json:"synthetic"`
}
type capture struct {
	View      string `json:"view"`
	Camera    string `json:"camera"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Synthetic bool   `json:"synthetic"`
}
type issue struct {
	Rank   int    `json:"rank"`
	ID     string `json:"id"`
	Detail string `json:"detail"`
}
type model struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Version  string `json:"version"`
}
type review struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	Iteration     int       `json:"iteration"`
	CandidateID   string    `json:"candidate_id"`
	PolicyHash    string    `json:"policy_hash"`
	ConfigHash    string    `json:"config_hash"`
	ReferenceHash string    `json:"reference_hash"`
	TestHash      string    `json:"test_hash"`
	CapturesHash  string    `json:"captures_hash"`
	Categories    []float64 `json:"categories"`
	Issues        []issue   `json:"ranked_issues"`
	Model         model     `json:"model"`
	Synthetic     bool      `json:"synthetic"`
}
type signedReview struct {
	Review    review `json:"review"`
	Signature string `json:"signature"`
}
type evidence struct {
	SchemaVersion int               `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Iteration     int               `json:"iteration"`
	CandidateID   string            `json:"candidate_id"`
	WorkflowHash  string            `json:"workflow_hash"`
	AdapterHash   string            `json:"adapter_hash"`
	PolicyHash    string            `json:"policy_hash"`
	ConfigHash    string            `json:"config_hash"`
	ReferenceHash string            `json:"reference_hash"`
	Manifest      workflow.Manifest `json:"artifact_manifest"`
	Tests         testResult        `json:"tests"`
	Captures      []capture         `json:"captures"`
	Review        *signedReview     `json:"review,omitempty"`
	Usage         workflow.Usage    `json:"usage"`
	Synthetic     bool              `json:"synthetic"`
}
type verdict struct {
	Veto          bool      `json:"veto"`
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	Iteration     int       `json:"iteration"`
	CandidateID   string    `json:"candidate_id"`
	PolicyHash    string    `json:"policy_hash"`
	ConfigHash    string    `json:"config_hash"`
	Technical     bool      `json:"technical"`
	Reviewed      bool      `json:"reviewed"`
	Categories    []float64 `json:"categories,omitempty"`
	Total         *float64  `json:"total,omitempty"`
	Accepted      bool      `json:"accepted"`
	Issues        []issue   `json:"ranked_issues"`
	Synthetic     bool      `json:"synthetic"`
}

func exampleCandidatePolicy() workflow.CandidatePolicy {
	return workflow.CandidatePolicy{Root: "candidate", Include: []string{"project.json", "assets"}, Exclude: []string{"assets/cache"}, Protected: []string{"policy.json", "engine.json", "reference.txt", "adapter.json", "workflow.json"}}
}
func readConfig(root string) (adapterConfig, policy, engineConfig, error) {
	var a adapterConfig
	var p policy
	var c engineConfig
	for path, target := range map[string]any{"adapter.json": &a, "policy.json": &p, "engine.json": &c} {
		if e := workflow.Read(filepath.Join(root, path), target); e != nil {
			return a, p, c, e
		}
	}
	if a.SchemaVersion != 1 || !a.Synthetic || len(a.Builder) == 0 || len(a.Test) == 0 || len(a.Critic) == 0 {
		return a, p, c, fmt.Errorf("only explicit simulated fixture roles are enabled")
	}
	if p.SchemaVersion != 1 || workflow.JSONHash(p.Views) != workflow.JSONHash([]string{"front", "left", "right", "back", "top", "detail"}) || workflow.JSONHash(p.Weights) != workflow.JSONHash([]int{25, 25, 20, 15, 15}) || p.Floor != 7 || p.Target != 8.5 || p.ExploratoryMax != 2 {
		return a, p, c, fmt.Errorf("invalid fixture review policy")
	}
	if c.SchemaVersion != 1 || len(c.CameraIDs) != 6 || c.Renderer != "synthetic" {
		return a, p, c, fmt.Errorf("invalid fixture engine config")
	}
	seen := map[string]bool{}
	for _, id := range c.CameraIDs {
		if id == "" || seen[id] {
			return a, p, c, fmt.Errorf("invalid camera IDs")
		}
		seen[id] = true
	}
	return a, p, c, nil
}
func attemptDir(root string, iter int) string {
	return filepath.Join(root, "evidence", fmt.Sprintf("attempt-%06d", iter))
}
func bytesHash(path string) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return "", e
	}
	if len(b) > workflow.MaxEvidence {
		return "", fmt.Errorf("fixture evidence oversized")
	}
	return workflow.Hash(b), nil
}
func checkSignature(a adapterConfig, s signedReview) bool {
	pub, e := base64.StdEncoding.DecodeString(a.PublicKey)
	if e != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, e := base64.StdEncoding.DecodeString(s.Signature)
	if e != nil {
		return false
	}
	data, _ := json.Marshal(s.Review)
	return ed25519.Verify(pub, data, sig)
}
