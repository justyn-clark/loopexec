package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justyn-clark/loopexec/internal/recipeadapter"
	"github.com/justyn-clark/loopexec/internal/recipes"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"github.com/spf13/cobra"
)

type recipeSession struct {
	SchemaVersion int            `json:"schema_version"`
	RunID         string         `json:"run_id"`
	Root          string         `json:"root"`
	Directory     string         `json:"directory"`
	Binary        string         `json:"binary"`
	BinarySHA256  string         `json:"binary_sha256"`
	Config        recipes.Config `json:"config"`
	Plan          recipes.Plan   `json:"plan"`
	WorkflowHash  string         `json:"workflow_hash"`
}
type recipeEvidence struct {
	SchemaVersion int                       `json:"schema_version"`
	RunID         string                    `json:"run_id"`
	Iteration     int                       `json:"iteration"`
	WorkflowHash  string                    `json:"workflow_hash"`
	Manifest      workflow.Manifest         `json:"manifest"`
	Measurement   recipeadapter.Measurement `json:"measurement"`
	Files         map[string]string         `json:"files"`
}
type recipeSeal struct {
	Files     map[string]string `json:"files"`
	Signature string            `json:"signature"`
}

func recipeTechnical(m recipeadapter.Measurement, id string) bool {
	if id != "creative-workflow" {
		return m.Passed()
	}
	for _, check := range m.Checks {
		if check.ID == "threshold:score" || check.ID == "reviewer-accepts" {
			continue
		}
		if !check.Passed {
			return false
		}
	}
	return len(m.Checks) > 0
}

