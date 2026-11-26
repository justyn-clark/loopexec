package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func recipeCLI(t *testing.T, args ...string) (response, error) {
	t.Helper()
	jsonOutput = false
	defer func() { jsonOutput = false }()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, "--json"))
	err := cmd.Execute()
	var result response
	if e := json.Unmarshal(out.Bytes(), &result); e != nil {
		t.Fatalf("not one JSON response: %s %v", out.String(), e)
	}
	return result, err
}
func TestRecipesCLIPlansAndRejectsInvalidConfig(t *testing.T) {
	result, err := recipeCLI(t, "recipes", "list")
	if err != nil || len(result.Recipes.Entries) != 33 {
		t.Fatalf("list %+v %v", result, err)
	}
	result, err = recipeCLI(t, "recipes", "plan", "audio-mastering", "--set", "source=audio/source.wav", "--set", "output=audio/master.wav", "--set", "target_lufs=-14", "--set", "lufs_tolerance=0.5", "--set", "max_true_peak_dbtp=-1", "--set", "min_dynamic_range=6")
	if err != nil || result.Recipes.Plan.Readiness != "adapter_required" || result.Recipes.Plan.ExecutionReady {
		t.Fatalf("plan %+v %v", result, err)
	}
	for _, raw := range []string{`{"source":"a","source":"b"}`, `{"unknown":1}`, `null`} {
		result, err = recipeCLI(t, "recipes", "plan", "audio-mastering", "--inputs-json", raw)
		if exitCode(err) != 30 || result.Status != "error" {
			t.Fatalf("accepted %s: %v", raw, err)
		}
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	os.WriteFile(config, []byte(`{"schema_version":1,"recipe":"code-repair","inputs":{},"unknown":1}`), 0600)
	if _, err = recipeCLI(t, "recipes", "plan", "code-repair", "--config", config); exitCode(err) != 30 {
		t.Fatal("accepted unknown config field")
	}
	config = filepath.Join(dir, "valid.json")
	os.WriteFile(config, []byte(`{"schema_version":1,"recipe":"code-repair","inputs":{"project":"fixture","test_command":"go test ./...","allowed_paths":["sum.go"]},"limits":{"max_iterations":2,"timeout_seconds":60,"command_timeout_seconds":30}}`), 0600)
	result, err = recipeCLI(t, "recipes", "plan", "code-repair", "--config", config, "--set", "test_command=go test .")
	if err != nil || result.Recipes.Plan.Inputs["test_command"] != "go test ." || result.Recipes.Plan.Limits.MaxIterations != 2 {
		t.Fatalf("override %+v %v", result, err)
	}
}
func TestRecipeDemoRefusesExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "sum.go")
	os.WriteFile(marker, []byte("owner content"), 0600)
	if err := prepareRecipeDemo(dir); err == nil {
		t.Fatal("accepted existing fixture")
	}
	body, _ := os.ReadFile(marker)
	if string(body) != "owner content" {
		t.Fatal("clobbered owner file")
	}
}

func TestRecipeFixtureStartsWithRealFailingTest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fixture")
	if e := prepareRecipeDemo(dir); e != nil {
		t.Fatal(e)
	}
	command := exec.Command("go", "test", ".")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOWORK=off", "GOPROXY=off", "GOFLAGS=")
	b, e := command.CombinedOutput()
	if e == nil || !bytes.Contains(b, []byte("want 5")) {
		t.Fatalf("missing actual red test: %s %v", b, e)
	}
}
