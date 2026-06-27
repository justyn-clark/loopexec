package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type contextJSON struct {
	Status     string `json:"status"`
	HaltReason string `json:"halt_reason"`
	Context    *struct {
		TokensEstimated int      `json:"tokens_estimated"`
		BudgetTokens    int      `json:"budget_tokens"`
		FilesIncluded   []string `json:"files_included"`
		FilesDropped    []string `json:"files_dropped"`
	} `json:"context"`
}

func parseContext(t *testing.T, s string) contextJSON {
	t.Helper()
	var c contextJSON
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		t.Fatalf("not a single JSON object: %v\n%s", err, s)
	}
	return c
}

func TestEstimateTokens(t *testing.T) {
	if got := estimateTokens(strings.Repeat("a", 33)); got != 10 { // ceil(33/3.3)
		t.Fatalf("estimateTokens(33) = %d, want 10", got)
	}
	if got := estimateTokens(""); got != 0 {
		t.Fatalf("estimateTokens(\"\") = %d, want 0", got)
	}
}

func TestFenceFor(t *testing.T) {
	if got := fenceFor("no backticks"); got != "```" {
		t.Fatalf("fenceFor(plain) = %q, want 3", got)
	}
	if got := fenceFor("a ``` b"); got != "````" {
		t.Fatalf("fenceFor(```) = %q, want 4", got)
	}
	if got := fenceFor("````x"); got != "`````" {
		t.Fatalf("fenceFor(````) = %q, want 5", got)
	}
}

func TestExtractPaths(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package foo"), 0o644)
	os.WriteFile(filepath.Join(dir, "comp.tsx"), []byte("x"), 0o644)
	// foo.go + comp.tsx exist; missing.go and bar.ts do not. comp.tsx must match
	// as .tsx (longest-first), not truncate to .ts.
	failure := "panic\n at foo.go:10\n at missing.go:3\n at bar.ts:1\n ./foo.go\n at comp.tsx:2"
	var rels []string
	for _, rf := range extractPaths(dir, failure) {
		rels = append(rels, rf.Rel)
	}
	sort.Strings(rels)
	if strings.Join(rels, ",") != "comp.tsx,foo.go" {
		t.Fatalf("extractPaths = %v, want [comp.tsx foo.go]", rels)
	}
}

func TestExtractPathsConfinesTraversal(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "repo")
	os.MkdirAll(work, 0o755)
	os.WriteFile(filepath.Join(base, "secret.yaml"), []byte("k"), 0o644) // OUTSIDE work
	got := extractPaths(work, "boom at ../secret.yaml:1")
	if len(got) != 0 {
		t.Fatalf("extractPaths admitted a traversal path: %v", got)
	}
}

func TestMergeRelevant(t *testing.T) {
	a := []relevantFile{{Rel: "b"}, {Rel: "a"}}
	b := []relevantFile{{Rel: "a"}, {Rel: "c"}}
	var rels []string
	for _, rf := range mergeRelevant(a, b) {
		rels = append(rels, rf.Rel)
	}
	if strings.Join(rels, ",") != "a,b,c" {
		t.Fatalf("mergeRelevant = %v, want [a b c]", rels)
	}
}

func TestBuildContextIncludesFailureFile(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package foo\n// body\n"), 0o644)
	exit, stdout, _ := runCLI(t, bin, "build-context", "--json", "--workdir", dir, "--failure", "boom at foo.go:1")
	if exit != 0 {
		t.Fatalf("build-context exit=%d, want 0", exit)
	}
	c := parseContext(t, stdout)
	if c.Context == nil || len(c.Context.FilesIncluded) != 1 || c.Context.FilesIncluded[0] != "foo.go" {
		t.Fatalf("files_included=%v, want [foo.go]", c.Context)
	}
	if _, err := os.Stat(filepath.Join(dir, ".loopexec", "context.md")); err != nil {
		t.Fatal("context.md not written")
	}
}

func TestBuildContextBudgetUnsatisfiable(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "build-context", "--json", "--workdir", dir,
		"--failure", "a long enough failure to exceed three tokens", "--budget-tokens", "3")
	if exit != 19 {
		t.Fatalf("tiny-budget exit=%d, want 19", exit)
	}
	if c := parseContext(t, stdout); c.HaltReason != "context_budget_unsatisfiable" {
		t.Fatalf("halt_reason=%q, want context_budget_unsatisfiable", c.HaltReason)
	}
}

