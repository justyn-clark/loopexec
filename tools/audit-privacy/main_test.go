package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostPath(t *testing.T) {
	for _, value := range []string{
		"cd /Users/example/repo", "GOCACHE=/private/tmp/cache go test ./...",
		"/home/example/repo", "(/var/tmp/output)", "'/opt/homebrew/bin/small'",
		`C:\Users\example\repo`, `D:/temp/output`, `\\server\share\file`,
		"file:///workspace/output", "~/private-file", "//server/share/file",
		"https://example.org/?path=/Users/example/repo",
	} {
		if !hostPath(value) {
			t.Errorf("missed host path: %q", value)
		}
	}
	for _, value := range []string{
		"go test ./...", "build/proofs/run.json", ".small-cache/logs/replay/commands/proof.txt",
		"build/tmp/output", "examples/home/model.json",
		"https://github.com/justyn-clark/loopexec/pull/7", "s3://public-bucket/object",
		"$WORKSPACE/build/output", "[workspace]/build/output", "builder/checker", "score 4/5",
	} {
		if hostPath(value) {
			t.Errorf("rejected portable value: %q", value)
		}
	}
}

func TestCanonicalHistoryAndEncoding(t *testing.T) {
	base := []byte("entries:\n  - evidence: legacy /Users/example/work\n")
	good := append(append([]byte{}, base...), []byte("  - evidence: checked build/proofs\n")...)
	if hits, err := inspect(".small/progress.small.yml", good, base); err != nil || len(hits) != 0 {
		t.Fatalf("preserved history rejected: %v %v", hits, err)
	}
	bad := append(append([]byte{}, base...), []byte("  - evidence: \"\\u002fUsers\\u002fexample\\u002fwork\"\n")...)
	if hits, err := inspect(".small/progress.small.yml", bad, base); err != nil || len(hits) != 1 {
		t.Fatalf("encoded new path not rejected: %v %v", hits, err)
	}
	if _, err := inspect(".small/progress.small.yml", []byte("entries: []\n"), base); err == nil {
		t.Fatal("removed history accepted")
	}
	if _, err := inspect(".small/progress.small.yml", []byte("entries:\n  - evidence: changed\n"), base); err == nil {
		t.Fatal("rewritten history accepted")
	}
	if hits, err := inspect(".small/handoff.small.yml", []byte("summary: /Users/example/work\n"), []byte("summary: safe\n")); err != nil || len(hits) != 1 {
		t.Fatal("handoff path accepted")
	}
	if _, err := inspect(".small/handoff.small.yml", []byte("summary: [\n"), nil); err == nil {
		t.Fatal("invalid YAML accepted")
	}
}

func TestExactCommandProof(t *testing.T) {
	path := ".small-command-proofs/logs/replay/commands/proof.txt"
	legacy := []byte("cd /Users/example/work")
	if hits, err := inspect(path, legacy, legacy); err != nil || len(hits) != 0 {
		t.Fatal("unchanged legacy proof rejected")
	}
	if _, err := inspect(path, []byte("cd [workspace]"), legacy); err == nil {
		t.Fatal("altered proof accepted")
	}
	if hits, err := inspect(path, legacy, nil); err != nil || len(hits) != 1 {
		t.Fatal("new unsafe proof accepted")
	}
	if hits, err := inspect(path, []byte("go test ./..."), nil); err != nil || len(hits) != 0 {
		t.Fatal("portable proof rejected")
	}
}

func TestPrivateKeysStayOutOfDiagnostics(t *testing.T) {
	locations := newPaths(map[string]any{"/Users/example/private": "/home/example/work"}, nil, "record")
	if len(locations) != 2 {
		t.Fatalf("expected key and value violations: %v", locations)
	}
	for _, location := range locations {
		if strings.Contains(location, "/Users/") || strings.Contains(location, "/home/") {
			t.Fatal("private key exposed in diagnostic location")
		}
	}
}

func TestChecksUntrackedFiles(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git failed: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "--quiet")
	run("config", "user.name", "Privacy fixture")
	run("config", "user.email", "fixture@example.invalid")
	write(".small/progress.small.yml", "entries:\n  - evidence: legacy /Users/example/work\n")
	run("add", ".small")
	run("commit", "--quiet", "-m", "Legacy fixture")
	base := run("rev-parse", "HEAD")
	if err := check(root, base); err != nil {
		t.Fatal(err)
	}
	write(".small-command-proofs/new.txt", "cd /Users/example/work")
	if err := check(root, base); err == nil {
		t.Fatal("untracked unsafe proof accepted")
	}
	write(".small-command-proofs/new.txt", "go test ./...")
	if err := check(root, base); err != nil {
		t.Fatal(err)
	}
	run("rm", ".small/progress.small.yml")
	if err := check(root, base); err == nil {
		t.Fatal("deleted historical progress accepted")
	}
}