func recipeReadSession(file string) (recipeSession, error) {
	var s recipeSession
	if err := workflow.Read(file, &s); err != nil {
		return s, err
	}
	self, err := os.Executable()
	if err != nil {
		return s, err
	}
	hash, _, err := workflow.FileHash(context.Background(), self)
	if err != nil || hash != s.BinarySHA256 {
		return s, fmt.Errorf("recipe binary identity changed; retain the original executable for verification")
	}
	return s, nil
}
func recipeDir(s recipeSession, i int) string {
	return filepath.Join(s.Root, s.Directory, fmt.Sprintf("attempt-%06d", i))
}
func recipeContext(s recipeSession, i int) recipeadapter.Context {
	return recipeadapter.Context{Root: s.Root, EvidenceDir: recipeDir(s, i), Recipe: s.Plan.Recipe, Inputs: s.Plan.Inputs, Adapter: *s.Config.Adapter, Iteration: i, TimeoutSeconds: s.Plan.Limits.CommandTimeoutSeconds}
}
func recipeWriteExclusive(file string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
func recipeMeasure(ctx context.Context, s recipeSession, i int, work bool) error {
	c := recipeContext(s, i)
	if err := os.Mkdir(c.EvidenceDir, 0700); err != nil {
		return fmt.Errorf("attempt evidence must be new: %w", err)
	}
	contextFile := filepath.Join(c.EvidenceDir, "context.json")
	if err := recipeWriteExclusive(contextFile, c); err != nil {
		return err
	}
	if work {
		if err := recipeadapter.Work(ctx, c, contextFile); err != nil {
			return err
		}
	}
	measured, err := recipeadapter.Evaluate(ctx, c)
	if err != nil {
		return err
	}
	if err := recipeadapter.ExtraChecks(ctx, c, &measured); err != nil {
		return err
	}
	manifest, err := recipeadapter.Manifest(ctx, c.Root, c.Adapter.Artifacts)
	if err != nil {
		return err
	}
	evidence := recipeEvidence{SchemaVersion: 1, RunID: s.RunID, Iteration: i, WorkflowHash: s.WorkflowHash, Manifest: manifest, Measurement: measured, Files: map[string]string{}}
	err = filepath.WalkDir(c.EvidenceDir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in evidence")
		}
		rel, e := filepath.Rel(c.EvidenceDir, path)
		if e != nil {
			return e
		}
		hash, _, e := workflow.FileHash(ctx, path)
		if e != nil {
			return e
		}
		evidence.Files[filepath.ToSlash(rel)] = hash
		return nil
	})
	if err != nil {
		return err
	}
	if err = recipeWriteExclusive(filepath.Join(c.EvidenceDir, "report.json"), evidence); err != nil {
		return err
	}
	return workflow.Atomic(filepath.Join(s.Root, s.Directory, "current.json"), struct {
		Iteration int `json:"iteration"`
	}{i})
}
func recipeLoadEvidence(ctx context.Context, s recipeSession, i int, checkCandidate bool) (recipeEvidence, error) {
	var e recipeEvidence
	dir := recipeDir(s, i)
	if err := workflow.Read(filepath.Join(dir, "report.json"), &e); err != nil {
		return e, err
	}
	if e.SchemaVersion != 1 || e.RunID != s.RunID || e.Iteration != i || e.WorkflowHash != s.WorkflowHash || len(e.Measurement.Checks) == 0 {
		return e, fmt.Errorf("stale/malformed recipe evidence")
	}
	for path, want := range e.Files {
		full, err := workflow.SafePath(dir, filepath.FromSlash(path))
		if err != nil {
			return e, err
		}
		hash, _, err := workflow.FileHash(ctx, full)
		if err != nil || hash != want {
			return e, fmt.Errorf("recipe evidence file changed")
		}
	}
	if checkCandidate {
		actual, err := recipeadapter.Manifest(ctx, s.Root, s.Config.Adapter.Artifacts)
		if err != nil {
			return e, err
		}
		if workflow.JSONHash(actual) != workflow.JSONHash(e.Manifest) {
			return e, fmt.Errorf("candidate changed after measurement")
		}
	}
	return e, nil
}
func recipeIntegrity(ctx context.Context, s recipeSession, sessionFile string) ([]string, error) {
	result := []string{}
	for _, path := range append([]string{sessionFile, s.Binary}, s.Config.Adapter.Protected...) {
		full := path
		if !filepath.IsAbs(full) {
			var err error
			full, err = workflow.SafePath(s.Root, path)
			if err != nil {
				return nil, err
			}
		}
		if _, err := os.Stat(full); os.IsNotExist(err) {
			continue
		}
		info, err := os.Stat(full)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			rel, err := filepath.Rel(s.Root, full)
			if err != nil {
				return nil, err
			}
			manifest, err := recipeadapter.Manifest(ctx, s.Root, []string{rel})
			if err != nil {
				return nil, err
			}
			for _, file := range manifest.Files {
				result = append(result, "protected:"+file.Path+":"+file.SHA256)
			}
		} else {
			hash, _, err := workflow.FileHash(ctx, full)
			if err != nil {
				return nil, err
			}
			name := path
			if path == s.Binary {
				name = "[recipe-binary]"
			}
			if path == sessionFile {
				name = "[recipe-policy]"
			}
			result = append(result, "protected:"+name+":"+hash)
		}
	}
	if len(s.Config.Adapter.Inventory) > 0 {
		manifest, err := recipeadapter.Manifest(ctx, s.Root, s.Config.Adapter.Inventory)
		if err != nil {
			return nil, err
		}
		for _, f := range manifest.Files {
			if f.SHA256 != "missing" {
				result = append(result, "measured:"+f.Path)
			}
		}
	}
	if allowed, ok := s.Plan.Inputs["allowed_paths"].([]any); ok {
		manifest, e := recipeadapter.Manifest(ctx, s.Root, s.Config.Adapter.Artifacts)
		if e != nil {
			return nil, e
		}
		fixed := workflow.Manifest{Files: []workflow.Artifact{}}
		for _, f := range manifest.Files {
			mutable := false
			for _, v := range allowed {
				p := v.(string)
				if !workflow.CleanRelative(p) {
					return nil, fmt.Errorf("allowed_paths must be workspace relative")
				}
				if f.Path == p || strings.HasPrefix(f.Path, p+"/") {
					mutable = true
				}
			}
			if !mutable {
				fixed.Files = append(fixed.Files, f)
			}
		}
		result = append(result, "scope:unmodifiable-artifacts:"+workflow.JSONHash(fixed))
	}
	sort.Strings(result)
	return result, nil
}
func newRecipeHookCmd() *cobra.Command {
	return &cobra.Command{Use: "_hook <phase> <session> [request]", Hidden: true, Args: cobra.RangeArgs(2, 3), RunE: func(cmd *cobra.Command, args []string) error {
		s, err := recipeReadSession(args[1])
		if err != nil {
			return &cliError{Code: 2, Message: err.Error(), Silent: true}
		}
		output := func(v any) error { return json.NewEncoder(cmd.OutOrStdout()).Encode(v) }
		phase := args[0]
		if phase == "integrity" {
			ids, e := recipeIntegrity(cmd.Context(), s, args[1])
			if e != nil {
				return &cliError{Code: 2, Message: e.Error(), Silent: true}
			}
			return output(map[string]any{"ids": ids})
		}
		var req workflow.Request
		if len(args) == 3 {
			if err = workflow.Read(args[2], &req); err != nil || req.RunID != s.RunID || req.ConfigHash != s.WorkflowHash {
				return &cliError{Code: 2, Message: "recipe request mismatch", Silent: true}
			}
		} else {
			var current struct {
				Iteration int `json:"iteration"`
			}
			if err = workflow.Read(filepath.Join(s.Root, s.Directory, "current.json"), &current); err != nil {
				return err
			}
			req.Iteration = current.Iteration
			req.RunID = s.RunID
		}
		switch phase {
		case "exec":
			err = recipeMeasure(cmd.Context(), s, req.Iteration, true)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return &cliError{Code: 2, Message: "recipe work/measurement failed", Silent: true}
			}
			return nil
		case "reserve":
			phaseName := "builder"
			if s.Plan.Recipe == "creative-workflow" {
				phaseName = "reviewer"
			}
			return output(workflow.Preflight{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Calls: []workflow.Reservation{{ID: workflow.CallID(req.RunID, req.Iteration, req.Phase, "work"), Phase: phaseName}}})
		case "usage":
			phaseName := "builder"
			if s.Plan.Recipe == "creative-workflow" {
				phaseName = "reviewer"
			}
			_, err := os.Stat(filepath.Join(recipeDir(s, req.Iteration), "context.json"))
			if err != nil {
				return &cliError{Code: 2, Message: "missing work accounting evidence", Silent: true}
			}
			return output(workflow.Usage{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase, Calls: []workflow.Call{{ID: workflow.CallID(req.RunID, req.Iteration, req.Phase, "work"), Phase: phaseName, Status: "unknown"}}})
		case "score":
			e, err := recipeLoadEvidence(cmd.Context(), s, req.Iteration, true)
			if err != nil {
				return err
			}
			technical := recipeTechnical(e.Measurement, s.Plan.Recipe)
			score := workflow.Score{SchemaVersion: 1, RunID: req.RunID, Iteration: req.Iteration, CandidateID: req.CandidateID, Technical: technical}
			if technical {
				value, ok := e.Measurement.Metrics["score"]
				if !ok {
					return fmt.Errorf("missing reviewed score")
				}
				score.Value = &value
				score.Reviewed = true
				for _, check := range e.Measurement.Checks {
					if check.ID == "reviewer-accepts" && !check.Passed {
						score.Veto = true
					}
				}
			}
			return output(score)
		case "check", "failures":
			evidence, err := recipeLoadEvidence(cmd.Context(), s, req.Iteration, true)
			if err != nil {
				return &cliError{Code: 2, Message: "recipe evidence invalid", Silent: true}
			}
			if phase == "failures" {
				return output(map[string]any{"ids": evidence.Measurement.Failures()})
			}
			code := 0
			if !recipeTechnical(evidence.Measurement, s.Plan.Recipe) {
				code = 1
			}
			out := map[string]any{"run_id": s.RunID, "iteration": req.Iteration, "candidate_id": req.CandidateID, "evidence_sha256": workflow.JSONHash(evidence), "passed": code == 0, "failure_ids": evidence.Measurement.Failures()}
			if err := output(out); err != nil {
				return err
			}
			if code != 0 {
				return &cliError{Code: 1, Message: "recipe acceptance not met", Silent: true}
			}
			return nil
		}
		return &cliError{Code: 2, Message: "unknown recipe hook", Silent: true}
	}}
}
func recipeInitialize(ctx context.Context, root, runID string, cfg recipes.Config) (recipeSession, string, error) {
	var s recipeSession
	catalog, err := recipes.Load()
	if err != nil {
		return s, "", err
	}
	r, err := catalog.Find(cfg.Recipe)
	if err != nil {
		return s, "", err
	}
	plan, err := recipes.MakePlan(r, cfg)
	if err != nil {
		return s, "", err
	}
	if !plan.ExecutionReady {
		return s, "", fmt.Errorf("recipe requires a configured adapter and every required input")
	}
	if !validRunID(runID) {
		return s, "", fmt.Errorf("invalid recipe run ID")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return s, "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return s, "", err
	}
	self, err := os.Executable()
	if err != nil {
		return s, "", err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return s, "", err
	}
	hash, _, err := workflow.FileHash(ctx, self)
	if err != nil {
		return s, "", err
	}
	s = recipeSession{SchemaVersion: 1, RunID: runID, Root: root, Directory: filepath.Join(".loopexec", runID+"-recipe"), Binary: self, BinarySHA256: hash, Config: cfg, Plan: plan}
	referenceKeys := []string{"source", "reference", "truth", "openapi", "evaluation_set", "stems"}
	if r.ID == "migration-safety" || r.ID == "sql-optimization" {
		referenceKeys = append(referenceKeys, "fixture")
	}
	for _, key := range referenceKeys {
		if value, ok := plan.Inputs[key].(string); ok && value != "." {
			p, e := workflow.SafePath(root, value)
			if e != nil {
				return s, "", e
			}
			if _, e = os.Stat(p); e == nil {
				s.Config.Adapter.Protected = append(s.Config.Adapter.Protected, value)
			}
		}
	}
	if values, ok := plan.Inputs["references"].([]any); ok {
		for _, value := range values {
			p := value.(string)
			if workflow.CleanRelative(p) {
				s.Config.Adapter.Protected = append(s.Config.Adapter.Protected, p)
			}
		}
	}
	for _, key := range []string{"database", "baseline_query", "advisory"} {
		var path string
		if raw, ok := cfg.Adapter.Options[key]; ok && json.Unmarshal(raw, &path) == nil && workflow.CleanRelative(path) {
			s.Config.Adapter.Protected = append(s.Config.Adapter.Protected, path)
		}
	}
	if err = os.MkdirAll(filepath.Join(root, ".loopexec"), 0700); err != nil {
		return s, "", err
	}
	if err = os.Mkdir(filepath.Join(root, s.Directory), 0700); err != nil {
		return s, "", fmt.Errorf("recipe run already exists or cannot be created")
	}
	sessionFile := filepath.Join(root, s.Directory, "session.json")
	hook := func(phase string) []string { return []string{self, "recipes", "_hook", phase, sessionFile} }
	w := workflow.Config{SchemaVersion: 1, Exec: hook("exec"), Check: hook("check"), Meter: workflow.MeterPolicy{Mode: "unmetered", Usage: hook("usage")}, Candidate: cfg.Adapter.Candidate}
	if r.ID == "creative-workflow" {
		target := plan.Inputs["target_score"].(float64)
		w.Score = &workflow.ScorePolicy{Command: hook("score"), Direction: "maximize", Min: 0, Max: 1, Target: target, MinImprovement: 0.000001, Patience: plan.Limits.MaxIterations}
		if cfg.Adapter.Score != nil {
			copy := *cfg.Adapter.Score
			w.Score = &copy
			w.Score.Command = hook("score")
			if w.Score.Target != target || w.Score.Direction != "maximize" {
				return s, "", fmt.Errorf("creative score target/direction must match user acceptance")
			}
		}
		if err = validScorePolicy(w.Score); err != nil {
			return s, "", err
		}
		store := cfg.Adapter.ScopeStore
		if store == "" {
			store = filepath.Join(".loopexec", "recipe-call-scopes")
		}
		store, err = workflow.SafePath(root, store)
		if err != nil {
			return s, "", err
		}
		if err = os.MkdirAll(store, 0700); err != nil {
			return s, "", err
		}
		scope := cfg.Adapter.ReviewScope
		if scope == "" {
			scope = "creative-" + recipeHash([]byte(root + plan.Inputs["brief"].(string)))[:24]
		}
		maximum := int(plan.Inputs["max_reviews"].(float64))
		policy := scopedBudgetPolicy{SchemaVersion: 1, Scope: scope, MaxCalls: maximum, PhaseLimits: map[string]int{"reviewer": maximum}}
		if !scopedName.MatchString(scope) {
			return s, "", fmt.Errorf("invalid review_scope")
		}
		if _, err = reserveScopedCall(store, policy, workflow.CallID(runID, 0, "reviewer", "baseline"), "reviewer"); err != nil {
			return s, "", fmt.Errorf("critic admission refused before baseline: %w", err)
		}
		w.Meter.MaxCalls = int64(maximum)
		w.Meter.Reserve = hook("reserve")
		w.Meter.CallScope = &workflow.CallScopePolicy{StoreDir: store, Scope: scope, MaxCalls: maximum, PhaseLimits: policy.PhaseLimits}
	}
	if r.ID == "cost-budget" {
		w.Meter.MaxCalls = int64(plan.Inputs["max_calls"].(float64))
		w.Meter.Reserve = hook("reserve")
		store := cfg.Adapter.ScopeStore
		if store == "" {
			store = filepath.Join(".loopexec", "recipe-call-scopes")
		}
		if !workflow.CleanRelative(store) {
			return s, "", fmt.Errorf("scope_store must be relative to the workspace")
		}
		store, err = workflow.SafePath(root, store)
		if err != nil {
			return s, "", err
		}
		if err = os.MkdirAll(store, 0700); err != nil {
			return s, "", err
		}
		phases := map[string]int{}
		for _, phase := range plan.Inputs["phases"].([]any) {
			phases[phase.(string)] = int(w.Meter.MaxCalls)
		}
		if _, ok := phases["builder"]; !ok {
			return s, "", fmt.Errorf("the work binding requires builder in its declared phases")
		}
		w.Meter.CallScope = &workflow.CallScopePolicy{StoreDir: store, Scope: plan.Inputs["scope_id"].(string), MaxCalls: int(w.Meter.MaxCalls), PhaseLimits: phases}

	}
	s.WorkflowHash = workflow.JSONHash(w)
	if err = recipeWriteExclusive(sessionFile, s); err != nil {
		return s, "", err
	}
	workflowFile := filepath.Join(root, s.Directory, "workflow.json")
	if err = recipeWriteExclusive(workflowFile, w); err != nil {
		return s, "", err
	}
	if err = recipeMeasure(ctx, s, 0, false); err != nil {
		return s, "", err
	}
	return s, workflowFile, nil
}
func recipeShellArg(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func recipeExecute(cmd *cobra.Command, s recipeSession, workflowFile string) (loopState, error) {
	sessionFile := filepath.Join(s.Root, s.Directory, "session.json")
	hook := func(phase string) string {
		return recipeShellArg(s.Binary) + " recipes _hook " + phase + " " + recipeShellArg(sessionFile)
	}
	inner := &cobra.Command{}
	inner.SetContext(cmd.Context())
	inner.SetOut(io.Discard)
	inner.SetErr(io.Discard)
	cfg := runConfig{runID: s.RunID, workdir: s.Root, workflowFile: workflowFile, maxIterations: s.Plan.Limits.MaxIterations, totalTimeout: time.Duration(s.Plan.Limits.TimeoutSeconds) * time.Second, commandTimeout: time.Duration(s.Plan.Limits.CommandTimeoutSeconds) * time.Second, grace: 200 * time.Millisecond, integrityCmd: hook("integrity"), contextFiles: []string{filepath.Join(s.Directory, "session.json")}}
	if s.Plan.Recipe == "oscillation" || s.Plan.Recipe == "regression" {
		cfg.failuresCmd = hook("failures")
		cfg.noProgressK = s.Plan.Limits.MaxIterations
	}
	runErr := executeRun(inner, cfg)
	state, err := readState(filepath.Join(s.Root, ".loopexec", perRunStateName(s.RunID)))
	if err != nil {
		return state, err
	}
	if exitCode(runErr) != haltExitCode(state.HaltReason) {
		return state, fmt.Errorf("recipe run failed before a durable halt")
	}
	return state, nil
}
func recipeSealRun(s recipeSession) error {
	base := filepath.Join(s.Root, s.Directory)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(base, "verification.key"), key, 0600); err != nil {
		return err
	}
	seal := recipeSeal{Files: map[string]string{}}
	paths := []string{filepath.Join(".loopexec", perRunStateName(s.RunID)), filepath.Join(".loopexec", "run-"+s.RunID+".jsonl")}
	err := filepath.WalkDir(base, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(p) == "verification.key" {
			return nil
		}
		rel, e := filepath.Rel(s.Root, p)
		if e != nil {
			return e
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return err
	}
	for _, p := range paths {
		hash, _, err := workflow.FileHash(context.Background(), filepath.Join(s.Root, p))
		if err != nil {
			return err
		}
		seal.Files[p] = hash
	}
	body, _ := json.Marshal(seal.Files)
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	seal.Signature = hex.EncodeToString(mac.Sum(nil))
	return recipeWriteExclusive(filepath.Join(base, "verification.json"), seal)
}
func recipeVerify(ctx context.Context, s recipeSession) (recipes.Verification, error) {
	v := recipes.Verification{Recipe: s.Plan.Recipe, ExpectedHalt: "success_condition_met", Evidence: filepath.ToSlash(filepath.Join(s.Directory, "verification.json"))}
	catalog, _ := recipes.Load()
	r, _ := catalog.Find(s.Plan.Recipe)
	v.ExpectedHalt = r.ExpectedHalt
	var seal recipeSeal
	if err := workflow.Read(filepath.Join(s.Root, s.Directory, "verification.json"), &seal); err != nil {
		return v, err
	}
	key, err := os.ReadFile(filepath.Join(s.Root, s.Directory, "verification.key"))
	if err != nil {
		return v, err
	}
	body, _ := json.Marshal(seal.Files)
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	if !hmac.Equal([]byte(seal.Signature), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return v, fmt.Errorf("recipe verification seal changed")
	}
	for p, want := range seal.Files {
		full, err := workflow.SafePath(s.Root, p)
		if err != nil {
			return v, err
		}
		hash, _, err := workflow.FileHash(ctx, full)
		if err != nil || hash != want {
			return v, fmt.Errorf("receipt, policy or evidence changed")
		}
	}
	state, err := readState(filepath.Join(s.Root, ".loopexec", perRunStateName(s.RunID)))
	if err != nil {
		return v, err
	}
	v.HaltReason = state.HaltReason
	baseline, err := recipeLoadEvidence(ctx, s, 0, false)
	if err != nil {
		return v, err
	}
	v.BaselinePassed = baseline.Measurement.Passed()
	last := 0
	for i := 1; i <= state.Iteration; i++ {
		if _, err := os.Stat(filepath.Join(recipeDir(s, i), "report.json")); os.IsNotExist(err) {
			continue
		}
		evidence, err := recipeLoadEvidence(ctx, s, i, false)
		if err != nil {
			return v, err
		}
		v.Measurements = append(v.Measurements, evidence.Measurement.Metrics)
		last = i
	}
	selected := last
	if state.Candidate != nil {
		if state.Candidate.Exposed {
			selected = state.Candidate.BestIteration
		} else if state.Candidate.Restored != "" && state.Candidate.Best == "" {
			selected = 0
		}
	}
	if selected >= 0 {
		evidence, err := recipeLoadEvidence(ctx, s, selected, true)
		if err != nil {
			return v, err
		}
		v.OraclePassed = evidence.Measurement.Passed()
		v.AcceptanceMet = v.OraclePassed && state.HaltReason == "success_condition_met"
	}
	switch state.HaltReason {
	case "success_condition_met":
		_, verified, err := verifyStateFingerprint(state, s.Root)
		if err != nil || !verified {
			return v, fmt.Errorf("recipe replay mismatch")
		}
		v.ReplayVerified = &verified
		if !v.AcceptanceMet {
			return v, fmt.Errorf("green receipt without accepted measurements")
		}
	case "metric_integrity_violation":
		current, err := recipeIntegrity(ctx, s, filepath.Join(s.Root, s.Directory, "session.json"))
		if err != nil {
			return v, err
		}
		base, _ := parseFailures(strings.Join(state.IntegrityBaseline, "\n"))
		now, _ := parseFailures(strings.Join(current, "\n"))
		if len(missingMembers(base, now)) == 0 {
			return v, fmt.Errorf("integrity halt has no independently observed missing member")
		}
	case "oscillation_detected", "same_test_regressed":
		tracker := newProgressTracker(s.Plan.Limits.MaxIterations)
		reason := ""
		for i := 1; i <= last; i++ {
			e, err := recipeLoadEvidence(ctx, s, i, false)
			if err != nil {
				return v, err
			}
			set, ids := parseFailures(strings.Join(e.Measurement.Failures(), "\n"))
			reason = tracker.observe(i, set, ids)
		}
		if reason != state.HaltReason {
			return v, fmt.Errorf("recorded progress halt cannot be reproduced")
		}
		_, verified, err := verifyStateFingerprint(state, s.Root)
		if err != nil || !verified {
			return v, fmt.Errorf("guard replay mismatch")
		}
		v.ReplayVerified = &verified
	case "budget_exceeded":
		maximum, hasCalls := s.Plan.Inputs["max_calls"].(float64)
		exhausted := hasCalls && state.Budget != nil && state.Budget.Calls >= int64(maximum)
		if state.Workflow != nil && state.Workflow.Meter.CallScope != nil {
			scope := state.Workflow.Meter.CallScope
			policy := scopedBudgetPolicy{SchemaVersion: 1, Scope: scope.Scope, MaxCalls: scope.MaxCalls, PhaseLimits: scope.PhaseLimits}
			dir, e := scopedStore(scope.StoreDir, policy)
			if e != nil {
				return v, e
			}
			status, _, e := scopedStatus(dir, policy)
			if e != nil {
				return v, e
			}
			exhausted = exhausted || status.Remaining == 0
		}
		if !exhausted || (last > 0 && last >= state.Iteration) {
			return v, fmt.Errorf("budget halt lacks exhausted calls and a refused next attempt")
		}
	case "max_iterations_reached":
		if state.Iteration != s.Plan.Limits.MaxIterations || v.OraclePassed {
			return v, fmt.Errorf("iteration halt contradicts observed work")
		}
	default:
		return v, fmt.Errorf("receipt retained but halt is not verified by this adapter: %s", state.HaltReason)
	}
	v.ReceiptVerified = true
	return v, nil
}
func newRecipesRunCmd() *cobra.Command {
	var config, root, id string
	cmd := &cobra.Command{Use: "run <id>", Short: "Run a configured live recipe through the existing governor and seal its evidence", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var cfg recipes.Config
		if config == "" || workflow.Read(config, &cfg) != nil || cfg.Recipe != args[0] {
			return failResponse(cmd, "", 30, "workspace_invalid", "a matching recipe --config is required")
		}
		if id == "" {
			bytes := make([]byte, 6)
			rand.Read(bytes)
			id = args[0] + "-" + hex.EncodeToString(bytes)
		}
		s, file, err := recipeInitialize(cmd.Context(), root, id, cfg)
		if err != nil {
			return failResponse(cmd, id, 30, "workspace_invalid", err.Error())
		}
		state, err := recipeExecute(cmd, s, file)
		if err != nil {
			return failResponse(cmd, id, 40, "execution_failure", err.Error())
		}
		if err = recipeSealRun(s); err != nil {
			return failResponse(cmd, id, 40, "execution_failure", err.Error())
		}
		verification, err := recipeVerify(cmd.Context(), s)
		if err != nil {
			return failResponse(cmd, id, 13, "objective_unverified", err.Error())
		}
		verified := verification.ReceiptVerified
		if err := printResponse(cmd, response{Tool: toolName, Version: toolVersion, Status: "halted", RunID: id, Iteration: state.Iteration, HaltReason: state.HaltReason, CheckExit: state.LastCheckExit, Receipt: filepath.Join(root, ".loopexec", "run-"+id+".jsonl"), Verified: &verified, Recipes: &recipes.Result{CatalogSHA256: recipes.Digest(), Verification: &verification}, Errors: []string{}}); err != nil {
			return err
		}
		return &cliError{Code: haltExitCode(state.HaltReason), Silent: true}
	}}
	cmd.Flags().StringVar(&config, "config", "", "Versioned recipe JSON with adapter binding (required)")
	cmd.Flags().StringVar(&root, "workdir", ".", "Existing workspace root; adapter paths resolve within this directory")
	cmd.Flags().StringVar(&id, "run-id", "", "Unique run identity (default: random recipe-specific identity)")
	return cmd
}
func newRecipesVerifyCmd() *cobra.Command {
	var root, id string
	cmd := &cobra.Command{Use: "verify", Short: "Verify a recipe receipt seal, candidate binding and independently computed outcome", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !validRunID(id) {
			return failResponse(cmd, "", 30, "workspace_invalid", "verify requires a valid --run-id")
		}
		path := filepath.Join(root, ".loopexec", id+"-recipe", "session.json")
		s, err := recipeReadSession(path)
		if err != nil {
			return failResponse(cmd, id, 13, "objective_unverified", err.Error())
		}
		v, err := recipeVerify(cmd.Context(), s)
		if err != nil {
			return failResponse(cmd, id, 13, "objective_unverified", err.Error())
		}
		verified := v.ReceiptVerified
		return printResponse(cmd, response{Tool: toolName, Version: toolVersion, Status: "verified", RunID: id, HaltReason: v.HaltReason, Verified: &verified, Recipes: &recipes.Result{CatalogSHA256: recipes.Digest(), Verification: &v}, Errors: []string{}})
	}}
	cmd.Flags().StringVar(&root, "workdir", ".", "Recipe workspace")
	cmd.Flags().StringVar(&id, "run-id", "", "Original recipe run identity (required)")
	return cmd
}
