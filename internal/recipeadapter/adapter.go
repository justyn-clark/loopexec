// Package recipeadapter implements domain measurement, never an outer loop.
package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/recipes"
	"github.com/justyn-clark/loopexec/internal/subprocess"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Check struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}
type Measurement struct {
	Checks  []Check            `json:"checks"`
	Metrics map[string]float64 `json:"metrics"`
	Files   []string           `json:"files,omitempty"`
	Tools   []string           `json:"tools,omitempty"`
}
type Context struct {
	Root           string          `json:"root"`
	EvidenceDir    string          `json:"evidence_dir"`
	Recipe         string          `json:"recipe"`
	Inputs         map[string]any  `json:"inputs"`
	Adapter        recipes.Adapter `json:"adapter"`
	Iteration      int             `json:"iteration"`
	TimeoutSeconds int             `json:"timeout_seconds"`
}

func (c Context) String(key string) string  { v, _ := c.Inputs[key].(string); return v }
func (c Context) Number(key string) float64 { v, _ := c.Inputs[key].(float64); return v }
func (c Context) Strings(key string) []string {
	var out []string
	for _, x := range func() []any { v, _ := c.Inputs[key].([]any); return v }() {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
func (c Context) Tool(name string) string {
	if p := c.Adapter.Tools[name]; p != "" {
		return p
	}
	return name
}
func (c Context) Option(key string, v any) error {
	b, ok := c.Adapter.Options[key]
	if !ok {
		return nil
	}
	return workflow.Decode(b, v)
}
func (c Context) Path(rel string) (string, error) {
	if rel == "." {
		return c.Root, nil
	}
	return workflow.SafePath(c.Root, filepath.FromSlash(rel))
}
func (c Context) Run(ctx context.Context, dir string, argv ...string) (subprocess.Result, error) {
	r := subprocess.Run(ctx, subprocess.Options{Dir: dir, Argv: argv, Timeout: time.Duration(c.TimeoutSeconds) * time.Second, Grace: 200 * time.Millisecond, JoinGroup: true})
	if r.Cause != "" || r.Truncated {
		return r, fmt.Errorf("tool execution failed or exceeded output bound: %s", r.Cause)
	}
	return r, nil
}
func (m *Measurement) Add(id string, passed bool, detail string) {
	m.Checks = append(m.Checks, Check{id, passed, detail})
}
func (m Measurement) Failures() []string {
	out := []string{}
	for _, v := range m.Checks {
		if !v.Passed {
			out = append(out, v.ID)
		}
	}
	sort.Strings(out)
	return out
}
func (m Measurement) Passed() bool { return len(m.Checks) > 0 && len(m.Failures()) == 0 }
func (m *Measurement) Metric(name string, n float64) {
	if m.Metrics == nil {
		m.Metrics = map[string]float64{}
	}
	m.Metrics[name] = n
}
func Manifest(ctx context.Context, root string, paths []string) (workflow.Manifest, error) {
	files := map[string]workflow.Artifact{}
	for _, rel := range paths {
		path, err := workflow.SafePath(root, rel)
		if err != nil {
			return workflow.Manifest{}, err
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			files[rel] = workflow.Artifact{Path: rel, SHA256: "missing"}
			continue
		}
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if len(files) > 10000 {
				return fmt.Errorf("artifact inventory too large")
			}
			name, e := filepath.Rel(root, p)
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("artifact symlink rejected")
			}
			if d.IsDir() {
				return nil
			}
			hash, size, e := workflow.FileHash(ctx, p)
			if e != nil {
				return e
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			files[filepath.ToSlash(name)] = workflow.Artifact{Path: filepath.ToSlash(name), SHA256: hash, Size: size, Mode: uint32(info.Mode().Perm())}
			return nil
		})
		if err != nil {
			return workflow.Manifest{}, err
		}
	}
	result := workflow.Manifest{Files: []workflow.Artifact{}}
	for _, file := range files {
		result.Files = append(result.Files, file)
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, nil
}
func Evaluate(ctx context.Context, c Context) (Measurement, error) {
	if err := os.MkdirAll(c.EvidenceDir, 0700); err != nil {
		return Measurement{}, err
	}
	switch c.Recipe {
	case "asset-optimization", "godot-performance", "prompt-optimization", "creative-workflow", "dependency-remediation":
		return structured(ctx, c)
	case "docker-reduction":
		return docker(ctx, c)
	case "infrastructure-drift":
		return infrastructure(ctx, c, false)
	case "metric-gaming", "oscillation", "regression", "cost-budget":
		return governance(ctx, c)
	case "code-repair", "lint-types", "build-system", "configuration-repair":
		return commandRecipe(ctx, c)
	case "api-contract":
		return openapi(ctx, c)
	case "media-optimization":
		return media(ctx, c)
	case "batch-transformation":
		return batch(ctx, c)
	case "data-cleaning":
		return cleaning(ctx, c)
	case "etl-reconciliation":
		return reconciliation(ctx, c)
	case "sql-optimization":
		return sqlOptimization(ctx, c)
	case "migration-safety":
		return migration(ctx, c)
	case "ocr-extraction":
		return ocr(ctx, c)
	case "performance":
		return Performance(ctx, c)
	case "documentation-drift", "release-readiness":
		return documentation(ctx, c)
	case "audio-mastering", "music-mix":
		return audio(ctx, c)
	case "frontend-visual", "responsive-ui", "accessibility", "static-site", "seo-metadata":
		return browser(ctx, c)
	default:
		return Measurement{}, fmt.Errorf("domain adapter not implemented: %s", c.Recipe)
	}
}
func Work(ctx context.Context, c Context, contextFile string) error {
	if len(c.Adapter.Work) == 0 {
		if c.Recipe == "infrastructure-drift" {
			_, err := infrastructure(ctx, c, true)
			return err
		}
		if c.Recipe == "media-optimization" {
			return CompressMedia(ctx, c)
		}
		if c.Recipe == "audio-mastering" {
			return NormalizeAudio(ctx, c)
		}
		return nil
	}
	argv := append([]string{}, c.Adapter.Work...)
	for i, arg := range argv {
		switch arg {
		case "{context}":
			argv[i] = contextFile
		case "{iteration}":
			argv[i] = fmt.Sprint(c.Iteration)
		case "{root}":
			argv[i] = c.Root
		default:
			if strings.HasPrefix(arg, "{input.") && strings.HasSuffix(arg, "}") {
				key := strings.TrimSuffix(strings.TrimPrefix(arg, "{input."), "}")
				value, ok := c.Inputs[key]
				if !ok {
					return fmt.Errorf("unknown argv input")
				}
				if s, ok := value.(string); ok {
					argv[i] = s
				} else {
					b, _ := json.Marshal(value)
					argv[i] = string(b)
				}
			}
		}
	}
	r, err := c.Run(ctx, c.Root, argv...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, "work.stdout"), []byte(r.Stdout), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, "work.stderr"), []byte(r.Stderr), 0600); err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return fmt.Errorf("work command exited %d", r.ExitCode)
	}
	return nil
}
