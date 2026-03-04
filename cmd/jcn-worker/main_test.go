package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chdirRepoRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"version"}, &out, &out); err != nil {
		t.Fatalf("run version: %v", err)
	}
	if !strings.Contains(out.String(), "jcn-worker ") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunCallsLMStudioAndPrintsSelection(t *testing.T) {
	tmp := t.TempDir()
	chdirRepoRoot(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"OK."}}]}`))
	}))
	defer server.Close()
	t.Setenv("JCN_LMSTUDIO_BASE_URL", server.URL)

	task := writeFile(t, tmp, "task.json", `{"job_type":"repo_refactor","repo_size":"large","latency_budget":"balanced","context_need":"high","tool_calling_needed":true,"prompt":"Say OK."}`)
	registry := writeFile(t, tmp, "registry.json", `{"version":"1","machines":[{"id":"mac-studio","ram_gb":64,"role":"host"}],"models":[{"id":"qwen","family":"qwen","quant":"Q4_K_M","memory_gb_estimate":10.0,"capabilities":["repo_refactor"],"machine_allowlist":["mac-studio"],"priority":1,"status":"active"}]}`)
	policy := writeFile(t, tmp, "policy.json", `{"version":"1","job_type_priority":{"repo_refactor":["qwen"]},"machine_priority":["mac-studio"]}`)

	var out bytes.Buffer
	err := run([]string{"run", task, "--registry", registry, "--policy", policy}, &out, &out)
	if err != nil {
		t.Fatalf("run command: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "selected_model_id: qwen") || !strings.Contains(text, "selected_machine_target: mac-studio") || !strings.Contains(text, "OK.") {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestMissingTaskFile(t *testing.T) {
	chdirRepoRoot(t)
	var out bytes.Buffer
	err := run([]string{"run", "/missing/task.json"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "read task") {
		t.Fatalf("expected missing task error, got: %v", err)
	}
}

func TestUnsupportedJobType(t *testing.T) {
	tmp := t.TempDir()
	chdirRepoRoot(t)
	task := writeFile(t, tmp, "task.json", `{"job_type":"unknown"}`)
	registry := writeFile(t, tmp, "registry.json", `{"version":"1","machines":[{"id":"mac-studio","ram_gb":64,"role":"host"}],"models":[{"id":"qwen","family":"qwen","quant":"Q4_K_M","memory_gb_estimate":10.0,"capabilities":["repo_refactor"],"machine_allowlist":["mac-studio"],"priority":1,"status":"active"}]}`)
	policy := writeFile(t, tmp, "policy.json", `{"version":"1","job_type_priority":{"repo_refactor":["qwen"]},"machine_priority":["mac-studio"]}`)

	var out bytes.Buffer
	err := run([]string{"run", task, "--registry", registry, "--policy", policy}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "unsupported job_type") {
		t.Fatalf("expected unsupported job_type error, got: %v", err)
	}
}

func TestStablePromptHash(t *testing.T) {
	task := workerTask{JobType: "repo_refactor", RepoSize: "large", LatencyBudget: "balanced", ContextNeed: "high", ToolCallingNeeded: true, Prompt: "Pin prompt"}
	systemPrompt := "You are a deterministic coding assistant. Reply in one short sentence."
	p1 := hashBytes([]byte(systemPrompt + "\n" + buildUserPrompt(task)))
	p2 := hashBytes([]byte(systemPrompt + "\n" + buildUserPrompt(task)))
	if p1 != p2 {
		t.Fatalf("prompt hash is not stable")
	}
}

func TestRunRecordJSONFields(t *testing.T) {
	r := runRecord{RunID: "x", Status: "OK"}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal runRecord: %v", err)
	}
	if !strings.Contains(string(b), "runId") || !strings.Contains(string(b), "status") {
		t.Fatalf("missing expected json fields: %s", string(b))
	}
}
