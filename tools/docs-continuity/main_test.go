package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeployedDocumentationGate(t *testing.T) {
	doc := []byte("# Canonical documentation\n")
	m := manifest{1, "0.4.0", "source-digest", "docs-digest", []fileDigest{{"README.md", digest(doc)}}}
	for _, mode := range []string{"matching", "stale-manifest", "stale-home", "stale-content", "missing", "invalid", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				switch r.URL.Path {
				case "/loopexec-documentation.json":
					actual := m
					if mode == "stale-manifest" {
						actual.Version = "0.2.0"
					}
					if mode == "invalid" {
						fmt.Fprint(w, "{broken")
						return
					}
					if err := json.NewEncoder(w).Encode(actual); err != nil {
						t.Error(err)
					}
				case "/":
					if mode != "stale-home" {
						fmt.Fprint(w, siteMarker(m))
					}
				case "/loopexec-documentation/README.md":
					if mode == "missing" {
						http.NotFound(w, r)
						return
					}
					if mode == "stale-content" {
						fmt.Fprint(w, "old documentation")
						return
					}
					if _, err := w.Write(doc); err != nil {
						t.Error(err)
					}
				default:
					http.NotFound(w, r)
				}
			})
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, r)
				return response.Result(), nil
			})}
			err := verifySite(client, "https://example.invalid", m)
			if (err == nil) != (mode == "matching") {
				t.Fatalf("gate returned %v for %s", err, mode)
			}
		})
	}
}

func TestDocsCoChangeGate(t *testing.T) {
	for _, tc := range []struct {
		name, paths string
		pass        bool
	}{
		{"code-only", "cmd/loopexec/run.go\n", false},
		{"changelog-only", "internal/process/run.go\nCHANGELOG.md\n", false},
		{"generated-only", "cmd/loopexec/main.go\ndocs/cli-reference.generated.md\nCHANGELOG.md\n", false},
		{"unrelated-note", "cmd/loopexec/main.go\ndocs/status-and-roadmap.md\nCHANGELOG.md\n", false},
		{"paired", "cmd/loopexec/run.go\ndocs/workflows.md\nCHANGELOG.md\n", true},
		{"workflow-only", ".github/workflows/release.yml\n", false},
		{"policy-only", "AGENTS.md\n", false},
		{"docs-only", "docs/cli.md\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "../../scripts/check-docs-sync.sh")
			cmd.Stdin = strings.NewReader(tc.paths)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.pass {
				t.Fatalf("gate: %v: %s", err, out)
			}
		})
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"cmd/loopexec", "internal", "examples", "scripts", "tools", ".github/workflows", "docs/releases"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "examples/scoped-budget"), 0755); err != nil {
		t.Fatal(err)
	}
	files := []string{"README.md", "SPEC.md", "CHANGELOG.md", "docs/documentation-files.json", "docs/cli.md", "docs/cli-reference.generated.md", "docs/documentation-continuity.md", "docs/releases/v0.4.0.md", "examples/scoped-budget/README.md"}
	for _, path := range append(append([]string{}, files...), "go.mod", "go.sum", "AGENTS.md") {
		if err := os.WriteFile(filepath.Join(root, path), []byte("fixture\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(struct {
		Files []string `json:"files"`
	}{files})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs/documentation-files.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd/loopexec/main.go"), []byte("package main\nconst toolVersion = \"0.4.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBundleBoundToSourceAndDocs(t *testing.T) {
	root := fixture(t)
	before, err := makeManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd/loopexec/new.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	after, err := makeManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if before.SourceSHA256 == after.SourceSHA256 || before.DocumentationSHA256 != after.DocumentationSHA256 {
		t.Fatal("source binding did not isolate source change")
	}
	if err := os.WriteFile(filepath.Join(root, "docs/cli.md"), []byte("changed docs\n"), 0644); err != nil {
		t.Fatal(err)
	}
	docChanged, err := makeManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.DocumentationSHA256 == docChanged.DocumentationSHA256 {
		t.Fatal("documentation mutation escaped binding")
	}
	out := filepath.Join(t.TempDir(), "documentation")
	if err := writeBundle(root, out); err != nil {
		t.Fatal(err)
	}
	if err := writeBundle(root, out); err == nil {
		t.Fatal("overwrote existing bundle")
	}
	if _, err := verifyBundle(root, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved manifest
	if err := decode(data, &saved); err != nil {
		t.Fatal(err)
	}
	for _, file := range saved.Files {
		data, err := safeRead(out, file.Path)
		if err != nil || digest(data) != file.SHA256 {
			t.Fatalf("bad bundled document %s: %v", file.Path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(out, "README.md"), []byte("tampered\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyBundle(root, out); err == nil {
		t.Fatal("tampered bundle accepted")
	}
}

func TestUnsafeDocumentationRejected(t *testing.T) {
	for _, path := range []string{".env", "docs/private-handoff.md", "docs/.private/note.md", "docs/../../private.md", "build/private.md"} {
		if publicPath(path) {
			t.Fatalf("non-public path accepted: %s", path)
		}
	}
	root := fixture(t)
	if _, err := safeRead(root, "../private.txt"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err := os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "docs/link.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := safeRead(root, "docs/link.md"); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, _, err := hashFiles(root, []string{"README.md", "README.md"}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "docs/documentation-files.json"), []byte(`{"files":["README.md"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := makeManifest(root); err == nil {
		t.Fatal("incomplete public allowlist accepted")
	}
}

func TestReleasePublicationGatesRemainWired(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	check := strings.Index(text, "verify-site")
	create := strings.Index(text, "gh release create")
	publish := strings.Index(text, "--draft=false")
	if check < 0 || create < check || publish < create || !strings.Contains(text[create:publish], "verify-site") {
		t.Fatal("deployed docs must be verified before draft creation and immediately before publication")
	}
	data, err = os.ReadFile("../../scripts/build-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"TestDocumentationContract", "docs-continuity bundle", "documentation", "zip -qr"} {
		if !strings.Contains(string(data), token) {
			t.Fatalf("packaging gate missing: %s", token)
		}
	}
}
