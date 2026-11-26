package recipes

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"strings"
	"unicode"
)

type Limits struct {
	MaxIterations         int `json:"max_iterations"`
	TimeoutSeconds        int `json:"timeout_seconds"`
	CommandTimeoutSeconds int `json:"command_timeout_seconds"`
}
type Config struct {
	Adapter       *Adapter                   `json:"adapter,omitempty"`
	SchemaVersion int                        `json:"schema_version"`
	Recipe        string                     `json:"recipe"`
	Inputs        map[string]json.RawMessage `json:"inputs"`
	Limits        *Limits                    `json:"limits,omitempty"`
}
type Plan struct {
	SchemaVersion  int            `json:"schema_version"`
	CatalogSHA256  string         `json:"catalog_sha256"`
	Recipe         string         `json:"recipe"`
	Inputs         map[string]any `json:"inputs"`
	Limits         Limits         `json:"limits"`
	Missing        []string       `json:"missing_inputs"`
	ExecutionReady bool           `json:"execution_ready"`
	Readiness      string         `json:"readiness"`
	Attempt        string         `json:"attempt"`
	Checks         []string       `json:"checks"`
	Protected      []string       `json:"protected"`
	Evidence       []string       `json:"evidence"`
	ExpectedHalt   string         `json:"expected_halt"`
	NextSteps      []string       `json:"next_steps"`
}

func printable(s string) bool {
	return strings.TrimSpace(s) != "" && !strings.ContainsFunc(s, unicode.IsControl)
}
func validateInput(in Input, raw json.RawMessage) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("input %s must be valid JSON", in.Name)
	}
	fail := func() (any, error) {
		return nil, fmt.Errorf("input %s requires %s within its declared bounds", in.Name, in.Type)
	}
	switch in.Type {
	case "string", "path":
		s, ok := v.(string)
		if !ok || !printable(s) {
			return fail()
		}
		if in.Type == "path" && (s != "." && !fs.ValidPath(s) || strings.ContainsAny(s, "\\:") || strings.HasPrefix(s, "~")) {
			return fail()
		}
	case "strings":
		list, ok := v.([]any)
		if !ok || len(list) == 0 {
			return fail()
		}
		for _, item := range list {
			s, ok := item.(string)
			if !ok || !printable(s) {
				return fail()
			}
		}
	case "integer", "number":
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return fail()
		}
		if in.Type == "integer" && (math.Trunc(n) != n || math.Abs(n) > 9007199254740991) {
			return fail()
		}
		if in.Minimum != nil && n < *in.Minimum || in.Maximum != nil && n > *in.Maximum {
			return fail()
		}
	default:
		return fail()
	}
	return v, nil
}
func MakePlan(r Recipe, cfg Config) (Plan, error) {
	p := Plan{SchemaVersion: 1, CatalogSHA256: Digest(), Recipe: r.ID, Inputs: map[string]any{}, Limits: Limits{5, 600, 120}, Missing: []string{}, Readiness: "adapter_required", Attempt: r.Attempt, Checks: r.Checks, Protected: r.Protect, Evidence: r.Evidence, ExpectedHalt: r.ExpectedHalt,
		NextSteps: []string{"Bind a one-attempt builder and independent verifier to this recipe's acceptance contract.", "Use the existing workflow schema for argv hooks, mandatory usage collection and optional candidates/score.", "Pin checks, model profiles and authorized budgets before starting a bounded loopexec run; preserve reservations across retries.", "A plan validates inputs only; it runs no hooks, checks, models or tools."}}
	if cfg.Adapter != nil && r.ID == "creative-workflow" && cfg.Adapter.Candidate == nil {
		return p, fmt.Errorf("creative workflow requires adapter.candidate to retain and restore reviewed candidates")
	}
	if err := ValidateAdapter(cfg.Adapter); err != nil {
		return p, err
	}
	if cfg.SchemaVersion != 0 && cfg.SchemaVersion != 1 {
		return p, fmt.Errorf("unsupported recipe config schema")
	}
	if cfg.Recipe != "" && cfg.Recipe != r.ID {
		return p, fmt.Errorf("configuration recipe does not match requested recipe")
	}
	if cfg.Limits != nil {
		p.Limits = *cfg.Limits
	}
	if p.Limits.MaxIterations < 1 || p.Limits.MaxIterations > 1000 || p.Limits.TimeoutSeconds < 1 || p.Limits.TimeoutSeconds > 86400 || p.Limits.CommandTimeoutSeconds < 1 || p.Limits.CommandTimeoutSeconds > p.Limits.TimeoutSeconds {
		return p, fmt.Errorf("limits require 1..1000 attempts and 1..86400 total seconds, with command timeout within total timeout")
	}
	known := map[string]bool{}
	for _, in := range r.Inputs {
		known[in.Name] = true
		raw, ok := cfg.Inputs[in.Name]
		if !ok {
			raw = in.Default
		}
		if raw == nil {
			if in.Required {
				p.Missing = append(p.Missing, in.Name)
			}
			continue
		}
		v, err := validateInput(in, raw)
		if err != nil {
			return p, err
		}
		p.Inputs[in.Name] = v
	}
	for name := range cfg.Inputs {
		if !known[name] {
			return p, fmt.Errorf("unknown recipe input %q", name)
		}
	}
	if r.ID == "performance" {
		if d, ok := p.Inputs["direction"]; ok && d != "minimize" && d != "maximize" {
			return p, fmt.Errorf("direction must be minimize or maximize")
		}
	}
	if source, ok := p.Inputs["source"].(string); ok {
		if output, ok := p.Inputs["output"].(string); ok && output == source {
			return p, fmt.Errorf("source and output must be separate paths")
		}
	}
	if extras, ok := p.Inputs["additional_constraints"].([]any); ok {
		p.Checks = append(append([]string{}, p.Checks...), "Additional user acceptance constraints:")
		for _, extra := range extras {
			p.Checks = append(p.Checks, extra.(string))
		}
	}
	if cfg.Adapter != nil {
		if extras, ok := p.Inputs["additional_constraints"].([]any); ok && len(extras) > len(cfg.Adapter.Checks) {
			return p, fmt.Errorf("additional constraints require explicitly bound extra checks")
		}
		p.ExecutionReady = true
		p.Readiness = "configured"
		p.NextSteps = []string{"Run this configured adapter through recipes run; readiness does not prove tool availability or successful execution."}
	}
	if len(p.Missing) > 0 {
		p.ExecutionReady = false
		p.Readiness = "needs_inputs"
	}
	return p, nil
}

type Result struct {
	Verification  *Verification `json:"verification,omitempty"`
	CatalogSHA256 string        `json:"catalog_sha256"`
	Entries       []Entry       `json:"entries,omitempty"`
	Suggestions   []Suggestion  `json:"suggestions,omitempty"`
	Definition    *Recipe       `json:"definition,omitempty"`
	Plan          *Plan         `json:"plan,omitempty"`
	Export        *ExportResult `json:"export,omitempty"`
	Catalog       *Catalog      `json:"catalog,omitempty"`
}
