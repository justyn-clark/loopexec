package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/recipeadapter"
	"github.com/justyn-clark/loopexec/internal/recipes"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed recipefixtures/*
var recipeFixtures embed.FS

func prepareAdditionalRecipeDemo(ctx context.Context, id, root string, cfg recipes.Config) (result recipes.Config, failure error) {
	defer func() {
		if v := recover(); v != nil {
			result = cfg
			failure = fmt.Errorf("fixture preparation: %v", v)
		}
	}()
	set := func(k string, v any) { cfg.Inputs[k] = recipeRaw(v) }
	opt := func(k string, v any) { cfg.Adapter.Options[k] = recipeRaw(v) }
	put := func(p, s string) error {
		path := filepath.Join(root, p)
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		return os.WriteFile(path, []byte(s), 0600)
	}
	copies := map[string]string{}
	pair := func(path, bad, good string) error {
		if e := put(path, bad); e != nil {
			return e
		}
		ref := "references/fixed/" + strings.TrimPrefix(path, "candidate/")
		if e := put(ref, good); e != nil {
			return e
		}
		copies[path] = ref
		return nil
	}
	c := recipeadapter.Context{Root: root, EvidenceDir: root, Adapter: *cfg.Adapter, TimeoutSeconds: 60}
	run := func(argv ...string) error {
		r, e := c.Run(ctx, root, argv...)
		if e != nil {
			return e
		}
		if r.ExitCode != 0 {
			return fmt.Errorf("fixture tool failed: %s", r.Stderr)
		}
		return nil
	}
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	} // only local fixture construction; caller catches no acceptance errors
	for _, name := range []string{"check.mjs", "oracle.mjs", "asset.py", "benchmark.gd"} {
		b, e := recipeFixtures.ReadFile("recipefixtures/" + name)
		if e != nil {
			return cfg, e
		}
		if e = put("references/"+name, string(b)); e != nil {
			return cfg, e
		}
	}
	switch id {
	case "code-repair", "lint-types", "build-system":
		set("project", "candidate")
		set("allowed_paths", []string{"candidate/main.go"})
		set("packages", []string{"."})
		must(put("candidate/go.mod", "module jcfixture\n\ngo 1.20\n"))
		must(pair("candidate/main.go", "package main\nvar value int = \"wrong\"\nfunc main(){}\n", "package main\nvar value int = 42\nfunc main(){}\n"))
		cmd := "env GOTOOLCHAIN=local GOPROXY=off GOWORK=off GOFLAGS= go test ./..."
		set("check_command", cmd)
		set("build_command", "env GOTOOLCHAIN=local GOPROXY=off GOWORK=off GOFLAGS= go build ./...")
		set("test_command", cmd)
		set("boundary_command", "node ../references/check.mjs boundaries")
		// Drop inputs belonging to the other command recipe.
		if id == "code-repair" {
			for _, k := range []string{"packages", "build_command", "boundary_command", "check_command"} {
				delete(cfg.Inputs, k)
			}
			must(put("candidate/sum_test.go", "package main\nimport \"testing\"\nfunc TestValue(t *testing.T){if value!=42{t.Fatal(value)}}\n"))
			cfg.Adapter.Protected = append(cfg.Adapter.Protected, "candidate/sum_test.go")
			must(pair("candidate/main.go", "package main\nvar value int = 0\nfunc main(){}\n", "package main\nvar value int = 42\nfunc main(){}\n"))
		} else if id == "lint-types" {
			for _, k := range []string{"packages", "build_command", "test_command", "boundary_command"} {
				delete(cfg.Inputs, k)
			}
		} else {
			delete(cfg.Inputs, "allowed_paths")
			delete(cfg.Inputs, "check_command")
		}
	case "configuration-repair":
		must(pair("candidate/compose.json", `{"services":{"app":{"port":-1}}}`, `{"services":{"app":{"port":8080}}}`))
		set("configuration", "candidate/compose.json")
		set("sandbox", "disposable-jc-config")
		set("validator", "node references/check.mjs config-syntax")
		set("integration_command", "node references/check.mjs config-integration")
	case "performance":
		must(pair("candidate/benchmark.sh", "sleep 0.15\necho 42\n", "echo 42\n"))
		set("project", "candidate")
		set("metric", "elapsed_ms")
		set("direction", "minimize")
		set("target", 65)
		set("samples", 3)
		set("correctness_command", "node references/check.mjs benchmark")
		opt("benchmark", []string{"/bin/sh", "candidate/benchmark.sh"})
	case "responsive-ui", "accessibility", "static-site", "seo-metadata":
		good := `<!doctype html><html lang="en"><meta charset="utf-8"><title>JC independent verification</title><meta name="description" content="Bounded workflows"><link rel="canonical" href="https://demo.example/"><meta property="og:title" content="JC"><meta property="og:image" content="https://demo.example/card.png"><style>body{margin:0;font:20px Arial}main{width:90%;max-width:700px;margin:auto}</style><main><h1>Verified workflow</h1><label for="email">Email</label><input id="email" type="email"><a href="about.html">About</a></main></html>`
		bad := good
		switch id {
		case "responsive-ui":
			bad = strings.Replace(good, "width:90%", "width:1400px", 1)
			set("viewports", []string{"360x640", "768x768", "1280x800"})
			set("max_overflow_px", 0)
		case "accessibility":
			bad = strings.Replace(good, `<label for="email">Email</label>`, "", 1)
			set("rules", []string{"label"})
			set("max_critical", 0)
		case "static-site":
			bad = strings.Replace(good, "about.html", "missing.html", 1)
			set("base_url", "local-file")
			set("required_metadata", []string{"title", "description", "links"})
		case "seo-metadata":
			bad = strings.Replace(good, "<title>JC independent verification</title>", "<title></title>", 1)
			set("origin", "https://demo.example/")
			set("pages", []string{"/"})
			set("metadata_rules", []string{"title", "canonical", "og", "description", "sitemap"})
			opt("sitemap", "references/sitemap.xml")
		}
		set("project", "candidate")
		if id != "seo-metadata" {
			set("routes", []string{"/"})
		}
		if id == "static-site" {
			delete(cfg.Inputs, "base_url")
			set("base_url", "file://"+filepath.ToSlash(filepath.Join(root, "candidate"))+"/")
		}
		must(pair("candidate/index.html", bad, good))
		must(put("candidate/about.html", good))
		must(put("references/sitemap.xml", "<urlset><url><loc>https://demo.example/</loc></url></urlset>"))
	case "api-contract":
		set("project", "candidate")
		set("openapi", "references/openapi.json")
		set("base_url", "http://127.0.0.1:1")
		set("cases", []string{"GET /health"})
		must(put("references/openapi.json", `{"openapi":"3.0.3","paths":{"/health":{"get":{"responses":{"200":{"headers":{"X-Contract":{"required":true}},"content":{"application/json":{"schema":{"type":"object","required":["status"],"properties":{"status":{"type":"string","enum":["ok"]}},"additionalProperties":false}}}}}}}}}`))
		must(pair("candidate/api.json", `{"status":"broken"}`, `{"status":"ok"}`))
	case "documentation-drift", "release-readiness":
		set("project", "candidate")
		set("version", "1.2.3")
		opt("version_command", []string{"node", "references/check.mjs", "version"})
		must(pair("candidate/guide.md", "version 0.9.0", "version 1.2.3\n[artifact](app.txt)"))
		must(put("candidate/app.txt", "JC fixture 1.2.3\n"))
		if id == "documentation-drift" {
			set("docs", []string{"candidate/guide.md"})
			set("contract_command", "node references/check.mjs docs")
			set("deployment_origin", "local-only")
		} else {
			set("artifacts", []string{"candidate/app.txt", "candidate/guide.md"})
			set("gates", []string{"docs"})
			set("docs_origin", "local-only")
			must(put("candidate/CHANGELOG.md", "1.2.3"))
			opt("changelog", "candidate/CHANGELOG.md")
			opt("checksums", "candidate/SHA256SUMS")
			opt("release_checks", []map[string]any{{"id": "docs", "argv": []string{"node", "references/check.mjs", "docs"}}})
			must(put("candidate/SHA256SUMS", "bad  candidate/app.txt\nbad  candidate/guide.md\n"))
		}
	case "data-cleaning":
		source := "id,date,value\na,2026-10-27,10\nb,27/10/2026,20\n"
		must(put("references/source.csv", source))
		must(pair("candidate/output.csv", source, "id,date,value\na,2026-10-27,10\nb,2026-10-27,20\n"))
		set("source", "references/source.csv")
		set("output", "candidate/output.csv")
		set("required_fields", []string{"id", "date", "value"})
		set("id_field", "id")
		set("date_fields", []string{"date:2006-01-02"})
		set("ranges", []string{"value:0:100"})
		set("max_dropped_rows", 0)
	case "etl-reconciliation":
		source := "id,value\na,10\nb,20\n"
		must(put("references/source.csv", source))
		must(pair("candidate/target.csv", "id,value\na,10\nb,2\n", source))
		must(put("candidate/transform.mjs", "// transformation is independently reconciled against source\n"))
		set("source", "references/source.csv")
		set("target", "candidate/target.csv")
		set("transformation", "candidate/transform.mjs")
		set("reconciliation_keys", []string{"id"})
		set("totals", []string{"value"})
	case "sql-optimization":
		must(put("references/fixture.sql", "CREATE TABLE items(id INTEGER PRIMARY KEY,value INTEGER);INSERT INTO items VALUES(1,42);"))
		set("query", "candidate/query.sql")
		set("fixture", "references/fixture.sql")
		set("sandbox_database", "disposable-sqlite")
		set("max_latency_ms", 50)
		set("result_comparison", "ordered-exact")
		opt("database", "references/fixture.db")
		opt("baseline_query", "references/query.sql")
		opt("samples", 3)
		must(put("references/query.sql", "SELECT value FROM items;"))
		must(pair("candidate/query.sql", "WITH RECURSIVE work(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM work WHERE x<1000000) SELECT value FROM items WHERE (SELECT max(x) FROM work)=1000000;", "SELECT value FROM items;"))
		must(run("sqlite3", filepath.Join(root, "references/fixture.db"), "CREATE TABLE items(id INTEGER PRIMARY KEY,value INTEGER);INSERT INTO items VALUES(1,42);"))
	case "migration-safety":
		must(put("references/fixture.sql", "CREATE TABLE items(id INTEGER PRIMARY KEY,value INTEGER);INSERT INTO items VALUES(1,42);"))
		must(pair("candidate/migration.sql", "DELETE FROM items;", "ALTER TABLE items ADD COLUMN note TEXT;"))
		set("migration", "candidate/migration.sql")
		set("fixture", "references/fixture.sql")
		set("sandbox_database", "disposable-sqlite")
		set("invariants", []string{"rows", "compatibility"})
		set("rollback_policy", "forward-only")
		opt("disposable", true)
		opt("invariants", []map[string]any{{"id": "rows", "query": "SELECT COUNT(*) AS count FROM items;", "expected": []map[string]any{{"count": 1}}}, {"id": "compatibility", "query": "SELECT value FROM items WHERE id=1;", "expected": []map[string]any{{"value": 42}}}})
	case "batch-transformation":
		must(put("references/source/a.txt", `{"value":1}`))
		must(pair("candidate/output/a.json", "{broken", `{"value":1}`))
		set("source", "references/source")
		set("output", "candidate/output")
		set("format", "json")
		set("validation_rules", []string{"utf8", "json", "name:^[a-z]+\\.json$"})
	case "media-optimization":
		set("source", "references/original.png")
		set("output", "candidate/compressed.jpg")
		set("max_bytes", 30000)
		set("quality_metric", "ssim")
		set("quality_floor", 0.90)
		cfg.Adapter.Work = nil
		must(run("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=640x360:duration=1", "-frames:v", "1", filepath.Join(root, "references/original.png")))
		must(run("ffmpeg", "-v", "error", "-y", "-i", filepath.Join(root, "references/original.png"), "-frames:v", "1", "-q:v", "1", filepath.Join(root, "candidate/compressed.jpg")))
		info, e := os.Stat(filepath.Join(root, "candidate/compressed.jpg"))
		if e != nil {
			return cfg, e
		}
		set("max_bytes", info.Size()-1000)
		opt("compression_steps", []int{8, 12, 16})
	case "music-mix":
		must(os.MkdirAll(filepath.Join(root, "references/stems"), 0700))
		must(run("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=4:sample_rate=48000", "-filter_complex", "[0:a]asplit=2[a][b];[a][b]amerge=inputs=2[out]", "-map", "[out]", filepath.Join(root, "references/stems/tone.wav")))
		must(run("ffmpeg", "-v", "error", "-y", "-i", filepath.Join(root, "references/stems/tone.wav"), "-af", "pan=stereo|c0=c0|c1=-0.1*c1", filepath.Join(root, "candidate/mix.wav")))
		copies["candidate/mix.wav"] = "references/stems/tone.wav"
		set("render", "candidate/mix.wav")
		set("stems", "references/stems")
		set("max_true_peak_dbtp", -1)
		set("max_silence_seconds", 0.1)
		set("min_phase_correlation", 0.9)
		set("max_channel_imbalance_db", 1)
		opt("expected_stems", []string{"tone.wav"})
	case "ocr-extraction":
		set("source", "references/source.png")
		set("truth", "references/truth.json")
		set("required_fields", []string{"invoice", "total"})
		set("min_field_accuracy", 1)
		set("ocr_engine", "tesseract")
		opt("image", "candidate/page.png")
		opt("patterns", map[string]string{"invoice": "Invoice ([A-Z0-9]+)", "total": "Total ([0-9]+)"})
		must(put("references/truth.json", `{"invoice":"JC123","total":"1250"}`))
		must(put("references/source.html", "<!doctype html><style>body{font:56px Arial;background:white;color:black;margin:50px}</style><p>Invoice JC123</p><p>Total 1250</p>"))
		capture := c
		capture.Recipe = "frontend-capture"
		capture.Inputs = map[string]any{"page_url": "file://" + filepath.ToSlash(filepath.Join(root, "references/source.html"))}
		capture.EvidenceDir = filepath.Join(root, "ocr-source-capture")
		must(os.MkdirAll(capture.EvidenceDir, 0700))
		capture.Adapter.Options = nil
		_, e := recipeadapter.BrowserCapture(ctx, capture)
		must(e)
		data, e := os.ReadFile(filepath.Join(capture.EvidenceDir, "page-000.png"))
		must(e)
		must(os.WriteFile(filepath.Join(root, "references/source.png"), data, 0600))
		must(run("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "color=c=white:s=1200x300", "-frames:v", "1", filepath.Join(root, "candidate/page.png")))
		copies["candidate/page.png"] = "references/source.png"
	case "prompt-optimization", "creative-workflow", "dependency-remediation", "asset-optimization", "godot-performance":
		opt("oracle", []string{"node", "references/oracle.mjs", "{context}"})
		if e := prepareOracleFixture(ctx, id, root, &cfg, copies); e != nil {
			return cfg, e
		}
	case "infrastructure-drift":
		must(put("candidate/main.tf", `resource "terraform_data" "demo" { input = "desired" }`))
		cfg.Adapter.Protected = append(cfg.Adapter.Protected, "candidate/main.tf")
		cfg.Adapter.Artifacts = []string{"candidate/terraform.tfstate"}
		cfg.Adapter.Work = nil
		set("configuration", "candidate")
		set("sandbox", "jc-local-terraform-data")
		set("drift_command", "terraform plan")
		set("allowed_changes", []string{"terraform_data.demo:create", "terraform_data.demo:update"})
		opt("disposable", true)
		opt("sandbox_identity", "jc-local-terraform-data")
		must(run("terraform", "-chdir="+filepath.Join(root, "candidate"), "init", "-backend=false", "-input=false", "-no-color"))
	case "docker-reduction":
		set("project", "candidate")
		set("dockerfile", "candidate/Dockerfile")
		set("max_image_bytes", 4000000)
		set("platform", "linux/arm64")
		set("checks", []string{"selftest"})
		opt("disposable", true)
		must(put("references/container.go", `package main
import("fmt";"os")
func main(){if len(os.Args)!=2||os.Args[1]!="selftest"{os.Exit(1)};fmt.Println("healthy")}
`))
		must(run("env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOPROXY=off", "GO111MODULE=off", "GOWORK=off", c.Tool("go"), "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(root, "candidate/app"), filepath.Join(root, "references/container.go")))
		must(os.WriteFile(filepath.Join(root, "candidate/padding"), make([]byte, 6<<20), 0600))
		must(pair("candidate/Dockerfile", "FROM scratch\nCOPY app /app\nCOPY padding /padding\nENTRYPOINT [\"/app\"]\n", "FROM scratch\nCOPY app /app\nENTRYPOINT [\"/app\"]\n"))
	default:
		return cfg, fmt.Errorf("no fixture for %s", id)
	}
	if len(copies) > 0 {
		opt("demo_copies", copies)
	}
	if err := recipeWriteExclusive(filepath.Join(root, "recipe.json"), cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
func additionalRecipeDemoWork(ctx context.Context, id string, c recipeadapter.Context) error {
	var copies map[string]string
	if e := c.Option("demo_copies", &copies); e != nil {
		return e
	}
	for dest, source := range copies {
		from, e := c.Path(source)
		if e != nil {
			return e
		}
		to, e := c.Path(dest)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(from)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(filepath.Dir(to), 0700); e != nil {
			return e
		}
		if e = os.WriteFile(to, b, 0600); e != nil {
			return e
		}
	}
	if id == "release-readiness" {
		var sums strings.Builder
		for _, p := range c.Strings("artifacts") {
			b, e := os.ReadFile(filepath.Join(c.Root, p))
			if e != nil {
				return e
			}
			fmt.Fprintf(&sums, "%s  %s\n", recipeHash(b), p)
		}
		return os.WriteFile(filepath.Join(c.Root, "candidate/SHA256SUMS"), []byte(sums.String()), 0600)
	}
	if id == "asset-optimization" {
		return runAssetFixture(ctx, c, "optimize")
	}
	if len(copies) == 0 {
		return fmt.Errorf("no bounded work operation configured")
	}
	return nil
}
func recipeDemoServer(root string) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" || r.Method != "GET" {
			http.NotFound(w, r)
			return
		}
		b, e := os.ReadFile(filepath.Join(root, "candidate/api.json"))
		if e != nil {
			http.Error(w, "missing", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Contract", "jc")
		w.Write(b)
	})}
	go server.Serve(listener)
	return "http://" + listener.Addr().String(), func() { server.Close() }, nil
}
func recipeHash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
