package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// probeReport is the determinism measurement for a check (SPEC.md O2).
// Determinism is reported as a confidence bound, not a one-time pass/fail of N
// runs: 0 instability in n runs bounds the flake rate to ~3/n at 95% (rule of
// three), so a useful probe reports the achieved bound and the run count.
type probeReport struct {
	Runs              int     `json:"runs"`
	Passes            int     `json:"passes"`
	Fails             int     `json:"fails"`
	FlakeCount        int     `json:"flake_count"`
	Stable            bool    `json:"stable"`
	FlakeUpperBound   float64 `json:"flake_upper_bound"`
	ConfidencePct     int     `json:"confidence_pct"`
	DistinctExitCodes []int   `json:"distinct_exit_codes"`
	MaxFlakeRate      float64 `json:"max_flake_rate,omitempty"`
	Certified         bool    `json:"certified"`
}

// probeRunsCap bounds the auto-derived run count so a tiny --max-flake-rate
// cannot launch an unbounded number of check executions.
const probeRunsCap = 500

// resolveProbeRuns derives the run count from the target flake rate via the
// rule of three (n >= 3/rate). An explicit --runs always wins.
func resolveProbeRuns(runsFlag int, maxFlakeRate float64) (runs int, capped bool) {
	if runsFlag > 0 {
		return runsFlag, false
	}
	if maxFlakeRate > 0 {
		n := int(math.Ceil(3.0 / maxFlakeRate))
		if n > probeRunsCap {
			return probeRunsCap, true
		}
		return n, false
	}
	return 10, false
}

// ruleOfThreeUpper is the 95% upper bound on the flake rate given 0 observed
// instabilities in n runs.
func ruleOfThreeUpper(n int) float64 {
	if n <= 0 {
		return 1.0
	}
	return 3.0 / float64(n)
}

// runProbe executes the check `runs` times and tallies verdict stability. The
// minority verdict count is the instability ("flake") count; a check is stable
// only if every run agreed (all pass, or all fail).
func runProbe(workdir, check string, runs int) probeReport {
	passes, fails := 0, 0
	seen := map[int]bool{}
	for range runs {
		rc, _ := runShell(workdir, check)
		seen[rc] = true
		if rc == 0 {
			passes++
		} else {
			fails++
		}
	}
	codes := make([]int, 0, len(seen))
	for c := range seen {
		codes = append(codes, c)
	}
	sort.Ints(codes)

	flake := min(passes, fails)
	return probeReport{
		Runs:              runs,
		Passes:            passes,
		Fails:             fails,
		FlakeCount:        flake,
		Stable:            flake == 0,
		FlakeUpperBound:   ruleOfThreeUpper(runs),
		ConfidencePct:     95,
		DistinctExitCodes: codes,
	}
}

func newProbeCheckCmd() *cobra.Command {
	var check, workdir string
	var runs int
	var maxFlakeRate float64

	cmd := &cobra.Command{
		Use:          "probe-check",
		Short:        "Measure check determinism as a confidence bound (no check, no loop)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(check) == "" {
				return failResponse(cmd, "", exitWorkspaceInvalid, "workspace_invalid",
					"probe-check requires an external check (--check). no check, no loop")
			}
			if workdir == "" {
				workdir = "."
			}

			n, capped := resolveProbeRuns(runs, maxFlakeRate)
			rep := runProbe(workdir, check, n)
			rep.MaxFlakeRate = maxFlakeRate
			rep.Certified = rep.Stable && (maxFlakeRate <= 0 || rep.FlakeUpperBound <= maxFlakeRate)

			r := response{Tool: toolName, Version: toolVersion, Errors: []string{}, Probe: &rep}
			if !rep.Stable {
				r.Status = "error"
				r.HaltReason = "check_flaky"
				r.Errors = []string{fmt.Sprintf(
					"check verdict varied: %d of %d runs disagreed (exit codes %v)",
					rep.FlakeCount, rep.Runs, rep.DistinctExitCodes)}
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return &cliError{Code: exitOracleUntrusted, Message: "halted: check_flaky"}
			}

			r.Status = "ok"
			if err := printResponse(cmd, r); err != nil {
				return err
			}
			if capped {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"note: probe runs capped at %d; achieved bound may exceed --max-flake-rate\n", probeRunsCap)
			}
			if maxFlakeRate > 0 && !rep.Certified {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"note: stable across %d runs but bound %.4f exceeds target %.4f; raise --runs to certify\n",
					rep.Runs, rep.FlakeUpperBound, maxFlakeRate)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&check, "check", "", "External check command to probe (required)")
	cmd.Flags().IntVar(&runs, "runs", 0, "Number of probe runs (default: derived from --max-flake-rate, else 10)")
	cmd.Flags().Float64Var(&maxFlakeRate, "max-flake-rate", 0, "Target max flake rate to certify (derives runs via rule of three)")
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory to run the check in (default: current directory)")
	return cmd
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass | fail | planned
	Detail string `json:"detail,omitempty"`
}