func TestBuildContextDropsOversizeFile(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "big.go"), []byte(strings.Repeat("x", 1000)), 0o644)
	exit, stdout, _ := runCLI(t, bin, "build-context", "--json", "--workdir", dir,
		"--failure", "fail at big.go:1", "--budget-tokens", "100")
	if exit != 0 {
		t.Fatalf("drop exit=%d, want 0", exit)
	}
	c := parseContext(t, stdout)
	if c.Context == nil || len(c.Context.FilesIncluded) != 0 {
		t.Fatalf("files_included=%v, want empty", c.Context)
	}
	if len(c.Context.FilesDropped) != 1 || c.Context.FilesDropped[0] != "big.go" {
		t.Fatalf("files_dropped=%v, want [big.go]", c.Context.FilesDropped)
	}
}

func TestBuildContextNoFilesNotFatal(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	exit, stdout, _ := runCLI(t, bin, "build-context", "--json", "--workdir", dir)
	if exit != 0 {
		t.Fatalf("no-files exit=%d, want 0", exit)
	}
	c := parseContext(t, stdout)
	if c.Status != "ok" || c.Context == nil || len(c.Context.FilesIncluded) != 0 {
		t.Fatalf("no-files: status=%q included=%v", c.Status, c.Context)
	}
	if c.Context.FilesIncluded == nil || c.Context.FilesDropped == nil {
		t.Fatal("file lists must serialize as [], not null")
	}
}

func TestBuildContextRespectsCeiling(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(strings.Repeat("x", 300)), 0o644)
	exit, stdout, _ := runCLI(t, bin, "build-context", "--json", "--workdir", dir,
		"--failure", "at a.go:1", "--budget-tokens", "500")
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	c := parseContext(t, stdout)
	if c.Context.TokensEstimated > c.Context.BudgetTokens {
		t.Fatalf("ceiling violated: %d > %d", c.Context.TokensEstimated, c.Context.BudgetTokens)
	}
}

// Security regression: untrusted failure text with a '..' path must not read a
// file outside the workdir into the agent-facing context.
func TestBuildContextRejectsTraversal(t *testing.T) {
	bin := buildTestBinary(t)
	base := t.TempDir()
	work := filepath.Join(base, "repo")
	os.MkdirAll(work, 0o755)
	os.WriteFile(filepath.Join(base, "secret.yaml"), []byte("API_KEY=sk-SECRET-12345"), 0o644)
	exit, stdout, _ := runCLI(t, bin, "build-context", "--json", "--workdir", work, "--failure", "boom at ../secret.yaml:1")
	if exit != 0 {
		t.Fatalf("exit=%d, want 0", exit)
	}
	if c := parseContext(t, stdout); len(c.Context.FilesIncluded) != 0 {
		t.Fatalf("traversal not rejected: %v", c.Context.FilesIncluded)
	}
	md, _ := os.ReadFile(filepath.Join(work, ".loopexec", "context.md"))
	if strings.Contains(string(md), "sk-SECRET-12345") {
		t.Fatal("secret leaked into context via traversal")
	}
}

// Security regression: a symlink inside the workdir that resolves OUTSIDE it
// must not leak its target.
func TestBuildContextRejectsSymlinkEscape(t *testing.T) {
	bin := buildTestBinary(t)
	base := t.TempDir()
	work := filepath.Join(base, "repo")
	os.MkdirAll(work, 0o755)
	secret := filepath.Join(base, "secret.yaml")
	os.WriteFile(secret, []byte("PASSWORD=hunter2"), 0o644)
	if err := os.Symlink(secret, filepath.Join(work, "innocent.go")); err != nil {
		t.Skip("symlinks unsupported on this platform")
	}
	exit, stdout, _ := runCLI(t, bin, "build-context", "--json", "--workdir", work, "--failure", "see innocent.go:1")
	if exit != 0 {
		t.Fatalf("exit=%d, want 0", exit)
	}
	if c := parseContext(t, stdout); len(c.Context.FilesIncluded) != 0 {
		t.Fatalf("symlink escape not rejected: %v", c.Context.FilesIncluded)
	}
	md, _ := os.ReadFile(filepath.Join(work, ".loopexec", "context.md"))
	if strings.Contains(string(md), "hunter2") {
		t.Fatal("secret leaked via symlink escape")
	}
}

func TestBuildContextOutConfinement(t *testing.T) {
	bin := buildTestBinary(t)
	base := t.TempDir()
	work := filepath.Join(base, "repo")
	os.MkdirAll(work, 0o755)
	exit, _, _ := runCLI(t, bin, "build-context", "--json", "--workdir", work, "--out", "../escape.md")
	if exit != 30 {
		t.Fatalf("--out escape exit=%d, want 30", exit)
	}
	if _, err := os.Stat(filepath.Join(base, "escape.md")); err == nil {
		t.Fatal("--out wrote outside workdir")
	}
}
