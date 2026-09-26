package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/justyn-clark/loopexec/internal/workflow"
	"github.com/spf13/cobra"
)

// Scoped budgets count model-call reservations across independent run IDs.
// They are deliberately separate from a workflow's per-run monetary ledger:
// an adapter can invoke this command immediately before each child model call.
type scopedBudgetPolicy struct {
	SchemaVersion int            `json:"schema_version"`
	Scope         string         `json:"scope"`
	MaxCalls      int            `json:"max_calls"`
	PhaseLimits   map[string]int `json:"phase_limits"`
}

type scopedReservation struct {
	SchemaVersion int    `json:"schema_version"`
	Scope         string `json:"scope"`
	PolicyHash    string `json:"policy_hash"`
	CallID        string `json:"call_id"`
	Phase         string `json:"phase"`
}

type scopedBudgetReport struct {
	Scope          string         `json:"scope"`
	MaxCalls       int            `json:"max_calls"`
	Used           int            `json:"used"`
	Remaining      int            `json:"remaining"`
	PhaseRemaining map[string]int `json:"phase_remaining"`
	Reservation    string         `json:"reservation,omitempty"`
}

var scopedName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var scopedCallID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
var errScopedExhausted = errors.New("scoped call budget exhausted")
var errScopedDuplicate = errors.New("call_id was already reserved")
var errScopedPolicy = errors.New("scoped budget policy changed or ledger is invalid")

func readScopedPolicy(file string) (scopedBudgetPolicy, error) {
	var p scopedBudgetPolicy
	if err := workflow.Read(file, &p); err != nil {
		return p, err
	}
	if p.SchemaVersion != 1 || !scopedName.MatchString(p.Scope) || p.MaxCalls < 0 || p.MaxCalls > 1_000_000 || len(p.PhaseLimits) == 0 {
		return p, fmt.Errorf("invalid scoped budget policy")
	}
	for phase, limit := range p.PhaseLimits {
		if !scopedName.MatchString(phase) || limit < 0 || limit > p.MaxCalls {
			return p, fmt.Errorf("invalid scoped phase limit")
		}
	}
	return p, nil
}

func scopedStore(store string, p scopedBudgetPolicy) (string, error) {
	if !filepath.IsAbs(store) {
		return "", fmt.Errorf("store-dir must be absolute so run IDs cannot change its location")
	}
	info, err := os.Lstat(store)
	if err != nil {
		return "", fmt.Errorf("store-dir must be an existing directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("store-dir must be an existing directory")
	}
	return filepath.Join(store, p.Scope), nil
}

func regularDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("budget path is not a directory")
	}
	return nil
}

// exclusiveJSON creates a durable record before the caller can start a model.
// A partial record after a crash is treated as spent/invalid, never retried.
func exclusiveJSON(file string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return workflow.SyncDirectory(filepath.Dir(file))
}

func pinScopedPolicy(dir string, p scopedBudgetPolicy) error {
	file := filepath.Join(dir, "policy.json")
	if err := exclusiveJSON(file, p); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	var pinned scopedBudgetPolicy
	if err := workflow.Read(file, &pinned); err != nil || workflow.JSONHash(pinned) != workflow.JSONHash(p) {
		return errScopedPolicy
	}
	return nil
}

func scopedStatus(dir string, p scopedBudgetPolicy) (scopedBudgetReport, map[string]bool, error) {
	report := scopedBudgetReport{Scope: p.Scope, MaxCalls: p.MaxCalls, Remaining: p.MaxCalls, PhaseRemaining: map[string]int{}}
	for phase, limit := range p.PhaseLimits {
		report.PhaseRemaining[phase] = limit
	}
	seen := map[string]bool{}
	if err := regularDir(dir); errors.Is(err, os.ErrNotExist) {
		return report, seen, nil
	} else if err != nil {
		return report, nil, errScopedPolicy
	}
	var pinned scopedBudgetPolicy
	if err := workflow.Read(filepath.Join(dir, "policy.json"), &pinned); err != nil || workflow.JSONHash(pinned) != workflow.JSONHash(p) {
		return report, nil, errScopedPolicy
	}
	callsDir := filepath.Join(dir, "calls")
	if err := regularDir(callsDir); err != nil {
		return report, nil, errScopedPolicy
	}
	entries, err := os.ReadDir(callsDir)
	if err != nil {
		return report, nil, err
	}
	for i, entry := range entries {
		if entry.Name() != fmt.Sprintf("call-%06d.json", i+1) || !entry.Type().IsRegular() {
			return report, nil, errScopedPolicy
		}
		var call scopedReservation
		if err := workflow.Read(filepath.Join(callsDir, entry.Name()), &call); err != nil || call.SchemaVersion != 1 || call.Scope != p.Scope || call.PolicyHash != workflow.JSONHash(p) || !scopedCallID.MatchString(call.CallID) || seen[call.CallID] {
			return report, nil, errScopedPolicy
		}
		if _, ok := report.PhaseRemaining[call.Phase]; !ok {
			return report, nil, errScopedPolicy
		}
		seen[call.CallID] = true
		report.Used++
		report.PhaseRemaining[call.Phase]--
	}
	if report.Used > report.MaxCalls {
		return report, nil, errScopedPolicy
	}
	report.Remaining = report.MaxCalls - report.Used
	for _, remaining := range report.PhaseRemaining {
		if remaining < 0 {
			return report, nil, errScopedPolicy
		}
	}
	return report, seen, nil
}

