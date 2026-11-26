package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justyn-clark/loopexec/internal/subprocess"
)

func (c Context) RunInput(ctx context.Context, stdin string, argv ...string) (subprocess.Result, error) {
	r := subprocess.Run(ctx, subprocess.Options{Dir: c.Root, Argv: argv, Stdin: stdin, Timeout: timeSeconds(c.TimeoutSeconds), Grace: 200 * time.Millisecond, JoinGroup: true})
	if r.Cause != "" || r.Truncated {
		return r, fmt.Errorf("tool execution failed or exceeded output bound")
	}
	return r, nil
}
func sqliteQuery(ctx context.Context, c Context, database, query string) ([]map[string]any, error) {
	r, err := c.RunInput(ctx, query, c.Tool("sqlite3"), "-readonly", "-json", database)
	if err != nil || r.ExitCode != 0 {
		return nil, fmt.Errorf("read-only SQLite query failed")
	}
	var rows []map[string]any
	if strings.TrimSpace(r.Stdout) == "" {
		return []map[string]any{}, nil
	}
	if err = json.Unmarshal([]byte(r.Stdout), &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
func sqlOptimization(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"sqlite3/EXPLAIN QUERY PLAN"}}
	var database, baselineQuery string
	if err := c.Option("database", &database); err != nil {
		return m, err
	}
	if err := c.Option("baseline_query", &baselineQuery); err != nil {
		return m, err
	}
	db, err := c.Path(database)
	if err != nil {
		return m, err
	}
	reference, err := c.Path(baselineQuery)
	if err != nil {
		return m, err
	}
	candidate, err := c.Path(c.String("query"))
	if err != nil {
		return m, err
	}
	original, err := os.ReadFile(reference)
	if err != nil {
		return m, err
	}
	query, err := os.ReadFile(candidate)
	if err != nil {
		return m, err
	}
	expected, err := sqliteQuery(ctx, c, db, string(original))
	if err != nil {
		return m, err
	}
	actual, err := sqliteQuery(ctx, c, db, string(query))
	if err != nil {
		return m, err
	}
	canonical := func(rows []map[string]any) []string {
		out := []string{}
		for _, row := range rows {
			out = append(out, strings.TrimSpace(string(mustJSON(row))))
		}
		if c.String("result_comparison") == "unordered-exact" {
			sort.Strings(out)
		}
		return out
	}
	if !strings.Contains(c.String("result_comparison"), "exact") {
		return m, fmt.Errorf("SQLite built-in requires ordered-exact or unordered-exact result comparison")
	}
	m.Add("result-equivalence", string(mustJSON(canonical(expected))) == string(mustJSON(canonical(actual))), "actual result rows, NULLs and duplicates")
	samples := 5
	if err := c.Option("samples", &samples); err != nil {
		return m, err
	}
	if samples < 3 || samples > 100 {
		return m, fmt.Errorf("samples must be 3..100")
	}
	timings := []float64{}
	for i := 0; i < samples+1; i++ {
		start := time.Now()
		if _, err := sqliteQuery(ctx, c, db, string(query)); err != nil {
			return m, err
		}
		if i > 0 {
			timings = append(timings, float64(time.Since(start).Microseconds())/1000)
		}
	}
	sort.Float64s(timings)
	median := timings[len(timings)/2]
	m.Metric("query_latency_ms", median)
	m.Add("latency-target", median <= c.Number("max_latency_ms"), "median after one warmup")
	r, err := c.RunInput(ctx, "EXPLAIN QUERY PLAN "+string(query), c.Tool("sqlite3"), "-readonly", "-json", db)
	if err != nil || r.ExitCode != 0 {
		return m, fmt.Errorf("query plan unavailable")
	}
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, "explain.json"), []byte(r.Stdout), 0600); err != nil {
		return m, err
	}
	m.Metric("row_count", float64(len(actual)))
	return m, nil
}

