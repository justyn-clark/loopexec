package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/justyn-clark/loopexec/internal/recipes"
	"github.com/justyn-clark/loopexec/internal/workflow"
)

// CommandWords parses literal argv, without expansion, redirection or a shell.
func CommandWords(s string) ([]string, error) {
	var out []string
	var b strings.Builder
	quote := rune(0)
	escape := false
	started := false
	flush := func() {
		if started {
			out = append(out, b.String())
			b.Reset()
			started = false
		}
	}
	for _, r := range s {
		if escape {
			b.WriteRune(r)
			escape = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escape = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			started = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if r == ' ' || r == '\t' {
			flush()
			continue
		}
		if strings.ContainsRune("|;&<>", r) {
			return nil, fmt.Errorf("shell syntax requires an explicitly configured shell argv")
		}
		b.WriteRune(r)
		started = true
	}
	if quote != 0 || escape {
		return nil, fmt.Errorf("unterminated command quoting")
	}
	flush()
	if len(out) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return out, nil
}
func commandCheck(ctx context.Context, c Context, m *Measurement, id string, argv []string, dir string) error {
	if len(argv) == 0 {
		return fmt.Errorf("missing command for %s", id)
	}
	result, err := c.Run(ctx, dir, argv...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, id+".stdout"), []byte(result.Stdout), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, id+".stderr"), []byte(result.Stderr), 0600); err != nil {
		return err
	}
	m.Add(id, result.ExitCode == 0, fmt.Sprintf("independent command exit %d", result.ExitCode))
	return nil
}
func commandRecipe(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"operator-selected verifier"}}
	dir := c.Root
	if project := c.String("project"); project != "" {
		var err error
		dir, err = c.Path(project)
		if err != nil {
			return m, err
		}
	}
	if c.Recipe == "build-system" {
		var mapping map[string]string
		if err := c.Option("package_paths", &mapping); err != nil {
			return m, err
		}
		for _, name := range c.Strings("packages") {
			path := name
			if mapped := mapping[name]; mapped != "" {
				path = mapped
			}
			if path != "." && !workflow.CleanRelative(path) {
				return m, fmt.Errorf("package_paths must resolve within project")
			}
			full := filepath.Join(dir, path)
			if _, err := os.Stat(full); err != nil {
				return m, fmt.Errorf("declared build package absent: %s", name)
			}
			m.Add("package:"+name, true, "declared package exists within build project")
		}
	}
	var fields []string
	switch c.Recipe {
	case "code-repair":
		fields = []string{"test_command"}
	case "lint-types":
		fields = []string{"check_command"}
	case "build-system":
		fields = []string{"build_command", "test_command", "boundary_command"}
	case "configuration-repair":
		fields = []string{"validator", "integration_command"}
	}
	for _, field := range fields {
		argv, err := CommandWords(c.String(field))
		if err != nil {
			return m, err
		}
		if err := commandCheck(ctx, c, &m, field, argv, dir); err != nil {
			return m, err
		}
	}
	if len(m.Checks) == 0 {
		return m, fmt.Errorf("no independent verifier selected")
	}
	return m, nil
}
func ExtraChecks(ctx context.Context, c Context, m *Measurement) error {
	for _, probe := range c.Adapter.Checks {
		if err := commandCheck(ctx, c, m, "extra-"+probe.ID, probe.Argv, c.Root); err != nil {
			return err
		}
	}
	return nil
}
func Performance(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"fixed-workload benchmark"}}
	var argv []string
	if err := c.Option("benchmark", &argv); err != nil {
		return m, err
	}
	if len(argv) == 0 {
		return m, fmt.Errorf("performance requires options.benchmark argv")
	}
	samples := int(c.Number("samples"))
	if samples < 3 || samples > 100 {
		return m, fmt.Errorf("samples must be 3..100")
	}
	var metricPath string
	if err := c.Option("metric_path", &metricPath); err != nil {
		return m, err
	}
	values := []float64{}
	for i := 0; i < samples+1; i++ {
		started := time.Now()
		r, err := c.Run(ctx, c.Root, argv...)
		if err != nil {
			return m, err
		}
		elapsed := float64(time.Since(started).Microseconds()) / 1000
		if r.ExitCode != 0 {
			return m, fmt.Errorf("benchmark command failed")
		}
		if metricPath != "" {
			var fields map[string]float64
			if err = json.Unmarshal([]byte(r.Stdout), &fields); err != nil {
				return m, err
			}
			value, ok := fields[metricPath]
			if !ok {
				return m, fmt.Errorf("missing benchmark metric")
			}
			elapsed = value
		}
		if i > 0 {
			values = append(values, elapsed)
		}
	}
	sort.Float64s(values)
	value := values[len(values)/2]
	m.Metric(c.String("metric"), value)
	pass := value <= c.Number("target")
	if c.String("direction") == "maximize" {
		pass = value >= c.Number("target")
	}
	m.Add("performance-target", pass, "median of pinned samples after one warmup")
	argv, err := CommandWords(c.String("correctness_command"))
	if err != nil {
		return m, err
	}
	if err := commandCheck(ctx, c, &m, "correctness", argv, c.Root); err != nil {
		return m, err
	}
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, "samples.json"), mustJSON(values), 0600); err != nil {
		return m, err
	}
	return m, nil
}
func mustJSON(v any) []byte { b, _ := json.MarshalIndent(v, "", "  "); return append(b, '\n') }

