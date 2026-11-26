package main

import (
	"context"
	"github.com/justyn-clark/loopexec/internal/recipeadapter"
	"github.com/justyn-clark/loopexec/internal/recipes"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"os"
	"path/filepath"
	"testing"
)

func TestRecipeOracleAndAcceptanceAreSeparate(t *testing.T) {
	m := recipeadapter.Measurement{Checks: []recipeadapter.Check{{ID: "technical", Passed: true}, {ID: "threshold:score", Passed: false}, {ID: "reviewer-accepts", Passed: false}}}
	if !recipeTechnical(m, "creative-workflow") || recipeTechnical(m, "code-repair") || m.Passed() {
		t.Fatal("confused technical and acceptance verdicts")
	}
	m.Checks = append(m.Checks, recipeadapter.Check{ID: "hard-rule", Passed: false})
	if recipeTechnical(m, "creative-workflow") {
		t.Fatal("vetoed technical failure")
	}
}
func TestRecipeEvidenceRejectsCandidateAndLogMutation(t *testing.T) {
	for _, target := range []string{"candidate/value.txt", "attempt/log.txt"} {
		t.Run(target, func(t *testing.T) {
			root, _ := filepath.EvalSymlinks(t.TempDir())
			os.Mkdir(filepath.Join(root, "candidate"), 0700)
			os.Mkdir(filepath.Join(root, "attempt"), 0700)
			os.WriteFile(filepath.Join(root, "candidate/value.txt"), []byte("good"), 0600)
			os.WriteFile(filepath.Join(root, "attempt/log.txt"), []byte("proof"), 0600)
			s := recipeSession{SchemaVersion: 1, Root: root, Directory: ".", RunID: "unit", WorkflowHash: "policy", Config: recipes.Config{Adapter: &recipes.Adapter{Artifacts: []string{"candidate"}}}}
			s.Directory = "evidence"
			os.Mkdir(filepath.Join(root, "evidence"), 0700)
			dir := recipeDir(s, 1)
			os.Mkdir(dir, 0700)
			os.WriteFile(filepath.Join(dir, "log.txt"), []byte("proof"), 0600)
			manifest, _ := recipeadapter.Manifest(context.Background(), root, []string{"candidate"})
			hash, _, _ := workflow.FileHash(context.Background(), filepath.Join(dir, "log.txt"))
			report := recipeEvidence{SchemaVersion: 1, RunID: s.RunID, Iteration: 1, WorkflowHash: s.WorkflowHash, Manifest: manifest, Measurement: recipeadapter.Measurement{Checks: []recipeadapter.Check{{ID: "test", Passed: true}}}, Files: map[string]string{"log.txt": hash}}
			recipeWriteExclusive(filepath.Join(dir, "report.json"), report)
			if _, e := recipeLoadEvidence(context.Background(), s, 1, true); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(root, target)
			if target == "attempt/log.txt" {
				path = filepath.Join(dir, "log.txt")
			}
			os.WriteFile(path, []byte("changed"), 0600)
			if _, e := recipeLoadEvidence(context.Background(), s, 1, true); e == nil {
				t.Fatal("accepted mutated candidate/evidence")
			}
		})
	}
}
