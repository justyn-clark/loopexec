// recipe-proof runs independent, fresh fixture proofs; LoopExec owns each workflow's retries.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/recipes"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type envelope struct {
	RunID      string          `json:"run_id"`
	HaltReason string          `json:"halt_reason"`
	Recipes    *recipes.Result `json:"recipes"`
	Errors     []string        `json:"errors"`
}
type row struct {
	Recipe     string               `json:"recipe"`
	Verified   bool                 `json:"verified"`
	Halt       string               `json:"halt_reason"`
	Acceptance bool                 `json:"acceptance_met"`
	Receipt    string               `json:"receipt"`
	ReceiptSHA string               `json:"receipt_sha256,omitempty"`
	Error      string               `json:"error,omitempty"`
	Metrics    []map[string]float64 `json:"measurements,omitempty"`
}

func digest(path string) string {
	b, e := os.ReadFile(path)
	if e != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func main() {
	binary := flag.String("binary", "", "matching loopexec executable")
	out := flag.String("out", "", "fresh proof directory")
	tools := flag.String("tools", "", "private domain tool mappings")
	flag.Parse()
	if *binary == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "requires -binary and -out")
		os.Exit(2)
	}
	exe, e := filepath.Abs(*binary)
	if e != nil {
		panic(e)
	}
	root, e := filepath.Abs(*out)
	if e != nil {
		panic(e)
	}
	if e = os.Mkdir(root, 0700); e != nil {
		fmt.Fprintln(os.Stderr, "proof output must be new:", e)
		os.Exit(2)
	}
	toolFile := ""
	if *tools != "" {
		toolFile, _ = filepath.Abs(*tools)
	}
	catalog, e := recipes.Load()
	if e != nil {
		panic(e)
	}
	rows := []row{}
	failures := 0
	for n, recipe := range catalog.Recipes {
		dir := filepath.Join(root, recipe.ID)
		r := row{Recipe: recipe.ID, Receipt: filepath.ToSlash(filepath.Join(recipe.ID, ".loopexec", "run-jc-"+recipe.ID+".jsonl"))}
		args := []string{"--json", "recipes", "demo", recipe.ID, "--workdir", dir}
		if toolFile != "" {
			args = append(args, "--tools", toolFile)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		command := exec.CommandContext(ctx, exe, args...)
		b, err := command.CombinedOutput()
		cancel()
		os.WriteFile(filepath.Join(root, recipe.ID+".log"), b, 0600)
		var response envelope
		if decodeErr := json.Unmarshal(b, &response); decodeErr != nil {
			r.Error = "invalid fixture response"
		} else if err != nil {
			r.Error = fmt.Sprint(response.Errors)
		} else if response.Recipes == nil || response.Recipes.Verification == nil {
			r.Error = "missing verification"
		} else {
			v := response.Recipes.Verification
			r.Halt = v.HaltReason
			r.Acceptance = v.AcceptanceMet
			r.Metrics = v.Measurements
			r.Verified = v.ReceiptVerified && !v.BaselinePassed && v.HaltReason == recipe.ExpectedHalt
			if !r.Verified {
				r.Error = "baseline/outcome/receipt mismatch"
			}
		}
		if r.Verified {
			verify := exec.Command(exe, "--json", "recipes", "verify", "--workdir", dir, "--run-id", "jc-"+recipe.ID)
			b, e := verify.CombinedOutput()
			os.WriteFile(filepath.Join(root, recipe.ID+".verify.json"), b, 0600)
			var fresh envelope
			if e != nil || json.Unmarshal(b, &fresh) != nil || fresh.Recipes == nil || fresh.Recipes.Verification == nil || !fresh.Recipes.Verification.ReceiptVerified {
				r.Verified = false
				r.Error = "independent receipt readback failed"
			}
		}
		r.ReceiptSHA = digest(filepath.Join(root, filepath.FromSlash(r.Receipt)))
		if !r.Verified {
			failures++
		}
		rows = append(rows, r)
		body, _ := json.MarshalIndent(map[string]any{"schema_version": 1, "binary_sha256": digest(exe), "catalog_sha256": recipes.Digest(), "total": len(rows), "failures": failures, "recipes": rows}, "", "  ")
		os.WriteFile(filepath.Join(root, "coverage.json"), append(body, '\n'), 0600)
		fmt.Printf("%02d/33 %-25s verified=%t %s %s\n", n+1, recipe.ID, r.Verified, r.Halt, r.Error)
	}
	if failures > 0 {
		os.Exit(1)
	}
}
