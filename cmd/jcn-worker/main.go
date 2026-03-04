package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
)

const workerVersion = "0.1.0"

var workerNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*-worker$`)

type workerTask struct {
	JobType           string `json:"job_type"`
	RepoSize          string `json:"repo_size"`
	LatencyBudget     string `json:"latency_budget"`
	ContextNeed       string `json:"context_need"`
	ToolCallingNeeded bool   `json:"tool_calling_needed"`
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
	Worker        string   `json:"worker"`
	ModelID       string   `json:"model_id"`
	MachineTarget string   `json:"machine_target"`
	ReasonCodes   []string `json:"reason_codes"`
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
		workers := []string{"code-worker", "docs-worker", "mindrail-worker", "infra-worker", "reaper-worker"}
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
	fmt.Fprintln(out, "  jcn-worker run <domain-worker> --task <path> --registry <path> --policy <path>")
}

func runCommand(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing worker name")
	}
	workerName := args[0]
	if !workerNamePattern.MatchString(workerName) {
		return fmt.Errorf("invalid worker name: %s", workerName)
	}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(out)
	taskPath := fs.String("task", "", "task json path")
	registryPath := fs.String("registry", "", "model registry path")
	policyPath := fs.String("policy", "", "router policy path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *taskPath == "" || *registryPath == "" || *policyPath == "" {
		return errors.New("--task, --registry, and --policy are required")
	}

	task, err := readTask(*taskPath)
	if err != nil {
		return err
	}
	reg, err := readRegistry(*registryPath)
	if err != nil {
		return err
	}
	pol, err := readPolicy(*policyPath)
	if err != nil {
		return err
	}

	result, err := route(workerName, task, reg, pol)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func readTask(path string) (workerTask, error) {
	var t workerTask
	if err := readJSON(path, &t); err != nil {
		return t, fmt.Errorf("read task: %w", err)
	}
	if t.JobType == "" {
		return t, errors.New("read task: job_type is required")
	}
	return t, nil
}

func readRegistry(path string) (registry, error) {
	var r registry
	if err := readJSON(path, &r); err != nil {
		return r, fmt.Errorf("read registry: %w", err)
	}
	if len(r.Models) == 0 {
		return r, errors.New("read registry: models is required")
	}
	return r, nil
}

func readPolicy(path string) (policy, error) {
	var p policy
	if err := readJSON(path, &p); err != nil {
		return p, fmt.Errorf("read policy: %w", err)
	}
	if len(p.JobTypePriority) == 0 || len(p.MachinePriority) == 0 {
		return p, errors.New("read policy: job_type_priority and machine_priority are required")
	}
	return p, nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}

func route(workerName string, task workerTask, reg registry, pol policy) (routeResult, error) {
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
		return routeResult{
			Worker:        workerName,
			ModelID:       modelID,
			MachineTarget: machineID,
			ReasonCodes:   []string{"job_type_match", "capability_match", "machine_priority_match"},
		}, nil
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
