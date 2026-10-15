package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/justyn-clark/loopexec/internal/modelpolicy"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"github.com/spf13/cobra"
)

func newModelsCmd() *cobra.Command {
	root := &cobra.Command{Use: "models", Short: "Inspect configured task-to-model assignments without invoking models"}
	var file, workdir string
	plan := &cobra.Command{Use: "plan", Short: "Explain the selected workflow task, role profiles and dispatch assurance; spends nothing", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var config workflow.Config
		if file == "" {
			return failResponse(cmd, "", 30, "workspace_invalid", "models plan requires --workflow")
		}
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(workdir, path)
		}
		if workflow.Read(path, &config) != nil || config.SchemaVersion != 1 || config.Models == nil {
			return failResponse(cmd, "", 30, "workspace_invalid", "workflow model policy missing or unreadable")
		}
		selection, err := modelpolicy.Resolve(config.Models)
		if err != nil {
			return failResponse(cmd, "", 30, "workspace_invalid", err.Error())
		}
		if selection.Mode == "explicit" && (len(config.Exec) != 0 || len(config.Meter.Reserve) == 0) {
			return failResponse(cmd, "", 30, "workspace_invalid", "explicit model policy requires a reservation hook and no exec override")
		}
		if jsonOutput {
			return printResponse(cmd, response{Tool: toolName, Version: toolVersion, Status: "planned", ModelSelection: selection, Errors: []string{}})
		}
		renderModelSelection(cmd.OutOrStdout(), selection)
		fmt.Fprintln(cmd.OutOrStdout(), "No commands executed. Capabilities are operator declarations; model identity is not provider-verified.")
		return nil
	}}
	plan.Flags().StringVar(&file, "workflow", "", "Workflow JSON containing model_policy (required)")
	plan.Flags().StringVar(&workdir, "workdir", ".", "Directory used to resolve the workflow path")
	root.AddCommand(plan)
	return root
}

func renderModelSelection(out io.Writer, s *modelpolicy.Selection) {
	if s == nil {
		return
	}
	fmt.Fprintf(out, "model task: %s (%s; %s)\n", s.Task, s.Mode, s.Assurance)
	for _, role := range s.Roles {
		fmt.Fprintf(out, "  %s: profile=%s model=%s/%s", role.Role, role.Profile, role.Provider, role.ModelID)
		if role.Version != "" {
			fmt.Fprintf(out, " version=%s", role.Version)
		}
		if role.ReasoningEffort != "" {
			fmt.Fprintf(out, " reasoning=%s", role.ReasoningEffort)
		}
		fmt.Fprintf(out, "\n    reason: %s\n", role.Reason)
		if len(role.Requires) > 0 {
			fmt.Fprintf(out, "    requires (declared): %s\n", strings.Join(role.Requires, ", "))
		}
	}
}

func renderReportedModels(out io.Writer, book *budgetBook) {
	counts := map[string]int{}
	if book != nil {
		for _, call := range book.Entries {
			if call.Model != nil && call.Status != "skipped" {
				m := call.Model
				label := fmt.Sprintf("%s: %s/%s", call.Phase, m.Provider, m.ModelID)
				if m.ReasoningEffort != "" {
					label += " reasoning=" + m.ReasoningEffort
				}
				if m.Version != "" {
					label += " version=" + m.Version
				}
				counts[label]++
			}
		}
	}
	fmt.Fprintln(out, "adapter-reported identities (not provider-verified):")
	if len(counts) == 0 {
		fmt.Fprintln(out, "  none recorded")
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		unit := "calls"
		if counts[key] == 1 {
			unit = "call"
		}
		fmt.Fprintf(out, "  %s (%d %s)\n", key, counts[key], unit)
	}
}

func (w *receiptWriter) modelSelection(iteration int, selection *modelpolicy.Selection) {
	if w == nil || w.f == nil || selection == nil {
		return
	}
	line, err := json.Marshal(receiptEvent{TS: nowFunc().Unix(), RunID: w.runID, Iteration: iteration, Event: "model_selection", ModelSelection: selection})
	if err == nil {
		_, err = w.f.Write(append(line, '\n'))
	}
	if err != nil {
		w.err = err
	}
}

// The trusted adapter must declare model identities before dispatch and report
// them after work. This is not independent provider attestation.
func modelReservations(s *modelpolicy.Selection, calls []workflow.Reservation) bool {
	if s == nil || s.Mode != "explicit" {
		return true
	}
	seen := map[string]bool{}
	for _, call := range calls {
		role := s.Find(call.Phase)
		if role == nil {
			if call.Model != nil {
				return false
			}
			continue
		}
		if !role.Matches(call.Model) {
			return false
		}
		seen[call.Phase] = true
	}
	return len(seen) == len(s.Roles)
}

func modelUsage(s *modelpolicy.Selection, calls []workflow.Call) bool {
	if s == nil || s.Mode != "explicit" {
		return true
	}
	for _, call := range calls {
		role := s.Find(call.Phase)
		if role == nil {
			if call.Model != nil {
				return false
			}
		} else if !(call.Status == "skipped" && call.Model == nil) && !role.Matches(call.Model) {
			return false
		}
	}
	return true
}
