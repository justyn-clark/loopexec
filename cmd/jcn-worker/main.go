package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	workerVersion       = "0.2.0"
	defaultLMStudioBase = "http://localhost:1234"
	defaultPolicyPath   = "docs/jcn-agent-stack/router-policy.example.json"
	defaultRegistryPath = "docs/jcn-agent-stack/model-registry.example.json"
)

type workerTask struct {
	JobType           string `json:"job_type"`
	RepoSize          string `json:"repo_size"`
	LatencyBudget     string `json:"latency_budget"`
	ContextNeed       string `json:"context_need"`
	ToolCallingNeeded bool   `json:"tool_calling_needed"`
	Prompt            string `json:"prompt,omitempty"`
	Model             string `json:"model,omitempty"`
	RouterPolicyPath  string `json:"router_policy_path,omitempty"`
	ModelRegistryPath string `json:"model_registry_path,omitempty"`
}

type machine struct {
	ID    string `json:"id"`
	RAMGB int    `json:"ram_gb"`
	Role  string `json:"role"`
}

type model struct {
	ID               string   `json:"id"`
	Family           string   `json:"family"`
	Quant            string   `json:"quant"`
	MemoryGBEstimate float64  `json:"memory_gb_estimate"`
	Capabilities     []string `json:"capabilities"`
	MachineAllowlist []string `json:"machine_allowlist"`
	Priority         int      `json:"priority"`
	Status           string   `json:"status"`
}

type registry struct {
	Version  string    `json:"version"`
	Machines []machine `json:"machines"`
	Models   []model   `json:"models"`
}

type policy struct {
	Version         string              `json:"version"`
	JobTypePriority map[string][]string `json:"job_type_priority"`
	MachinePriority []string            `json:"machine_priority"`
}

type routeResult struct {
	ModelID       string
	MachineTarget string
	ReasonCodes   []string
}

type runRecord struct {
	RunID                 string `json:"runId"`
	StartedUTC            string `json:"started_utc"`
	EndedUTC              string `json:"ended_utc"`
	TaskPath              string `json:"task_path"`
	TaskSHA256            string `json:"task_sha256"`
	RouterPolicyPath      string `json:"router_policy_path"`
	RouterPolicySHA256    string `json:"router_policy_sha256"`
	ModelRegistryPath     string `json:"model_registry_path"`
	ModelRegistrySHA256   string `json:"model_registry_sha256"`
	SelectedModelID       string `json:"selected_model_id"`
	SelectedMachineTarget string `json:"selected_machine_target"`
	LMStudioBaseURL       string `json:"lmstudio_base_url"`
	PromptSHA256          string `json:"prompt_sha256"`
	ResponseSHA256        string `json:"response_sha256"`
	Status                string `json:"status"`
	Error                 string `json:"error,omitempty"`
}

type lmStudioRequest struct {
	Model       string            `json:"model"`
	Messages    []lmStudioMessage `json:"messages"`
	Temperature float64           `json:"temperature"`
}

type lmStudioMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type lmStudioResponse struct {
	Choices []struct {
		Message lmStudioMessage `json:"message"`
	} `json:"choices"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer, _ io.Writer) error {
	if len(args) == 0 {
		printUsage(out)
		return nil
	}

	switch args[0] {
	case "version":
		fmt.Fprintf(out, "jcn-worker %s\n", workerVersion)
		return nil
	case "list":
		workers := []string{"code-worker", "docs-worker", "infra-worker", "mindrail-worker", "reaper-worker"}
		sort.Strings(workers)
		for _, w := range workers {
			fmt.Fprintln(out, w)
		}
		return nil
	case "status":
		fmt.Fprintln(out, "status: ok")
		fmt.Fprintln(out, "mode: local")
		return nil
	case "run":
		return runCommand(args[1:], out)
	default:
		printUsage(out)
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  jcn-worker version")
	fmt.Fprintln(out, "  jcn-worker list")
	fmt.Fprintln(out, "  jcn-worker status")
	fmt.Fprintln(out, "  jcn-worker run <taskPath> [--policy <path>] [--registry <path>]")
}

func runCommand(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing task path")
	}
	taskPath := args[0]

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	policyOverride := fs.String("policy", "", "router policy json path")
	registryOverride := fs.String("registry", "", "model registry json path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	started := time.Now().UTC()
	runID := newRunID(started)
	record := runRecord{
		RunID:      runID,
		StartedUTC: started.Format(time.RFC3339),
		Status:     "FAIL",
	}

	taskAbs, err := filepath.Abs(taskPath)
	if err != nil {
		return err
	}
	record.TaskPath = taskAbs
	taskBytes, task, err := readTask(taskPath)
	if err != nil {
		record.Error = err.Error()
		writeRunArtifacts(record, "", "")
		return err
	}
	record.TaskSHA256 = hashBytes(taskBytes)

	policyPath := firstNonEmpty(*policyOverride, task.RouterPolicyPath, defaultPolicyPath)
	policyAbs, err := filepath.Abs(policyPath)
	if err != nil {
		record.Error = err.Error()
		writeRunArtifacts(record, "", "")
		return err
	}
	record.RouterPolicyPath = policyAbs
	policyBytes, pol, err := readPolicy(policyPath)
	if err != nil {
		record.Error = err.Error()
		writeRunArtifacts(record, "", "")
		return err
	}
	record.RouterPolicySHA256 = hashBytes(policyBytes)

	registryPath := firstNonEmpty(*registryOverride, task.ModelRegistryPath, defaultRegistryPath)
	registryAbs, err := filepath.Abs(registryPath)
	if err != nil {
		record.Error = err.Error()
		writeRunArtifacts(record, "", "")
		return err
	}
	record.ModelRegistryPath = registryAbs
	registryBytes, reg, err := readRegistry(registryPath)
	if err != nil {
		record.Error = err.Error()
		writeRunArtifacts(record, "", "")
		return err
	}
	record.ModelRegistrySHA256 = hashBytes(registryBytes)

	routeRes, err := route(task, reg, pol)
	if err != nil {
		record.Error = err.Error()
		writeRunArtifacts(record, "", "")
		return err
	}

	selectedModel := routeRes.ModelID
	if strings.TrimSpace(task.Model) != "" {
		selectedModel = strings.TrimSpace(task.Model)
	}
	record.SelectedModelID = selectedModel
	record.SelectedMachineTarget = routeRes.MachineTarget

	systemPrompt := "You are a deterministic coding assistant. Reply in one short sentence."
	userPrompt := buildUserPrompt(task)
	promptBytes := []byte(systemPrompt + "\n" + userPrompt)
	record.PromptSHA256 = hashBytes(promptBytes)

	baseURL := strings.TrimSuffix(firstNonEmpty(os.Getenv("JCN_LMSTUDIO_BASE_URL"), defaultLMStudioBase), "/")
	record.LMStudioBaseURL = baseURL

	response, err := chatCompletion(baseURL, selectedModel, systemPrompt, userPrompt)
	if err != nil {
		record.Error = err.Error()
		record.EndedUTC = time.Now().UTC().Format(time.RFC3339)
		writeRunArtifacts(record, string(promptBytes), "")
		return err
	}
	record.ResponseSHA256 = hashBytes([]byte(response))
	record.Status = "OK"
	record.EndedUTC = time.Now().UTC().Format(time.RFC3339)

	if err := writeRunArtifacts(record, string(promptBytes), response); err != nil {
		return err
	}

	fmt.Fprintf(out, "selected_model_id: %s\n", record.SelectedModelID)
	fmt.Fprintf(out, "selected_machine_target: %s\n", record.SelectedMachineTarget)
	fmt.Fprintln(out, response)
	return nil
}

func readTask(path string) ([]byte, workerTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, workerTask{}, fmt.Errorf("read task: %w", err)
	}
	var t workerTask
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, workerTask{}, fmt.Errorf("read task: %w", err)
	}
	if strings.TrimSpace(t.JobType) == "" {
		return nil, workerTask{}, errors.New("read task: job_type is required")
	}
	return data, t, nil
}

func readPolicy(path string) ([]byte, policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, policy{}, fmt.Errorf("read policy: %w", err)
	}
	var p policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, policy{}, fmt.Errorf("read policy: %w", err)
	}
	if len(p.JobTypePriority) == 0 || len(p.MachinePriority) == 0 {
		return nil, policy{}, errors.New("read policy: job_type_priority and machine_priority are required")
	}
	return data, p, nil
}

func readRegistry(path string) ([]byte, registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, registry{}, fmt.Errorf("read registry: %w", err)
	}
	var r registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, registry{}, fmt.Errorf("read registry: %w", err)
	}
	if len(r.Models) == 0 {
		return nil, registry{}, errors.New("read registry: models is required")
	}
	return data, r, nil
}

func route(task workerTask, reg registry, pol policy) (routeResult, error) {
	candidateIDs, ok := pol.JobTypePriority[task.JobType]
	if !ok || len(candidateIDs) == 0 {
		return routeResult{}, fmt.Errorf("unsupported job_type: %s", task.JobType)
	}

	modelByID := map[string]model{}
	for _, m := range reg.Models {
		modelByID[m.ID] = m
	}

	for _, modelID := range candidateIDs {
		m, ok := modelByID[modelID]
		if !ok || m.Status == "disabled" {
			continue
		}
		if !hasCapability(m.Capabilities, task.JobType) {
			continue
		}
		machineID := pickMachine(m.MachineAllowlist, pol.MachinePriority)
		if machineID == "" {
			continue
		}
		return routeResult{ModelID: modelID, MachineTarget: machineID, ReasonCodes: []string{"job_type_match", "capability_match", "machine_priority_match"}}, nil
	}

	return routeResult{}, errors.New("no route available for task")
}

func hasCapability(caps []string, v string) bool {
	for _, c := range caps {
		if c == v {
			return true
		}
	}
	return false
}

func pickMachine(allowlist, preferred []string) string {
	allowed := map[string]struct{}{}
	for _, m := range allowlist {
		allowed[m] = struct{}{}
	}
	for _, p := range preferred {
		if _, ok := allowed[p]; ok {
			return p
		}
	}
	return ""
}

func buildUserPrompt(task workerTask) string {
	if strings.TrimSpace(task.Prompt) != "" {
		return strings.TrimSpace(task.Prompt)
	}
	return fmt.Sprintf("job_type=%s; repo_size=%s; latency_budget=%s; context_need=%s; tool_calling_needed=%t", task.JobType, task.RepoSize, task.LatencyBudget, task.ContextNeed, task.ToolCallingNeeded)
}

func chatCompletion(baseURL, modelID, systemPrompt, userPrompt string) (string, error) {
	body := lmStudioRequest{
		Model: modelID,
		Messages: []lmStudioMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("lmstudio error status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed lmStudioResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("lmstudio response has no choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

func writeRunArtifacts(record runRecord, prompt, response string) error {
	runsDir := filepath.Join("docs", "jcn-agent-stack", "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return err
	}

	jsonPath := filepath.Join(runsDir, record.RunID+".json")
	txtPath := filepath.Join(runsDir, record.RunID+".txt")

	jsonBytes, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
		return err
	}

	transcript := "Prompt:\n" + prompt + "\n\nResponse:\n" + response + "\n"
	if err := os.WriteFile(txtPath, []byte(transcript), 0o644); err != nil {
		return err
	}

	return nil
}

func hashBytes(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func newRunID(start time.Time) string {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return start.Format("2006-01-02T15-04-05Z") + "-0000"
	}
	return start.Format("2006-01-02T15-04-05Z") + "-" + hex.EncodeToString(b)
}
