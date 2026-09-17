package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
)

type candidateState struct {
	BestIteration   int               `json:"best_iteration,omitempty"`
	Numeric         *numericState     `json:"numeric,omitempty"`
	Attempted       string            `json:"attempted_id,omitempty"`
	Accepted        string            `json:"accepted_id,omitempty"`
	Restored        string            `json:"restored_id,omitempty"`
	Best            string            `json:"best_id,omitempty"`
	Initial         string            `json:"initial_id,omitempty"`
	Status          string            `json:"status"`
	Exposed         bool              `json:"best_exposed"`
	Protected       []manifestEntry   `json:"protected"`
	BestFingerprint *checkFingerprint `json:"best_fingerprint,omitempty"`
}
type candidateManager struct {
	iteration            int
	root, store, journal string
	policy               workflow.CandidatePolicy
	state                candidateState
	ctx                  context.Context
}

func newCandidate(ctx context.Context, workdir, dir string, p workflow.CandidatePolicy) (*candidateManager, error) {
	if e := workflow.ValidateManifestPolicy(p); e != nil {
		return nil, e
	}
	root, e := workflow.SafePath(workdir, p.Root)
	if e != nil {
		return nil, e
	}
	if workflow.Within(root, dir) || workflow.Within(dir, root) {
		return nil, fmt.Errorf("candidate and runtime must be disjoint")
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	c := &candidateManager{root: root, store: filepath.Join(dir, "candidates"), journal: filepath.Join(dir, "candidate.json"), policy: p, ctx: ctx}
	if e = os.MkdirAll(c.store, 0700); e != nil {
		return nil, e
	}
	e = workflow.Read(c.journal, &c.state)
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	for _, id := range []string{c.state.Best, c.state.Initial, c.state.Accepted, c.state.Attempted, c.state.Restored} {
		if id != "" && !validCandidateID(id) {
			return nil, fmt.Errorf("invalid saved candidate ID")
		}
	}
	current := []manifestEntry{}
	for _, path := range p.Protected {
		abs, err := workflow.SafePath(workdir, path)
		if err != nil {
			return nil, err
		}
		if workflow.Within(root, abs) {
			return nil, fmt.Errorf("protected evidence must be outside candidate")
		}
		sum, _, err := hashProtected(ctx, abs)
		if err != nil {
			return nil, err
		}
		current = append(current, manifestEntry{path, sum})
	}
	if os.IsNotExist(e) {
		c.state = candidateState{Status: "initial", Protected: current}
	} else if workflow.JSONHash(current) != workflow.JSONHash(c.state.Protected) {
		return nil, fmt.Errorf("protected policy drift")
	}
	if c.state.Initial == "" {
		id, err := c.capture()
		if err != nil {
			return nil, err
		}
		c.state.Initial = id
		if err = c.save(); err != nil {
			return nil, err
		}
	}
	return c, nil
}
func hashProtected(ctx context.Context, path string) (string, int64, error) {
	return workflow.FileHash(ctx, path)
}
func (c *candidateManager) save() error { return workflow.Atomic(c.journal, c.state) }
func (c *candidateManager) intact(workdir string) error {
	for _, pin := range c.state.Protected {
		p, e := workflow.SafePath(workdir, pin.Path)
		if e != nil {
			return e
		}
		sum, _, e := hashProtected(c.ctx, p)
		if e != nil || sum != pin.SHA256 {
			return fmt.Errorf("protected policy drift")
		}
	}
	return nil
}
func (c *candidateManager) capture() (string, error) {
	m, e := workflow.Capture(c.ctx, c.root, c.policy)
	if e != nil {
		return "", e
	}
	id := workflow.JSONHash(m)
	final := filepath.Join(c.store, id)
	if _, e = os.Stat(final); e == nil {
		var old workflow.Manifest
		if e = workflow.Read(filepath.Join(final, "manifest.json"), &old); e != nil || workflow.JSONHash(old) != id {
			return "", fmt.Errorf("snapshot corrupt")
		}
		if e = c.validateSnapshot(id); e != nil {
			return "", e
		}
		return id, nil
	}
	tmp, e := os.MkdirTemp(c.store, "pending-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(tmp)
	if e = os.Mkdir(filepath.Join(tmp, "files"), 0700); e != nil {
		return "", e
	}
	for _, a := range m.Files {
		if e = workflow.CopyArtifact(c.ctx, c.root, filepath.Join(tmp, "files"), a); e != nil {
			return "", e
		}
	}
	if e = workflow.Atomic(filepath.Join(tmp, "manifest.json"), m); e != nil {
		return "", e
	}
	if e = workflow.SyncDirectories(c.ctx, tmp); e != nil {
		return "", e
	}
	if e = os.Rename(tmp, final); e != nil {
		return "", e
	}
	if e = workflow.SyncDirectory(c.store); e != nil {
		return "", e
	}
	return id, nil
}
func (c *candidateManager) begin() error {
	c.state.Status = "attempting"
	c.state.Exposed = false
	c.state.Attempted = ""
	return c.save()
}
func (c *candidateManager) attempted() (string, error) {
	id, e := c.capture()
	if e != nil {
		return "", e
	}
	c.state.Attempted = id
	c.state.Status = "evaluating"
	return id, c.save()
}
func (c *candidateManager) promote(fp *checkFingerprint, progress ...*numericState) error {
	if e := c.validateSnapshot(c.state.Attempted); e != nil {
		return e
	}
	// Previous Best remains authoritative through staging.
	// Recovery discards an interrupted promotion.
	// interrupted promotion and restores the last fully committed pointer.
	c.state.Status = "promoting"
	if e := c.save(); e != nil {
		return e
	}
	if e := c.ctx.Err(); e != nil {
		return e
	}
	next := c.state
	next.BestIteration = c.iteration
	if len(progress) > 0 && progress[0] != nil {
		v := *progress[0]
		next.Numeric = &v
	}
	next.Best = next.Attempted
	next.Accepted = next.Attempted
	next.Status = "accepted"
	next.Exposed = true
	next.BestFingerprint = fp
	if e := workflow.Atomic(c.journal, next); e != nil {
		return e
	}
	c.state = next
	return nil
}
func (c *candidateManager) restore() error {
	c.state.Status = "restoring"
	c.state.Exposed = false
	if e := c.save(); e != nil {
		return e
	}
	id := c.state.Best
	if id == "" {
		id = c.state.Initial
	}
	base := filepath.Join(c.store, id)
	var m workflow.Manifest
	if e := workflow.Read(filepath.Join(base, "manifest.json"), &m); e != nil || workflow.JSONHash(m) != id {
		return fmt.Errorf("restoration snapshot invalid")
	}
	current, e := workflow.Capture(c.ctx, c.root, c.policy)
	if e != nil {
		return e
	}
	wanted := map[string]bool{}
	for _, a := range m.Files {
		wanted[a.Path] = true
	}
	for _, a := range current.Files {
		if !wanted[a.Path] {
			p, e := workflow.SafePath(c.root, a.Path)
			if e != nil {
				return e
			}
			if e = c.ctx.Err(); e != nil {
				return e
			}
			if e = os.Remove(p); e != nil {
				return e
			}
		}
	}
	for _, a := range m.Files {
		if e = workflow.CopyArtifact(c.ctx, filepath.Join(base, "files"), c.root, a); e != nil {
			return e
		}
	}
	actual, e := workflow.Capture(c.ctx, c.root, c.policy)
	if e != nil || workflow.JSONHash(actual) != id {
		return fmt.Errorf("restoration verification failed")
	}
	c.state.Restored = id
	c.state.Status = "restored"
	c.state.Exposed = c.state.Best != ""
	return c.save()
}
func (c *candidateManager) validateSnapshot(id string) error {
	if !validCandidateID(id) {
		return fmt.Errorf("invalid snapshot ID")
	}
	base := filepath.Join(c.store, id)
	var manifest workflow.Manifest
	if e := workflow.Read(filepath.Join(base, "manifest.json"), &manifest); e != nil || workflow.JSONHash(manifest) != id {
		return fmt.Errorf("invalid snapshot manifest")
	}
	m, e := workflow.Capture(c.ctx, filepath.Join(base, "files"), c.policy)
	if e != nil || workflow.JSONHash(m) != id {
		return fmt.Errorf("snapshot content corrupt")
	}
	return nil
}
func validCandidateID(id string) bool { b, e := hex.DecodeString(id); return e == nil && len(b) == 32 }
