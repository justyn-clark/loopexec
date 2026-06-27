package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// parseFailures turns the output of a failures command into a set of failure
// identities: non-empty, trimmed, de-duplicated lines.
func parseFailures(out string) (map[string]struct{}, []string) {
	set := map[string]struct{}{}
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		set[ln] = struct{}{}
	}
	order := make([]string, 0, len(set))
	for k := range set {
		order = append(order, k)
	}
	sort.Strings(order)
	return set, order
}

// setHash is a stable key for a failing-set, used to detect exact repeats
// (oscillation). The sorted, newline-joined identities are unambiguous.
func setHash(order []string) string {
	return strings.Join(order, "\n")
}

// missingMembers returns the baseline identities absent from current, sorted.
// Used by the metric-integrity gate: the test-determining surface captured at
// t0 MUST NOT lose a member (you cannot pass by testing less).
func missingMembers(baseline, current map[string]struct{}) []string {
	var missing []string
	for id := range baseline {
		if _, ok := current[id]; !ok {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	return missing
}

// progressTracker implements the anti-random-walk machinery (SPEC.md section 3.2):
// a monotone best-so-far ratchet over the failing-test SET, with regression,
// oscillation, and no-progress detection. "Progress" is a strict decrease in
// |F_i|; a previously-resolved failure returning is a regression; an exact
// repeat of an earlier set is oscillation.
type progressTracker struct {
	k            int                 // halt after k iterations with no new best
	started      bool                // first observation seen
	initialSize  int                 // |F_1|
	bestSize     int                 // smallest |F_i| so far
	bestIter     int                 // iteration that achieved bestSize
	lastBestIter int                 // last iteration that improved the best
	everImproved bool                // |F_i| < |F_1| ever
	resolved     map[string]struct{} // identities that left F and could regress
	prev         map[string]struct{} // F_{i-1}
	seen         map[string]int      // set-hash -> first iteration seen
}

func newProgressTracker(k int) *progressTracker {
	if k < 1 {
		k = 3
	}
	return &progressTracker{
		k:        k,
		resolved: map[string]struct{}{},
		prev:     map[string]struct{}{},
		seen:     map[string]int{},
	}
}

// observe records F_i and returns a computed halt reason, or "" to continue.
func (p *progressTracker) observe(iter int, F map[string]struct{}, order []string) string {
	size := len(F)
	h := setHash(order)

	if !p.started {
		p.started = true
		p.initialSize = size
		p.bestSize = size
		p.bestIter = iter
		p.lastBestIter = iter
		p.seen[h] = iter
		p.prev = F
		return ""
	}

	// Oscillation: this exact failing-set was produced by an earlier iteration.
	if _, ok := p.seen[h]; ok {
		return "oscillation_detected"
	}
	p.seen[h] = iter

	// Regression: a failure that was previously resolved has returned.
	for id := range F {
		if _, was := p.resolved[id]; was {
			return "same_test_regressed"
		}
	}

	// Items present last iteration but gone now are newly resolved.
	for id := range p.prev {
		if _, still := F[id]; !still {
			p.resolved[id] = struct{}{}
		}
	}

	// Best-so-far ratchet.
	if size < p.bestSize {
		p.bestSize = size
		p.bestIter = iter
		p.lastBestIter = iter
		if size < p.initialSize {
			p.everImproved = true
		}
	}

	p.prev = F

	// No progress: no new best within k iterations.
	if iter-p.lastBestIter >= p.k {
		return "no_progress_detected"
	}
	return ""
}

// newExplainHaltCmd renders why a recorded run stopped, distinguishing
// "raise the limit" (was still improving) from "do not retry" (stalled,
// regressed, or oscillating) -- the one fact a 3am operator needs.
func newExplainHaltCmd() *cobra.Command {
	var workdir, runID string

	cmd := &cobra.Command{
		Use:          "explain-halt",
		Short:        "Explain why the recorded run halted (raise-the-limit vs do-not-retry)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			sp, perr := resolveStatePath(workdir, runID)
			if perr != nil {
				return perr
			}
			st, err := readState(sp)
			if err != nil {
				return &cliError{Code: exitWorkspaceInvalid, Message: noStateMsg(runID), Cause: err}
			}

			advice, rationale := explainHalt(st)

			r := response{
				Tool:       toolName,
				Version:    toolVersion,
				Status:     "ok",
				RunID:      st.RunID,
				Iteration:  st.Iteration,
				HaltReason: st.HaltReason,
				Verdict:    advice,
				Why:        rationale,
				Errors:     []string{},
			}
			if err := printResponse(cmd, r); err != nil {
				return err
			}
			// In JSON mode the verdict/why ride inside the single object; only
			// the human view prints them as separate lines.
			if !jsonOutput {
				fmt.Fprintf(cmd.OutOrStdout(), "verdict: %s\n", advice)
				fmt.Fprintf(cmd.OutOrStdout(), "why: %s\n", rationale)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory containing .loopexec (default: current directory)")
	cmd.Flags().StringVar(&runID, "run-id", "", "Explain a specific recorded run by id (default: the latest run)")
	return cmd
}

// explainHalt maps a recorded halt into operator advice. The key distinction is
// feasibility: max-iterations while still improving means raise the limit;
// everything in the no-convergence class means retrying as-is will not converge.
func explainHalt(st loopState) (advice, rationale string) {
	improving := st.EverImproved
	bestStr := "n/a"
	if st.BestFailCount != nil {
		bestStr = fmt.Sprintf("%d (iter %d)", *st.BestFailCount, st.BestIteration)
	}
	switch st.HaltReason {
	case "success_condition_met":
		return "done", "the external check passed; the loop converged."
	case "max_iterations_reached":
		if improving {
			return "raise-the-limit", fmt.Sprintf("the failing set was still shrinking at the cap (best %s); more iterations are rational.", bestStr)
		}
		return "do-not-retry", fmt.Sprintf("hit the iteration cap without improving on the initial state (best %s); raising the limit alone will not help.", bestStr)
	case "no_progress_detected":
		if improving {
			return "investigate", fmt.Sprintf("progress stalled after improving to best %s; the remaining failures need a different approach, not more identical iterations.", bestStr)
		}
		return "do-not-retry", "no progress was ever made on the failing set; the goal may be infeasible from here."
	case "same_test_regressed":
		return "do-not-retry", "a previously-resolved failure returned (a regression broke the ratchet); investigate before retrying."
	case "oscillation_detected":
		return "do-not-retry", "the failing set repeated an earlier state (the loop is cycling); retrying as-is will not converge."
	case "infeasible_suspected":
		return "do-not-retry", "a contradictory failing-set cycle with no reachable green suggests the goal is infeasible."
	case "execution_failure":
		return "fix-substrate", "the work command exited non-zero; the agent/substrate failed before the check could be trusted."
	case "workspace_invalid":
		return "fix-config", "a precondition failed (e.g. no external check); resolve it before running."
	default:
		return "review", fmt.Sprintf("halted with %q; review the receipt.", st.HaltReason)
	}
}