type doctorReport struct {
	Checks []doctorCheck `json:"checks"`
	Probe  *probeReport  `json:"probe,omitempty"`
}

// newDoctorCmd gates the preconditions a non-fraudulent loop requires. Slice 1
// enforces determinism (via the probe); hermeticity, adequacy, and isolation
// are reported as planned, not silently claimed (SPEC.md O3-O5, section 7).
func newDoctorCmd() *cobra.Command {
	var check, workdir string
	var runs int
	var maxFlakeRate float64

	cmd := &cobra.Command{
		Use:          "doctor",
		Short:        "Gate loop preconditions (determinism enforced; hermeticity/adequacy/isolation planned)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if workdir == "" {
				workdir = "."
			}
			dr := doctorReport{}

			if strings.TrimSpace(check) == "" {
				dr.Checks = []doctorCheck{{Name: "check-present", Status: "fail",
					Detail: "no --check provided; a loop requires an external check (SPEC O1)"}}
				r := response{Tool: toolName, Version: toolVersion, Status: "error",
					HaltReason: "workspace_invalid",
					Errors:     []string{"doctor: no external check (--check). no check, no loop"},
					Doctor:     &dr}
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return &cliError{Code: exitWorkspaceInvalid, Message: "doctor: no check"}
			}
			dr.Checks = append(dr.Checks, doctorCheck{Name: "check-present", Status: "pass",
				Detail: "external check provided"})

			n, _ := resolveProbeRuns(runs, maxFlakeRate)
			rep := runProbe(workdir, check, n)
			rep.MaxFlakeRate = maxFlakeRate
			rep.Certified = rep.Stable && (maxFlakeRate <= 0 || rep.FlakeUpperBound <= maxFlakeRate)
			dr.Probe = &rep

			green := rep.Stable
			if rep.Stable {
				dr.Checks = append(dr.Checks, doctorCheck{Name: "determinism", Status: "pass",
					Detail: fmt.Sprintf("stable across %d runs; flake upper bound %.4f (95%%)",
						rep.Runs, rep.FlakeUpperBound)})
			} else {
				dr.Checks = append(dr.Checks, doctorCheck{Name: "determinism", Status: "fail",
					Detail: fmt.Sprintf("verdict varied: %d of %d runs disagreed", rep.FlakeCount, rep.Runs)})
			}

			dr.Checks = append(dr.Checks,
				doctorCheck{Name: "hermeticity", Status: "planned", Detail: "not yet enforced (SPEC O3)"},
				doctorCheck{Name: "adequacy", Status: "planned", Detail: "coverage-delta + mutation canary not yet enforced (SPEC O4)"},
				doctorCheck{Name: "isolation", Status: "planned", Detail: "two-zone isolation not yet enforced (SPEC section 7)"},
			)

			r := response{Tool: toolName, Version: toolVersion, Errors: []string{}, Doctor: &dr}
			if green {
				r.Status = "ok"
				if err := printResponse(cmd, r); err != nil {
					return err
				}
				return nil
			}
			r.Status = "error"
			r.HaltReason = "check_flaky"
			r.Errors = []string{"doctor: determinism precondition failed (check_flaky)"}
			if err := printResponse(cmd, r); err != nil {
				return err
			}
			return &cliError{Code: exitOracleUntrusted, Message: "doctor: check_flaky"}
		},
	}

	cmd.Flags().StringVar(&check, "check", "", "External check command to validate")
	cmd.Flags().IntVar(&runs, "runs", 0, "Determinism probe runs (default: derived, else 10)")
	cmd.Flags().Float64Var(&maxFlakeRate, "max-flake-rate", 0, "Target max flake rate to certify")
	cmd.Flags().StringVar(&workdir, "workdir", "", "Directory to run the check in (default: current directory)")
	return cmd
}