type sqlInvariant struct {
	ID       string           `json:"id"`
	Query    string           `json:"query"`
	Expected []map[string]any `json:"expected"`
}

func migration(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"sqlite3 disposable migration/recovery"}}
	var disposable bool
	if err := c.Option("disposable", &disposable); err != nil || !disposable {
		return m, fmt.Errorf("migration adapter requires options.disposable=true")
	}
	fixture, err := c.Path(c.String("fixture"))
	if err != nil {
		return m, err
	}
	migration, err := c.Path(c.String("migration"))
	if err != nil {
		return m, err
	}
	base, err := os.ReadFile(fixture)
	if err != nil {
		return m, err
	}
	change, err := os.ReadFile(migration)
	if err != nil {
		return m, err
	}
	db := filepath.Join(c.EvidenceDir, "migration.db")
	r, err := c.RunInput(ctx, string(base), c.Tool("sqlite3"), db)
	if err != nil || r.ExitCode != 0 {
		return m, fmt.Errorf("database fixture setup failed")
	}
	before, err := c.Run(ctx, c.Root, c.Tool("sqlite3"), "-readonly", db, ".dump")
	if err != nil || before.ExitCode != 0 {
		return m, fmt.Errorf("baseline dump failed")
	}
	r, err = c.RunInput(ctx, string(change), c.Tool("sqlite3"), db)
	if err != nil {
		return m, err
	}
	m.Add("migration-applied", r.ExitCode == 0, "actual migration applied to a fresh database")
	if r.ExitCode != 0 {
		return m, nil
	}
	var invariants []sqlInvariant
	if err := c.Option("invariants", &invariants); err != nil {
		return m, err
	}
	if len(invariants) == 0 {
		return m, fmt.Errorf("independent invariant queries are required")
	}
	bound := map[string]bool{}
	for _, inv := range invariants {
		if bound[inv.ID] {
			return m, fmt.Errorf("duplicate invariant")
		}
		bound[inv.ID] = true
	}
	for _, id := range c.Strings("invariants") {
		if !bound[id] {
			return m, fmt.Errorf("declared invariant has no independent query: %s", id)
		}
	}
	for _, invariant := range invariants {
		rows, err := sqliteQuery(ctx, c, db, invariant.Query)
		if err != nil {
			return m, err
		}
		m.Add("invariant:"+invariant.ID, string(mustJSON(rows)) == string(mustJSON(invariant.Expected)), "actual database result")
	}
	for _, pragma := range []string{"integrity_check", "foreign_key_check"} {
		rows, err := sqliteQuery(ctx, c, db, "PRAGMA "+pragma+";")
		if err != nil {
			return m, err
		}
		valid := len(rows) == 0
		if pragma == "integrity_check" {
			valid = len(rows) == 1 && rows[0]["integrity_check"] == "ok"
		}
		m.Add(pragma, valid, "SQLite invariant")
	}
	if c.String("rollback_policy") == "required" {
		var rollback string
		if err := c.Option("rollback", &rollback); err != nil {
			return m, err
		}
		p, err := c.Path(rollback)
		if err != nil {
			return m, err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return m, err
		}
		r, err := c.RunInput(ctx, string(data), c.Tool("sqlite3"), db)
		if err != nil {
			return m, err
		}
		m.Add("rollback-applied", r.ExitCode == 0, "recovery executed")
		after, err := c.Run(ctx, c.Root, c.Tool("sqlite3"), "-readonly", db, ".dump")
		if err != nil {
			return m, err
		}
		normalize := func(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n") }
		m.Add("rollback-data-schema", normalize(before.Stdout) == normalize(after.Stdout), "full logical database dump equality")
	} else if c.String("rollback_policy") != "forward-only" {
		return m, fmt.Errorf("rollback_policy must be required or forward-only")
	}
	m.Metric("violations", float64(len(m.Failures())))
	return m, nil
}