func reserveScopedCall(store string, p scopedBudgetPolicy, callID, phase string) (scopedBudgetReport, error) {
	var empty scopedBudgetReport
	if !scopedCallID.MatchString(callID) || !scopedName.MatchString(phase) {
		return empty, fmt.Errorf("invalid call-id or phase")
	}
	if _, ok := p.PhaseLimits[phase]; !ok {
		return empty, fmt.Errorf("phase is absent from policy")
	}
	dir, err := scopedStore(store, p)
	if err != nil {
		return empty, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return empty, err
	}
	if err := regularDir(dir); err != nil {
		return empty, errScopedPolicy
	}
	if err := os.Mkdir(filepath.Join(dir, "calls"), 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return empty, err
	}
	if err := pinScopedPolicy(dir, p); err != nil {
		return empty, err
	}
	for {
		report, seen, err := scopedStatus(dir, p)
		if err != nil {
			return empty, err
		}
		if seen[callID] {
			return report, errScopedDuplicate
		}
		if report.Remaining == 0 || report.PhaseRemaining[phase] == 0 {
			return report, errScopedExhausted
		}
		file := filepath.Join(dir, "calls", fmt.Sprintf("call-%06d.json", report.Used+1))
		call := scopedReservation{SchemaVersion: 1, Scope: p.Scope, PolicyHash: workflow.JSONHash(p), CallID: callID, Phase: phase}
		if err := exclusiveJSON(file, call); errors.Is(err, os.ErrExist) {
			continue
		} else if err != nil {
			return empty, err
		}
		report.Used++
		report.Remaining--
		report.PhaseRemaining[phase]--
		report.Reservation = file
		return report, nil
	}
}

func newScopedBudgetCmd() *cobra.Command {
	var store, policyFile, phase, callID string
	root := &cobra.Command{Use: "budget", Short: "Shared call budgets across independent run IDs"}
	root.PersistentFlags().StringVar(&store, "store-dir", "", "Existing absolute directory shared by all runs in this budget scope")
	root.PersistentFlags().StringVar(&policyFile, "policy", "", "Pinned scoped budget policy JSON")
	status := &cobra.Command{Use: "status", Short: "Read remaining scoped model-call allowance", RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := readScopedPolicy(policyFile)
		if err != nil {
			return failResponse(cmd, "", exitInvariantFailed, "invariant_failed", err.Error())
		}
		dir, err := scopedStore(store, p)
		if err != nil {
			return failResponse(cmd, "", exitWorkspaceInvalid, "workspace_invalid", err.Error())
		}
		report, _, err := scopedStatus(dir, p)
		if err != nil {
			return failResponse(cmd, "", exitBudget, "cost_anomaly", err.Error())
		}
		return printResponse(cmd, response{Tool: toolName, Version: toolVersion, Status: "ok", BudgetScope: &report, Errors: []string{}})
	}}
	reserve := &cobra.Command{Use: "reserve", Short: "Consume one model-call slot before launching a child", RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := readScopedPolicy(policyFile)
		if err != nil {
			return failResponse(cmd, "", exitInvariantFailed, "invariant_failed", err.Error())
		}
		report, err := reserveScopedCall(store, p, callID, phase)
		if errors.Is(err, errScopedExhausted) {
			return failResponse(cmd, "", exitBudget, "budget_exceeded", fmt.Sprintf("scope %s has no remaining %s call allowance", p.Scope, phase))
		}
		if errors.Is(err, errScopedPolicy) || errors.Is(err, errScopedDuplicate) {
			return failResponse(cmd, "", exitBudget, "cost_anomaly", err.Error())
		}
		if err != nil {
			return failResponse(cmd, "", exitWorkspaceInvalid, "workspace_invalid", err.Error())
		}
		return printResponse(cmd, response{Tool: toolName, Version: toolVersion, Status: "reserved", BudgetScope: &report, Errors: []string{}})
	}}
	reserve.Flags().StringVar(&phase, "phase", "", "Budget phase, such as builder or critic")
	reserve.Flags().StringVar(&callID, "call-id", "", "Unique identity for this logical call; duplicates fail closed")
	root.AddCommand(status, reserve)
	return root
}
