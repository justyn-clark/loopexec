package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type candidateState struct {
	AttemptArchive  string            `json:"attempt_archive_id,omitempty"`
	Restoration     *restoreProgress  `json:"restoration,omitempty"`
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
	copyArtifact         func(context.Context, string, string, workflow.Artifact) error
	recoveryDeadline     time.Time
}

type restoreProgress struct {
	DeadlineNS   int64  `json:"deadline_ns,omitempty"`
	FailureCause string `json:"failure_cause,omitempty"`
	Target       string `json:"target_id"`
	Phase        string `json:"phase"`
	Completed    int    `json:"completed_files"`
	Total        int    `json:"total_files"`
}

func (c *candidateManager) recoveryDeadlineNS() int64 {
	if c.recoveryDeadline.IsZero() {
		return 0
	}
	return c.recoveryDeadline.UnixNano()
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
	for _, id := range []string{c.state.Best, c.state.Initial, c.state.Accepted, c.state.Attempted, c.state.Restored, c.state.AttemptArchive} {
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
	c.state.AttemptArchive = ""
	c.state.Restoration = nil
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
	sourceStatus := c.state.Status
	c.state.Status, c.state.Exposed = "restoring", false
	if e := c.save(); e != nil {
		return e
	}
	// Preserve the policy-included attempt before any rollback mutation. This is
	// recovery evidence only; it never changes the accepted/best pointers.
	if c.state.AttemptArchive == "" {
		id, err := c.capture()
		if err != nil {
			return err
		}
		archive := struct {
			Kind         string `json:"kind"`
			Accepted     bool   `json:"accepted"`
			Snapshot     string `json:"snapshot_id"`
			SourceStatus string `json:"source_status"`
		}{"attempt-recovery-only", false, id, sourceStatus}
		if err = workflow.Atomic(filepath.Join(c.store, "attempt-recovery-"+id+".json"), archive); err != nil {
			return err
		}
		c.state.AttemptArchive = id
	}
	c.state.Status = "restoring"
	c.state.Exposed = false
	if e := c.save(); e != nil {
		return e
	}
	id := c.state.Best
	if id == "" {
		id = c.state.Initial
	}
	c.state.Restoration = &restoreProgress{Target: id, Phase: "validating", DeadlineNS: c.recoveryDeadlineNS()}
	if err := c.save(); err != nil {
		return err
	}
	base := filepath.Join(c.store, id)
	var m workflow.Manifest
	if e := workflow.Read(filepath.Join(base, "manifest.json"), &m); e != nil || workflow.JSONHash(m) != id {
		return fmt.Errorf("restoration snapshot invalid")
	}
	if e := c.validateSnapshot(id); e != nil {
		return e
	}
	current, e := workflow.Capture(c.ctx, c.root, c.policy)
	if e != nil {
		return e
	}
	stage, e := os.MkdirTemp(c.store, "restore-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	copyFile := c.copyArtifact
	if copyFile == nil {
		copyFile = workflow.CopyArtifact
	}
	progress := func(phase string, n int) error {
		c.state.Restoration = &restoreProgress{Target: id, Phase: phase, Completed: n, Total: len(m.Files), DeadlineNS: c.recoveryDeadlineNS()}
		if n%64 == 0 || n == len(m.Files) {
			return c.save()
		}
		return c.ctx.Err()
	}
	if e = progress("staging", 0); e != nil {
		return e
	}
	for i, a := range m.Files {
		if e = copyFile(c.ctx, filepath.Join(base, "files"), stage, a); e != nil {
			return e
		}
		if e = progress("staging", i+1); e != nil {
			return e
		}
	}
	staged, e := workflow.Capture(c.ctx, stage, c.policy)
	if e != nil || workflow.JSONHash(staged) != id {
		return fmt.Errorf("restoration stage invalid")
	}
	if e = workflow.SyncDirectories(c.ctx, stage); e != nil {
		return e
	}
	if e = progress("pruning", 0); e != nil {
		return e
	}
	if e = c.clearRestoreObstructions(m, current); e != nil {
		return e
	}
	if e = progress("applying", 0); e != nil {
		return e
	}
	for i, a := range m.Files {
		if e = copyFile(c.ctx, stage, c.root, a); e != nil {
			return e
		}
		if e = progress("applying", i+1); e != nil {
			return e
		}
	}
	if e = progress("verifying", 0); e != nil {
		return e
	}
	actual, e := workflow.Capture(c.ctx, c.root, c.policy)
	if e != nil || workflow.JSONHash(actual) != id {
		return fmt.Errorf("restoration verification failed")
	}
	parents := map[string]bool{c.root: true}
	for _, files := range [][]workflow.Artifact{m.Files, current.Files} {
		for _, a := range files {
			for parent := filepath.Dir(filepath.Join(c.root, a.Path)); parent != c.root; parent = filepath.Dir(parent) {
				parents[parent] = true
			}
		}
	}
	for parent := range parents {
		if e = c.ctx.Err(); e != nil {
			return e
		}
		info, err := os.Stat(parent)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			continue
		}
		if e = workflow.SyncDirectory(parent); e != nil {
			return e
		}
	}
	if e = c.ctx.Err(); e != nil {
		return e
	}
	next := c.state
	next.Restored, next.Status, next.Exposed = id, "restored", c.state.Best != ""
	next.Restoration = &restoreProgress{Target: id, Phase: "verified", Completed: len(m.Files), Total: len(m.Files), DeadlineNS: c.recoveryDeadlineNS()}
	if e = workflow.Atomic(c.journal, next); e != nil {
		return e
	}
	c.state = next
	return nil
}

func (c *candidateManager) clearRestoreObstructions(target, current workflow.Manifest) error {
	wanted := map[string]bool{}
	for _, a := range target.Files {
		wanted[a.Path] = true
	}
	for _, a := range current.Files {
		if wanted[a.Path] {
			continue
		}
		if err := c.ctx.Err(); err != nil {
			return err
		}
		p, err := workflow.SafePath(c.root, a.Path)
		if err != nil {
			return err
		}
		if err = os.Remove(p); err != nil {
			return err
		}
	}
	for _, a := range target.Files {
		p, err := workflow.SafePath(c.root, a.Path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			continue
		}
		var dirs []string
		err = filepath.WalkDir(p, func(item string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err = c.ctx.Err(); err != nil {
				return err
			}
			rel, err := filepath.Rel(c.root, item)
			if err != nil {
				return err
			}
			for _, excluded := range c.policy.Exclude {
				if rel == excluded || strings.HasPrefix(rel, excluded+string(filepath.Separator)) {
					return fmt.Errorf("restoration blocked by excluded content")
				}
			}
			if !d.IsDir() {
				return fmt.Errorf("restoration blocked by retained content")
			}
			dirs = append(dirs, item)
			return nil
		})
		if err != nil {
			return err
		}
		for i := len(dirs) - 1; i >= 0; i-- {
			if err = c.ctx.Err(); err != nil {
				return err
			}
			if err = os.Remove(dirs[i]); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *candidateManager) hasVerifiedRecovery() bool {
	p := c.state.Restoration
	return !c.recoveryDeadline.IsZero() && c.state.Status == "restored" && p != nil && p.Phase == "verified" && p.DeadlineNS == c.recoveryDeadline.UnixNano() && p.Target == c.state.Best && c.state.Restored == c.state.Best
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
	ctx := c.ctx
	if id == c.state.Best && c.hasVerifiedRecovery() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(context.Background(), c.recoveryDeadline)
		defer cancel()
	}
	m, e := workflow.Capture(ctx, filepath.Join(base, "files"), c.policy)
	if e != nil || workflow.JSONHash(m) != id {
		return fmt.Errorf("snapshot content corrupt")
	}
	return nil
}
func validCandidateID(id string) bool { b, e := hex.DecodeString(id); return e == nil && len(b) == 32 }
