package main

import (
	"github.com/justyn-clark/loopexec/internal/workflow"
	"math"
	"testing"
)

func numericPolicy() *workflow.ScorePolicy {
	return &workflow.ScorePolicy{Direction: "maximize", Min: 0, Max: 10, Target: 8.5, MinImprovement: .2, Tolerance: .01, Patience: 2, Command: []string{"score"}}
}
func reviewed(v float64) workflow.Score {
	return workflow.Score{Technical: true, Reviewed: true, Value: &v}
}
func TestNumericCumulativeGainsAndPatience(t *testing.T) {
	n := &numericState{}
	p := numericPolicy()
	for _, v := range []float64{8, 8.1, 8.2, 8.3, 8.4} {
		a, _, h := n.observe(p, reviewed(v))
		if !a || h != "" {
			t.Fatal(v, a, h)
		}
	}
	if *n.Best != 8.4 || *n.Anchor != 8.4 || n.Stale != 0 {
		t.Fatalf("%+v", n)
	}
	if _, _, h := n.observe(p, reviewed(8.4)); h != "" {
		t.Fatal(h)
	}
	if _, _, h := n.observe(p, reviewed(8.4)); h != "no_progress_detected" {
		t.Fatal(h)
	}
}
func TestNumericToleranceRegressionAndUnreviewed(t *testing.T) {
	n := &numericState{}
	p := numericPolicy()
	n.observe(p, reviewed(8))
	if a, _, _ := n.observe(p, reviewed(7.99)); !a {
		t.Fatal("inclusive tolerance boundary")
	}
	if a, _, _ := n.observe(p, reviewed(7.989)); a {
		t.Fatal("regression accepted")
	}
	before := n.Reviewed
	stale := n.Stale
	if a, _, h := n.observe(p, workflow.Score{}); a || h != "" {
		t.Fatal(a, h)
	}
	if n.Reviewed != before || n.Stale != stale {
		t.Fatal("unreviewed consumed patience")
	}
	if a, target, h := n.observe(p, reviewed(8.49)); !a || !target || h != "" {
		t.Fatal(a, target, h)
	}
	if *n.Anchor != 8.49 {
		t.Fatal("independent improvement boundary")
	}
}
func TestNumericInvalidAndMinimize(t *testing.T) {
	p := numericPolicy()
	for _, v := range []float64{math.NaN(), math.Inf(1), -1, 11} {
		if _, _, h := new(numericState).observe(p, reviewed(v)); h != "objective_unverified" {
			t.Fatal(v, h)
		}
	}
	if _, _, h := new(numericState).observe(p, workflow.Score{Technical: true}); h != "objective_unverified" {
		t.Fatal(h)
	}
	p.Direction = "minimize"
	p.Target = 1
	n := &numericState{}
	n.observe(p, reviewed(2))
	if a, target, h := n.observe(p, reviewed(1)); !a || !target || h != "" {
		t.Fatal(a, target, h)
	}
}

func TestNumericVetoCannotPoisonBestScore(t *testing.T) {
	p := numericPolicy()
	n := new(numericState)
	veto := reviewed(9.4)
	veto.Veto = true
	if a, target, h := n.observe(p, veto); a || target || h != "" || n.Best != nil {
		t.Fatal("critic veto was promoted", a, target, h, n)
	}
	if a, target, h := n.observe(p, reviewed(8.6)); !a || !target || h != "" {
		t.Fatal("valid lower aggregate blocked by rejected review", a, target, h)
	}
}
