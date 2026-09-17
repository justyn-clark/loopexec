package main

import (
	"context"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"testing"
)

func candidateFixture(t *testing.T) (*candidateManager, string) {
	t.Helper()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	for _, p := range []string{"candidate/assets/cache", "runtime"} {
		if e := os.MkdirAll(filepath.Join(root, p), 0700); e != nil {
			t.Fatal(e)
		}
	}
	os.WriteFile(filepath.Join(root, "policy.json"), []byte("{}"), 0600)
	os.WriteFile(filepath.Join(root, "candidate/assets/generated.bin"), []byte{0, 255, 1}, 0600)
	os.WriteFile(filepath.Join(root, "candidate/assets/cache/keep"), []byte("cache"), 0600)
	os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("user work"), 0600)
	p := workflow.CandidatePolicy{Root: "candidate", Include: []string{"assets"}, Exclude: []string{"assets/cache"}, Protected: []string{"policy.json"}}
	c, e := newCandidate(context.Background(), root, filepath.Join(root, "runtime"), p)
	if e != nil {
		t.Fatal(e)
	}
	return c, root
}
func TestCandidateRestoresBinaryAndPreservesUnrelated(t *testing.T) {
	c, root := candidateFixture(t)
	if e := c.begin(); e != nil {
		t.Fatal(e)
	}
	id, e := c.attempted()
	if e != nil {
		t.Fatal(e)
	}
	if e = c.promote(&checkFingerprint{ExitCode: 1}); e != nil {
		t.Fatal(e)
	}
	c.begin()
	os.WriteFile(filepath.Join(c.root, "assets/generated.bin"), []byte("regression"), 0600)
	os.WriteFile(filepath.Join(c.root, "assets/new.bin"), []byte{9, 0, 9}, 0600)
	rejected, e := c.attempted()
	if e != nil {
		t.Fatal(e)
	}
	if rejected == id {
		t.Fatal("same snapshot")
	}
	if e = c.restore(); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(c.root, "assets/generated.bin"))
	if string(b) != string([]byte{0, 255, 1}) {
		t.Fatal(b)
	}
	if _, e = os.Stat(filepath.Join(c.root, "assets/new.bin")); !os.IsNotExist(e) {
		t.Fatal("rejected generated file remains")
	}
	for _, p := range []string{"unrelated.txt", "candidate/assets/cache/keep", "runtime/candidates/" + rejected + "/files/assets/new.bin"} {
		if _, e = os.Stat(filepath.Join(root, p)); e != nil {
			t.Fatal(e)
		}
	}
	if c.state.Best != id || c.state.Restored != id || c.state.Attempted != rejected || !c.state.Exposed {
		t.Fatalf("%+v", c.state)
	}
}
func TestCandidateInterruptedPromotionAndFailedRestore(t *testing.T) {
	c, root := candidateFixture(t)
	first, _ := c.attempted()
	c.promote(&checkFingerprint{ExitCode: 1})
	c.begin()
	os.WriteFile(filepath.Join(c.root, "assets/generated.bin"), []byte("uncommitted"), 0600)
	second, _ := c.attempted()
	c.state.Status = "promoting"
	c.save()
	fresh, e := newCandidate(context.Background(), root, filepath.Join(root, "runtime"), c.policy)
	if e != nil {
		t.Fatal(e)
	}
	if fresh.state.Best != first || fresh.state.Best == second {
		t.Fatal("half promotion accepted")
	}
	if e = fresh.restore(); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(c.store, first, "files/assets/generated.bin"), []byte("corrupt snapshot"), 0600)
	if e = fresh.restore(); e == nil {
		t.Fatal("corrupt restore accepted")
	}
	if fresh.state.Exposed || fresh.state.Status != "restoring" {
		t.Fatalf("%+v", fresh.state)
	}
}
func TestCandidatePathAndProtectedGuards(t *testing.T) {
	c, root := candidateFixture(t)
	os.WriteFile(filepath.Join(root, "policy.json"), []byte("weakened"), 0600)
	if c.intact(root) == nil {
		t.Fatal("policy drift")
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "keep"), []byte("outside"), 0600)
	if e := os.Symlink(outside, filepath.Join(c.root, "assets/link")); e != nil {
		t.Fatal(e)
	}
	if _, e := c.capture(); e == nil {
		t.Fatal("symlink followed")
	}
	b, _ := os.ReadFile(filepath.Join(outside, "keep"))
	if string(b) != "outside" {
		t.Fatal("unrelated changed")
	}
	for _, p := range []workflow.CandidatePolicy{{Root: "..", Include: []string{"x"}}, {Root: "candidate", Include: []string{"assets", "assets/file"}}, {Root: "candidate", Include: []string{"../outside"}}} {
		if workflow.ValidateManifestPolicy(p) == nil {
			t.Fatal(p)
		}
	}
}
