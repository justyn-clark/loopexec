package main

import (
	"context"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/recipeadapter"
	"github.com/justyn-clark/loopexec/internal/recipes"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
)

func prepareOracleFixture(ctx context.Context, id, root string, cfg *recipes.Config, copies map[string]string) error {
	set := func(k string, v any) { cfg.Inputs[k] = recipeRaw(v) }
	opt := func(k string, v any) { cfg.Adapter.Options[k] = recipeRaw(v) }
	put := func(path, body string) error {
		p := filepath.Join(root, path)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		return os.WriteFile(p, []byte(body), 0600)
	}
	pair := func(path, bad, good string) error {
		if e := put(path, bad); e != nil {
			return e
		}
		ref := "references/repaired-" + filepath.Base(path)
		if e := put(ref, good); e != nil {
			return e
		}
		copies[path] = ref
		return nil
	}
	c := recipeadapter.Context{Root: root, EvidenceDir: root, Adapter: *cfg.Adapter, TimeoutSeconds: 90}
	run := func(argv ...string) error {
		r, e := c.Run(ctx, root, argv...)
		if e != nil {
			return e
		}
		if r.ExitCode != 0 {
			return fmt.Errorf("oracle fixture tool failed: %s", r.Stderr)
		}
		return nil
	}
	switch id {
	case "prompt-optimization":
		set("prompt", "candidate/prompt.txt")
		set("evaluation_set", "references/cases.json")
		set("evaluator", "jc-local-token-evaluator-v1")
		set("target_score", 1)
		set("max_latency_ms", 100)
		set("max_cost_microusd", 0)
		if e := put("references/cases.json", `[{"id":"dev-1","input":"HELLO","label":"hello","split":"development"},{"id":"held-1","input":"WORLD","label":"world","split":"holdout"},{"id":"held-2","input":"MiXeD","label":"mixed","split":"holdout"}]`); e != nil {
			return e
		}
		return pair("candidate/prompt.txt", "Return the original token", "Return the lowercase token")
	case "creative-workflow":
		cfg.Adapter.Candidate = &workflow.CandidatePolicy{Root: "candidate", Include: []string{"art.png"}}
		set("brief", "Produce the approved blue test card")
		set("references", []string{"references/approved.png"})
		set("criteria", []string{"dimensions", "palette"})
		set("target_score", 0.99)
		set("max_reviews", 4)
		set("output", "candidate/art.png")
		for _, item := range []struct{ file, color string }{{"references/approved.png", "blue"}, {"candidate/art.png", "red"}} {
			if e := run("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "color=c="+item.color+":s=320x240", "-frames:v", "1", filepath.Join(root, item.file)); e != nil {
				return e
			}
		}
		copies["candidate/art.png"] = "references/approved.png"
	case "dependency-remediation":
		set("project", "candidate")
		set("manifests", []string{"candidate/package.json", "candidate/package-lock.json"})
		set("scanner", "JC pinned GHSA-35jh-r3h4-6jhm scope v1")
		set("severities", []string{"high", "critical"})
		set("test_command", "node ../references/check.mjs dependencies")
		opt("advisory", "references/advisory.json")
		if e := put("references/advisory.json", `{"id":"GHSA-35jh-r3h4-6jhm","package":"lodash","severity":"high","affected":"<4.17.21","fixed":"4.17.21","source":"https://github.com/advisories/GHSA-35jh-r3h4-6jhm","scope":"One historical advisory only; not a current comprehensive security scan"}`); e != nil {
			return e
		}
		if e := put("candidate/package.json", `{"name":"jc-dependency-fixture","version":"1.0.0","private":true}`); e != nil {
			return e
		}
		if e := run("npm", "install", "--prefix", filepath.Join(root, "candidate"), "--ignore-scripts", "--no-audit", "--no-fund", "--save-exact", "lodash@4.17.21"); e != nil {
			return e
		}
		if e := run("npm", "install", "--prefix", filepath.Join(root, "candidate"), "--ignore-scripts", "--no-audit", "--no-fund", "--save-exact", "lodash@4.17.20"); e != nil {
			return e
		}
		cfg.Adapter.Work = []string{"npm", "install", "--prefix", "candidate", "--offline", "--ignore-scripts", "--no-audit", "--no-fund", "--save-exact", "lodash@4.17.21"}
	case "asset-optimization":
		cfg.Limits.CommandTimeoutSeconds = 120
		cfg.Limits.TimeoutSeconds = 600
		set("source", "references/source.blend")
		set("output", "candidate/optimized.blend")
		set("max_triangles", 100)
		set("max_texture_edge", 1024)
		set("min_similarity", 0.99)
		set("views", []string{"front", "side", "three-quarter"})
		c.Inputs = map[string]any{"source": "references/source.blend", "output": "candidate/optimized.blend"}
		c.Adapter = *cfg.Adapter
		if e := recipeWriteExclusive(filepath.Join(root, "asset-create.json"), c); e != nil {
			return e
		}
		if e := run(c.Tool("blender"), "--background", "--factory-startup", "--python", filepath.Join(root, "references/asset.py"), "--", "create", filepath.Join(root, "asset-create.json")); e != nil {
			return e
		}
	case "godot-performance":
		cfg.Limits.CommandTimeoutSeconds = 30
		set("project", "candidate")
		set("scene", "main.tscn")
		set("max_frame_ms", 45)
		set("max_draw_calls", 20)
		set("max_memory_bytes", 268435456)
		set("gameplay_assertions", []string{"moves-right"})
		if e := put("candidate/project.godot", "config_version=5\n[application]\nconfig/name=\"JC governed game\"\nrun/main_scene=\"res://main.tscn\"\n[display]\nwindow/size/viewport_width=320\nwindow/size/viewport_height=240\nwindow/vsync/vsync_mode=0\n[rendering]\nrenderer/rendering_method=\"gl_compatibility\"\n"); e != nil {
			return e
		}
		if e := put("candidate/main.tscn", "[gd_scene format=3]\n[node name=\"Demo\" type=\"Node2D\"]\n"); e != nil {
			return e
		}
		return pair("candidate/delay.txt", "80", "0")
	}
	return nil
}
func runAssetFixture(ctx context.Context, c recipeadapter.Context, mode string) error {
	file := filepath.Join(c.EvidenceDir, "asset-work-context.json")
	if e := os.WriteFile(file, recipeRaw(c), 0600); e != nil {
		return e
	}
	r, e := c.Run(ctx, c.Root, c.Tool("blender"), "--background", "--factory-startup", "--python", filepath.Join(c.Root, "references/asset.py"), "--", mode, file)
	if e != nil {
		return e
	}
	if r.ExitCode != 0 {
		return fmt.Errorf("Blender work failed: %s", r.Stderr)
	}
	return nil
}