type versionFile struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
}

func documentation(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"source/version/documentation verification"}}
	var command []string
	if err := c.Option("version_command", &command); err != nil {
		return m, err
	}
	if len(command) == 0 {
		return m, fmt.Errorf("version_command required")
	}
	r, err := c.Run(ctx, c.Root, command...)
	if err != nil {
		return m, err
	}
	m.Add("version-identity", r.ExitCode == 0 && strings.Contains(strings.TrimSpace(r.Stdout), c.String("version")), "observed executable version")
	if c.Recipe == "documentation-drift" {
		argv, err := CommandWords(c.String("contract_command"))
		if err != nil {
			return m, err
		}
		if err = commandCheck(ctx, c, &m, "generated-contract", argv, c.Root); err != nil {
			return m, err
		}
	}
	var files []string
	if c.Recipe == "documentation-drift" {
		files = c.Strings("docs")
	} else {
		files = c.Strings("artifacts")
		for _, path := range files {
			full, err := c.Path(path)
			if err != nil {
				return m, err
			}
			info, err := os.Stat(full)
			m.Add("artifact:"+path, err == nil && info.Mode().IsRegular(), "required release artifact")
		}
		var changelog string
		if err := c.Option("changelog", &changelog); err != nil {
			return m, err
		}
		if changelog == "" {
			return m, fmt.Errorf("release changelog path required")
		}
		full, err := c.Path(changelog)
		if err != nil {
			return m, err
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return m, err
		}
		m.Add("changelog-version", strings.Contains(string(b), c.String("version")), "version listed in canonical changelog")
		var checksumFile string
		if err := c.Option("checksums", &checksumFile); err != nil {
			return m, err
		}
		if checksumFile == "" {
			return m, fmt.Errorf("checksums file required")
		}
		full, err = c.Path(checksumFile)
		if err != nil {
			return m, err
		}
		b, err = os.ReadFile(full)
		if err != nil {
			return m, err
		}
		checksums := map[string]string{}
		for _, line := range strings.Split(string(b), "\n") {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				checksums[strings.TrimPrefix(parts[1], "*")] = parts[0]
			}
		}
		for _, path := range files {
			full, err := c.Path(path)
			if err != nil {
				return m, err
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return m, err
			}
			m.Add("checksum:"+path, checksums[path] == hashBytes(data), "checksum matches current artifact")
		}
		var checks []recipes.Probe
		if err := c.Option("release_checks", &checks); err != nil {
			return m, err
		}
		if len(checks) == 0 {
			return m, fmt.Errorf("release_checks require tests/generated gates")
		}
		bound := map[string]bool{}
		for _, p := range checks {
			bound[p.ID] = true
		}
		for _, id := range c.Strings("gates") {
			if !bound[id] {
				return m, fmt.Errorf("release gate %s has no independent check", id)
			}
		}
		for _, probe := range checks {
			if err := commandCheck(ctx, c, &m, "release-"+probe.ID, probe.Argv, c.Root); err != nil {
				return m, err
			}
		}
	}
	for _, path := range files {
		if c.Recipe != "documentation-drift" {
			break
		}
		full, err := c.Path(path)
		if err != nil {
			return m, err
		}
		body, err := os.ReadFile(full)
		if err != nil {
			return m, err
		}
		m.Add("documentation-version:"+path, strings.Contains(string(body), c.String("version")), "canonical guide version")
		for _, match := range regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`).FindAllStringSubmatch(string(body), -1) {
			link := strings.Split(match[1], "#")[0]
			if link == "" || strings.Contains(link, "://") {
				continue
			}
			destination := filepath.Join(filepath.Dir(full), link)
			rel, err := filepath.Rel(c.Root, destination)
			if err != nil {
				return m, err
			}
			if _, err = c.Path(rel); err != nil {
				return m, err
			}
			_, err = os.Stat(destination)
			m.Add("documentation-link:"+path+":"+link, err == nil, "relative link exists")
		}
	}
	var readback struct {
		URL      string `json:"url"`
		Contains string `json:"contains"`
	}
	if err := c.Option("readback", &readback); err != nil {
		return m, err
	}
	origin := c.String("deployment_origin")
	if c.Recipe == "release-readiness" {
		origin = c.String("docs_origin")
	}
	if origin != "local-only" {
		if readback.URL == "" || readback.Contains == "" {
			return m, fmt.Errorf("deployed docs require explicit readback URL and expected marker")
		}
		data, err := readResource(ctx, c, readback.URL)
		if err != nil {
			return m, err
		}
		m.Add("documentation-readback", strings.Contains(string(data), readback.Contains), "actual source/deployment marker readback")
	}
	return m, nil
}
func parseNumber(s string) (float64, error) { return finiteNumber(strings.TrimSpace(s)) }
