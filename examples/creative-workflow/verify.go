package main

import (
	"context"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"math"
	"path/filepath"
)

// verify is read-only and agent-free. It checks the saved synthetic evidence;
// it neither captures new images nor makes a new performance measurement.
func verify(root string, req workflow.Request) (verdict, error) {
	a, p, c, e := readConfig(root)
	if e != nil {
		return verdict{}, e
	}
	var ev evidence
	dir := attemptDir(root, req.Iteration)
	if e = workflow.Read(filepath.Join(dir, "evidence.json"), &ev); e != nil {
		return verdict{}, e
	}
	var w workflow.Config
	if e = workflow.Read(filepath.Join(root, "workflow.json"), &w); e != nil {
		return verdict{}, e
	}
	binaryHash, _, err := workflow.FileHash(context.Background(), filepath.Join(root, "tools", "adapter"))
	if err != nil || binaryHash != a.BinaryHash {
		return verdict{}, fmt.Errorf("verifier binary drift")
	}
	ref, e := bytesHash(filepath.Join(root, "reference.txt"))
	if e != nil {
		return verdict{}, e
	}
	m, e := workflow.Capture(context.Background(), filepath.Join(root, "candidate"), exampleCandidatePolicy())
	if e != nil {
		return verdict{}, e
	}
	id := workflow.JSONHash(m)
	if ev.SchemaVersion != 1 || !ev.Synthetic || ev.RunID != req.RunID || ev.Iteration != req.Iteration || ev.CandidateID != id || req.CandidateID != id || workflow.JSONHash(ev.Manifest) != id || ev.WorkflowHash != req.ConfigHash || ev.WorkflowHash != workflow.JSONHash(w) || ev.AdapterHash != workflow.JSONHash(a) || ev.PolicyHash != workflow.JSONHash(p) || ev.ConfigHash != workflow.JSONHash(c) || ev.ReferenceHash != ref || c.ReferenceHash != ref {
		return verdict{}, fmt.Errorf("stale or wrong-candidate evidence")
	}
	t := ev.Tests
	if !t.Synthetic || t.SchemaVersion != 1 || t.CandidateID != id || t.ConfigHash != workflow.JSONHash(c) || workflow.JSONHash(t.IDs) != workflow.JSONHash([]string{"asset_exists", "scene_load"}) {
		return verdict{}, fmt.Errorf("test evidence invalid")
	}
	v := verdict{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, CandidateID: id, PolicyHash: ev.PolicyHash, ConfigHash: ev.ConfigHash, Technical: t.Passed, Synthetic: true, Issues: []issue{}}
	if ev.Usage.RunID != req.RunID || ev.Usage.Iteration != req.Iteration || ev.Usage.Phase != "exec" || ev.Usage.SchemaVersion != 1 || len(ev.Usage.Calls) != 3 {
		return v, fmt.Errorf("usage evidence invalid")
	}
	if !t.Passed {
		if ev.Review != nil || len(ev.Captures) > 0 {
			return v, fmt.Errorf("technical failure cannot have critic acceptance")
		}
		return v, nil
	}
	if ev.Review == nil || !checkSignature(a, *ev.Review) {
		return v, fmt.Errorf("review authorship invalid")
	}
	r := ev.Review.Review
	if r.SchemaVersion != 1 || r.RunID != req.RunID || r.Iteration != req.Iteration || r.CandidateID != id || r.PolicyHash != ev.PolicyHash || r.ConfigHash != ev.ConfigHash || r.ReferenceHash != ref || r.TestHash != workflow.JSONHash(t) || r.CapturesHash != workflow.JSONHash(ev.Captures) || !r.Synthetic || r.Model.Provider != "fixture" || r.Model.ID != "synthetic-independent-critic" || r.Model.Version != "1" {
		return v, fmt.Errorf("review binding invalid")
	}
	if len(ev.Captures) < 6 || len(ev.Captures) > 6+p.ExploratoryMax {
		return v, fmt.Errorf("capture count invalid")
	}
	seen := map[string]bool{}
	for i, cap := range ev.Captures {
		if cap.View == "" || seen[cap.View] || !cap.Synthetic || !workflow.CleanRelative(cap.Path) {
			return v, fmt.Errorf("capture schema invalid")
		}
		seen[cap.View] = true
		if i < 6 && (cap.View != p.Views[i] || cap.Camera != c.CameraIDs[i]) {
			return v, fmt.Errorf("fixed camera drift")
		}
		path, e := workflow.SafePath(dir, cap.Path)
		if e != nil {
			return v, e
		}
		hash, e := bytesHash(path)
		if e != nil || hash != cap.SHA256 {
			return v, fmt.Errorf("capture tampering")
		}
	}
	if c.RequiresMotion {
		if !seen["motion"] {
			return v, fmt.Errorf("motion evidence required")
		}
	}
	if len(r.Categories) != 5 {
		return v, fmt.Errorf("category count invalid")
	}
	total := 0.0
	floors := true
	for i, score := range r.Categories {
		if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 10 {
			return v, fmt.Errorf("score out of domain")
		}
		total += score * float64(p.Weights[i]) / 100
		if score < p.Floor {
			floors = false
		}
	}
	total = math.Round(total*1e9) / 1e9
	for i, issue := range r.Issues {
		if issue.Rank != i+1 || issue.ID == "" || issue.Detail == "" {
			return v, fmt.Errorf("ranked feedback invalid")
		}
	}
	v.Veto = !floors
	v.Reviewed = true
	v.Total = &total
	v.Categories = r.Categories
	v.Issues = r.Issues
	v.Accepted = floors && total >= p.Target
	return v, nil
}
