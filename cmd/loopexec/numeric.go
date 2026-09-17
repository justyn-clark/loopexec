package main

import (
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"math"
)

type numericState struct {
	Best     *float64 `json:"best_accepted,omitempty"`
	Anchor   *float64 `json:"patience_anchor,omitempty"`
	Reviewed int      `json:"reviewed"`
	Stale    int      `json:"reviews_since_improvement"`
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func validScorePolicy(p *workflow.ScorePolicy) error {
	if p == nil {
		return nil
	}
	if p.Direction != "maximize" && p.Direction != "minimize" || !finite(p.Min) || !finite(p.Max) || !finite(p.Target) || !finite(p.MinImprovement) || !finite(p.Tolerance) || p.Min >= p.Max || p.Target < p.Min || p.Target > p.Max || p.MinImprovement <= 0 || p.Tolerance < 0 || p.Patience < 1 || len(p.Command) == 0 {
		return fmt.Errorf("invalid numeric policy")
	}
	return nil
}

// Comparisons round to nine decimal places before boundary tests. Tolerance is
// ONLY an acceptance band; it never helps an improvement reset patience.
func scoreDelta(x float64) float64 { return math.Round(x*1e9) / 1e9 }
func (n *numericState) observe(p *workflow.ScorePolicy, s workflow.Score) (accept, target bool, halt string) {
	if !s.Technical {
		if s.Reviewed || s.Value != nil {
			return false, false, "objective_unverified"
		}
		return false, false, ""
	}
	if !s.Reviewed || s.Value == nil || !finite(*s.Value) || *s.Value < p.Min || *s.Value > p.Max {
		return false, false, "objective_unverified"
	}
	value := *s.Value
	sign := 1.0
	if p.Direction == "minimize" {
		sign = -1
	}
	n.Reviewed++
	if s.Veto {
		n.Stale++
		if n.Stale >= p.Patience {
			return false, false, "no_progress_detected"
		}
		return false, false, ""
	}

	accept = n.Best == nil || scoreDelta(sign*(value-*n.Best)) >= -p.Tolerance
	if accept && (n.Best == nil || scoreDelta(sign*(value-*n.Best)) > 0) {
		v := value
		n.Best = &v
	}
	if n.Anchor == nil || scoreDelta(sign*(value-*n.Anchor)) >= p.MinImprovement {
		v := value
		n.Anchor = &v
		n.Stale = 0
	} else {
		n.Stale++
	}
	target = accept && scoreDelta(sign*(value-p.Target)) >= -p.Tolerance
	if n.Stale >= p.Patience && !target {
		halt = "no_progress_detected"
	}
	return
}
