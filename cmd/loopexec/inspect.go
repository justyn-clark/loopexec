package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// costReport is the output of inspect-cost: the run-total against the cap and a
// sigma anomaly scan over the per-iteration cost ledger. loopexec does not meter
// live token cost (the model call lives in --exec); it owns the MATH over costs
// the operator/agent supplies, and decides budget_exceeded / cost_anomaly (18).
type costReport struct {
	Iterations int     `json:"iterations"`
	TotalUSD   float64 `json:"total_usd"`
	MeanUSD    float64 `json:"mean_usd"`
	StddevUSD  float64 `json:"stddev_usd"`
	MaxUSD     float64 `json:"max_usd"`
	BudgetUSD  float64 `json:"budget_usd,omitempty"`
	Sigma      float64 `json:"sigma"`
	OverBudget bool    `json:"over_budget"`
	// AnomalyAt holds the 1-based iteration indices whose cost spiked above the
	// rolling mean + sigma*stddev of the iterations before it.
	AnomalyAt []int `json:"anomaly_at,omitempty"`
}

// meanStddev returns the population mean and standard deviation of xs.
func meanStddev(xs []float64) (mean, stddev float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean = sum / n
	var sse float64
	for _, x := range xs {
		d := x - mean
		sse += d * d
	}
	return mean, math.Sqrt(sse / n)
}

// rollingAnomalies returns the 1-based indices whose cost exceeds the rolling
// mean + sigma*stddev of the iterations BEFORE it (online spike detection). It
// requires at least minPriors prior points and a non-zero prior stddev, so a
// flat or barely-sampled ledger raises nothing.
func rollingAnomalies(costs []float64, sigma float64, minPriors int) []int {
	var out []int
	for i := minPriors; i < len(costs); i++ {
		mean, sd := meanStddev(costs[:i])
		if sd > 0 && costs[i] > mean+sigma*sd {
			out = append(out, i+1)
		}
	}
	return out
}

// readLedger parses per-iteration USD costs from a file, one number per line.
// Blank lines are skipped and a leading '$' is tolerated.
func readLedger(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []float64
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "$"))
		if s == "" {
			continue
		}
		v, perr := strconv.ParseFloat(s, 64)
		if perr != nil {
			return nil, fmt.Errorf("line %d: %q is not a number", line, sc.Text())
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
func round4(x float64) float64 { return math.Round(x*10000) / 10000 }

// newInspectCostCmd analyzes a per-iteration cost ledger against a run-total cap
// and a sigma anomaly bound. Inputs are operator-provided (loopexec does not
// meter live cost); the decision - budget_exceeded vs cost_anomaly vs ok - is
// loopexec's, since it is a number a machine can decide (SPEC.md section 5).
func newInspectCostCmd() *cobra.Command {
	var ledger string
	var costStrs []string
	var budget, sigma float64

	cmd := &cobra.Command{
		Use:          "inspect-cost",
		Short:        "Analyze a per-iteration cost ledger against a budget cap and a sigma anomaly bound",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var series []float64
			for _, s := range costStrs {
				v, perr := strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(s), "$"), 64)
				if perr != nil {
					return &cliError{Code: exitWorkspaceInvalid, Message: "invalid --cost value: " + s}
				}
				series = append(series, v)
			}
			if ledger != "" {
				ls, err := readLedger(ledger)
				if err != nil {
					return &cliError{Code: exitWorkspaceInvalid, Message: "cannot read --ledger", Cause: err}
				}
				series = append(series, ls...)
			}
			if len(series) == 0 {
				return failResponse(cmd, "", exitInvariantFailed, "invariant_failed",
					"invariant failed: no costs (pass --ledger <file> or one or more --cost <usd>)")
			}
			for _, c := range series {
				if c < 0 {
					return failResponse(cmd, "", exitInvariantFailed, "invariant_failed",
						"invariant failed: cost ledger has a negative value")
				}
			}
			if sigma <= 0 {
				sigma = 3
			}

			total, max := 0.0, series[0]
			for _, c := range series {
				total += c
				if c > max {
					max = c
				}
			}
			mean, sd := meanStddev(series)
			anomalies := rollingAnomalies(series, sigma, 3)
			over := budget > 0 && total > budget

			rep := &costReport{
				Iterations: len(series),
				TotalUSD:   round2(total),
				MeanUSD:    round4(mean),
				StddevUSD:  round4(sd),
				MaxUSD:     round4(max),
				BudgetUSD:  budget,
				Sigma:      sigma,
				OverBudget: over,
				AnomalyAt:  anomalies,
			}

			haltReason := ""
			switch {
			case over:
				haltReason = "budget_exceeded"
			case len(anomalies) > 0:
				haltReason = "cost_anomaly"
			}

			r := response{Tool: toolName, Version: toolVersion, Status: "ok", Cost: rep, Errors: []string{}}
			if haltReason != "" {
				r.Status = "error"
				r.HaltReason = haltReason
				if over {
					r.Errors = []string{fmt.Sprintf("run-total $%.2f exceeds the $%.2f budget cap", total, budget)}
				} else {
					r.Errors = []string{fmt.Sprintf("cost spike at iteration(s) %v (> mean + %gsigma)", anomalies, sigma)}
				}
			}
			if err := printResponse(cmd, r); err != nil {
				return err
			}
			if !jsonOutput {
				renderCost(cmd, rep, haltReason)
			}
			if haltReason != "" {
				return &cliError{Code: haltExitCode(haltReason), Message: haltReason, Silent: true}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&ledger, "ledger", "", "File of per-iteration USD costs, one per line (the cost ledger)")
	cmd.Flags().StringArrayVar(&costStrs, "cost", nil, "Per-iteration USD cost (repeatable); an alternative to --ledger")
	cmd.Flags().Float64Var(&budget, "budget-usd", 0, "Run-total hard cap in USD (0 = no cap); over it halts budget_exceeded (18)")
	cmd.Flags().Float64Var(&sigma, "sigma", 3, "Anomaly threshold in standard deviations above the rolling mean (cost_anomaly, 18)")
	return cmd
}

func renderCost(cmd *cobra.Command, rep *costReport, haltReason string) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s %s\n", toolName, toolVersion)
	fmt.Fprintf(w, "iterations: %d\n", rep.Iterations)
	fmt.Fprintf(w, "total: $%.2f", rep.TotalUSD)
	if rep.BudgetUSD > 0 {
		fmt.Fprintf(w, " / $%.2f budget", rep.BudgetUSD)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "per-iteration: mean $%.4f, stddev $%.4f, max $%.4f\n", rep.MeanUSD, rep.StddevUSD, rep.MaxUSD)
	fmt.Fprintf(w, "anomaly bound: mean + %gsigma\n", rep.Sigma)
	if len(rep.AnomalyAt) > 0 {
		fmt.Fprintf(w, "anomalies: iteration(s) %v\n", rep.AnomalyAt)
	}
	switch haltReason {
	case "budget_exceeded":
		fmt.Fprintln(w, "verdict: budget_exceeded (exit 18) - over the hard cap")
	case "cost_anomaly":
		fmt.Fprintln(w, "verdict: cost_anomaly (exit 18) - a spike above the sigma bound")
	default:
		fmt.Fprintln(w, "verdict: ok - within cap and no anomaly")
	}
}
