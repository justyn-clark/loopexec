package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRouteSuccess(t *testing.T) {
	dir := t.TempDir()
	task := writeFile(t, dir, "task.json", `{"job_type":"repo_refactor","repo_size":"large","latency_budget":"balanced","context_need":"high","tool_calling_needed":true}`)
	registry := writeFile(t, dir, "registry.json", `{
  "version":"1",
  "machines":[{"id":"mac-studio","ram_gb":64,"role":"model-host"},{"id":"mac-mini","ram_gb":16,"role":"daemon"}],
  "models":[
    {"id":"qwen","family":"qwen","quant":"Q4_K_M","memory_gb_estimate":10.0,"capabilities":["repo_refactor"],"machine_allowlist":["mac-mini","mac-studio"],"priority":1,"status":"active"}
  ]
}`)
	policy := writeFile(t, dir, "policy.json", `{"version":"1","job_type_priority":{"repo_refactor":["qwen"]},"machine_priority":["mac-studio","mac-mini"]}`)

	var out bytes.Buffer
	err := run([]string{"run", "code-worker", "--task", task, "--registry", registry, "--policy", policy}, &out, &out)
	if err != nil {
		t.Fatalf("run route: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, `"model_id": "qwen"`) || !strings.Contains(s, `"machine_target": "mac-studio"`) {
		t.Fatalf("unexpected route output: %s", s)
	}
}

func TestRouteTieBreakDeterministic(t *testing.T) {
	dir := t.TempDir()
	task := writeFile(t, dir, "task.json", `{"job_type":"reasoning"}`)
	registry := writeFile(t, dir, "registry.json", `{
  "version":"1",
  "machines":[{"id":"mac-studio","ram_gb":64,"role":"model-host"}],
  "models":[
    {"id":"m1","family":"x","quant":"Q4_K_M","memory_gb_estimate":9.0,"capabilities":["reasoning"],"machine_allowlist":["mac-studio"],"priority":1,"status":"active"},
    {"id":"m2","family":"x","quant":"Q4_K_M","memory_gb_estimate":9.0,"capabilities":["reasoning"],"machine_allowlist":["mac-studio"],"priority":2,"status":"active"}
  ]
}`)
	policy := writeFile(t, dir, "policy.json", `{"version":"1","job_type_priority":{"reasoning":["m2","m1"]},"machine_priority":["mac-studio"]}`)

	var out bytes.Buffer
	err := run([]string{"run", "reason-worker", "--task", task, "--registry", registry, "--policy", policy}, &out, &out)
	if err != nil {
		t.Fatalf("run route: %v", err)
	}
	if !strings.Contains(out.String(), `"model_id": "m2"`) {
		t.Fatalf("expected m2 tie-break winner, got: %s", out.String())
	}
}

func TestInvalidWorkerName(t *testing.T) {
	dir := t.TempDir()
	task := writeFile(t, dir, "task.json", `{"job_type":"repo_refactor"}`)
	registry := writeFile(t, dir, "registry.json", `{"version":"1","machines":[],"models":[{"id":"q","family":"f","quant":"Q4_K_M","memory_gb_estimate":1.0,"capabilities":["repo_refactor"],"machine_allowlist":["mac-studio"],"priority":1,"status":"active"}]}`)
	policy := writeFile(t, dir, "policy.json", `{"version":"1","job_type_priority":{"repo_refactor":["q"]},"machine_priority":["mac-studio"]}`)

	var out bytes.Buffer
	err := run([]string{"run", "coder", "--task", task, "--registry", registry, "--policy", policy}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "invalid worker name") {
		t.Fatalf("expected invalid worker name error, got: %v", err)
	}
}

func TestMissingFiles(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"run", "code-worker", "--task", "/missing/task.json", "--registry", "/missing/registry.json", "--policy", "/missing/policy.json"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "read task") {
		t.Fatalf("expected missing file error, got: %v", err)
	}
}

func TestUnsupportedJobType(t *testing.T) {
	dir := t.TempDir()
	task := writeFile(t, dir, "task.json", `{"job_type":"unknown"}`)
	registry := writeFile(t, dir, "registry.json", `{"version":"1","machines":[{"id":"mac-studio","ram_gb":64,"role":"model-host"}],"models":[{"id":"q","family":"f","quant":"Q4_K_M","memory_gb_estimate":1.0,"capabilities":["repo_refactor"],"machine_allowlist":["mac-studio"],"priority":1,"status":"active"}]}`)
	policy := writeFile(t, dir, "policy.json", `{"version":"1","job_type_priority":{"repo_refactor":["q"]},"machine_priority":["mac-studio"]}`)

	var out bytes.Buffer
	err := run([]string{"run", "code-worker", "--task", task, "--registry", registry, "--policy", policy}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "unsupported job_type") {
		t.Fatalf("expected unsupported job_type error, got: %v", err)
	}
}
