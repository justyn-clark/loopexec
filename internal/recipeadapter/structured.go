package recipeadapter

import (
	"context"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// OracleReport is emitted by an independently configured, protected domain tool.
// Artifacts binds the report to every measured candidate byte; checks cannot be
// omitted, and unknown/absent measurements fail closed.
type OracleReport struct {
	SchemaVersion int                `json:"schema_version"`
	Artifacts     map[string]string  `json:"artifacts"`
	Checks        map[string]bool    `json:"checks"`
	Metrics       map[string]float64 `json:"metrics"`
	Identity      string             `json:"identity"`
}

func structured(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"configured independent domain oracle"}}
	if c.Recipe == "creative-workflow" && c.Iteration+1 > int(c.Number("max_reviews")) {
		return m, fmt.Errorf("review allowance exhausted before invoking critic")
	}
	var argv []string
	if err := c.Option("oracle", &argv); err != nil {
		return m, err
	}
	if len(argv) == 0 {
		return m, fmt.Errorf("options.oracle argv required for %s", c.Recipe)
	}
	contextFile := filepath.Join(c.EvidenceDir, "oracle-context.json")
	if err := os.WriteFile(contextFile, mustJSON(c), 0600); err != nil {
		return m, err
	}
	for i, arg := range argv {
		if arg == "{context}" {
			argv[i] = contextFile
		}
	}
	r, err := c.Run(ctx, c.Root, argv...)
	if err != nil {
		return m, err
	}
	if err = os.WriteFile(filepath.Join(c.EvidenceDir, "oracle.stdout"), []byte(r.Stdout), 0600); err != nil {
		return m, err
	}
	if err = os.WriteFile(filepath.Join(c.EvidenceDir, "oracle.stderr"), []byte(r.Stderr), 0600); err != nil {
		return m, err
	}
	if r.ExitCode != 0 {
		return m, fmt.Errorf("domain oracle exited %d; see retained stderr", r.ExitCode)
	}
	var report OracleReport
	if err = workflow.Decode([]byte(r.Stdout), &report); err != nil {
		return m, fmt.Errorf("oracle must emit one JSON report: %w", err)
	}
	if report.SchemaVersion != 1 || report.Identity == "" {
		return m, fmt.Errorf("oracle identity/schema missing")
	}
	manifest, err := Manifest(ctx, c.Root, c.Adapter.Artifacts)
	if err != nil {
		return m, err
	}
	if len(report.Artifacts) != len(manifest.Files) {
		return m, fmt.Errorf("oracle candidate inventory differs")
	}
	for _, f := range manifest.Files {
		if f.SHA256 == "missing" || report.Artifacts[f.Path] != f.SHA256 {
			return m, fmt.Errorf("oracle not bound to current candidate: %s", f.Path)
		}
	}
	for k, v := range report.Metrics {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return m, fmt.Errorf("nonfinite metric")
		}
		m.Metric(k, v)
	}
	required := []string{}
	type threshold struct {
		name   string
		target float64
		max    bool
	}
	limits := []threshold{}
	switch c.Recipe {
	case "asset-optimization":
		for _, v := range c.Strings("views") {
			required = append(required, "view:"+v)
			limits = append(limits, threshold{"similarity:" + v, c.Number("min_similarity"), false})
		}
		required = append(required, "uvs", "materials", "normals", "scale", "import")
		limits = append(limits, threshold{"triangles", c.Number("max_triangles"), true}, threshold{"texture_edge", c.Number("max_texture_edge"), true})
	case "godot-performance":
		required = append(required, "renderer-active", "workload-complete")
		required = append(required, c.Strings("gameplay_assertions")...)
		limits = append(limits, threshold{"frame_p95_ms", c.Number("max_frame_ms"), true}, threshold{"draw_calls", c.Number("max_draw_calls"), true}, threshold{"memory_bytes", c.Number("max_memory_bytes"), true})
	case "prompt-optimization":
		required = []string{"frozen-cases", "held-out", "hard-correctness", "cost-known"}
		limits = append(limits, threshold{"score", c.Number("target_score"), false}, threshold{"latency_ms", c.Number("max_latency_ms"), true}, threshold{"cost_microusd", c.Number("max_cost_microusd"), true})
	case "creative-workflow":
		required = append([]string{"independent-review", "technical", "reviewer-accepts"}, c.Strings("criteria")...)
		limits = append(limits, threshold{"score", c.Number("target_score"), false})
		m.Add("review-budget", c.Iteration+1 <= int(c.Number("max_reviews")), "baseline and every critic invocation count toward pinned review budget")
	case "dependency-remediation":
		required = []string{"lock-consistency", "scanner-complete"}
		for _, v := range c.Strings("severities") {
			limits = append(limits, threshold{"findings:" + v, 0, true})
		}
		for _, v := range c.Strings("manifests") {
			required = append(required, "manifest:"+v)
		}
	}
	for _, id := range required {
		pass, ok := report.Checks[id]
		if !ok {
			return m, fmt.Errorf("oracle omitted required check %s", id)
		}
		m.Add(id, pass, report.Identity)
	}
	requiredSet := map[string]bool{}
	for _, id := range required {
		requiredSet[id] = true
	}
	for id, pass := range report.Checks {
		if !requiredSet[id] {
			m.Add(id, pass, report.Identity)
		}
	}
	for _, t := range limits {
		v, ok := report.Metrics[t.name]
		if !ok || v < 0 {
			return m, fmt.Errorf("oracle omitted/invalid metric %s", t.name)
		}
		pass := v >= t.target
		if t.max {
			pass = v <= t.target
		}
		m.Add("threshold:"+t.name, pass, "configured domain threshold")
	}
	if c.Recipe == "dependency-remediation" {
		argv, e := CommandWords(c.String("test_command"))
		if e != nil {
			return m, e
		}
		dir, e := c.Path(c.String("project"))
		if e != nil {
			return m, e
		}
		if e = commandCheck(ctx, c, &m, "regression-tests", argv, dir); e != nil {
			return m, e
		}
	}
	if len(m.Checks) == 0 || strings.TrimSpace(report.Identity) == "" {
		return m, fmt.Errorf("empty domain verdict")
	}
	return m, nil
}
