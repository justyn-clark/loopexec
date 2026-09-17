package main

import (
	"github.com/justyn-clark/loopexec/internal/workflow"
	"math"
	"path/filepath"
	"testing"
)

func money(v int64) *int64 { return &v }
func budgetFixture() (workflow.Request, workflow.Preflight, workflow.Usage, workflow.MeterPolicy) {
	req := workflow.Request{SchemaVersion: 1, RunID: "r", Iteration: 1, Phase: "exec"}
	p := workflow.Preflight{SchemaVersion: 1, RunID: "r", Iteration: 1, Phase: "exec", Enforced: true, Calls: []workflow.Reservation{{ID: workflow.CallID("r", 1, "exec", "builder"), Phase: "builder", MaxMicroUSD: 400000, MaxTokens: 40}, {ID: workflow.CallID("r", 1, "exec", "critic"), Phase: "critic", MaxMicroUSD: 200000, MaxTokens: 20}}}
	u := workflow.Usage{SchemaVersion: 1, RunID: "r", Iteration: 1, Phase: "exec", Calls: []workflow.Call{{ID: p.Calls[0].ID, Phase: "builder", Status: "actual", MicroUSD: money(400000), Tokens: money(40)}, {ID: p.Calls[1].ID, Phase: "critic", Status: "actual", MicroUSD: money(200000), Tokens: money(20)}}}
	pol := workflow.MeterPolicy{Mode: "strict", MaxCalls: 2, MaxTokens: 60}
	return req, p, u, pol
}
func TestBudgetExactAggregateAndIdempotentResume(t *testing.T) {
	req, p, u, pol := budgetFixture()
	b := newBudget("strict")
	if r := b.reserve(p, req, pol, 600000); r != "" {
		t.Fatal(r)
	}
	path := filepath.Join(t.TempDir(), "budget.json")
	if e := workflow.Atomic(path, b); e != nil {
		t.Fatal(e)
	}
	b, e := loadBudget(path, "strict")
	if e != nil {
		t.Fatal(e)
	}
	if r := b.reconcile(u, req, pol, 600000); r != "" {
		t.Fatal(r)
	}
	if r := b.reconcile(u, req, pol, 600000); r != "" {
		t.Fatal(r)
	}
	if b.MicroUSD != 600000 || b.Calls != 2 || b.Tokens != 60 || b.Pending != nil {
		t.Fatalf("%+v", b)
	}
	u.Calls[0].MicroUSD = money(1)
	if r := b.reconcile(u, req, pol, 600000); r != "cost_anomaly" {
		t.Fatal(r)
	}
	if b.MicroUSD != 600000 {
		t.Fatal("conflict mutated totals")
	}
}
func TestBudgetReservationDenialAndLimits(t *testing.T) {
	req, p, _, pol := budgetFixture()
	for _, kind := range []string{"money", "calls", "tokens", "enforcement", "duplicate", "negative", "stale"} {
		t.Run(kind, func(t *testing.T) {
			cp := p
			cp.Calls = append([]workflow.Reservation{}, p.Calls...)
			lim := pol
			cap := int64(600000)
			want := "budget_exceeded"
			switch kind {
			case "money":
				cap--
			case "calls":
				lim.MaxCalls = 1
			case "tokens":
				lim.MaxTokens = 59
			case "enforcement":
				cp.Enforced = false
				want = "cost_anomaly"
			case "duplicate":
				cp.Calls[1].ID = cp.Calls[0].ID
				want = "cost_anomaly"
			case "negative":
				cp.Calls[0].MaxMicroUSD = -1
				want = "cost_anomaly"
			case "stale":
				cp.Iteration = 0
				want = "cost_anomaly"
			}
			b := newBudget("strict")
			if r := b.reserve(cp, req, lim, cap); r != want {
				t.Fatal(r)
			}
			if b.Pending != nil {
				t.Fatal("denial mutated state")
			}
		})
	}
}
func TestBudgetUnknownMalformedAndOverrun(t *testing.T) {
	for _, kind := range []string{"missing", "unknown", "negative", "duplicate", "overrun", "wrong_phase"} {
		t.Run(kind, func(t *testing.T) {
			req, p, u, pol := budgetFixture()
			b := newBudget("strict")
			b.reserve(p, req, pol, 600000)
			switch kind {
			case "missing":
				u.Calls = u.Calls[:1]
			case "unknown":
				u.Calls[0].Status = "unknown"
				u.Calls[0].MicroUSD = nil
			case "negative":
				u.Calls[0].MicroUSD = money(-1)
			case "duplicate":
				u.Calls = append(u.Calls, u.Calls[0])
			case "overrun":
				u.Calls[0].MicroUSD = money(400001)
			case "wrong_phase":
				u.Phase = "other"
			}
			if r := b.reconcile(u, req, pol, 600000); r != "cost_anomaly" {
				t.Fatal(r)
			}
			if kind == "overrun" && b.MicroUSD != 600001 {
				t.Fatal("known overspend must be retained")
			}
		})
	}
	for _, mode := range []string{"observed", "unmetered"} {
		req, _, u, pol := budgetFixture()
		pol.Mode = mode
		pol.MaxCalls = 0
		pol.MaxTokens = 0
		b := newBudget(mode)
		if mode == "unmetered" {
			for i := range u.Calls {
				u.Calls[i].Status = "unknown"
				u.Calls[i].MicroUSD = nil
			}
		}
		b.Pending = &workflow.Preflight{RunID: req.RunID, Iteration: req.Iteration, Phase: req.Phase}
		if r := b.reconcile(u, req, pol, 0); r != "" {
			t.Fatal(r)
		}
		if b.MonetaryKnown != (mode == "observed") {
			t.Fatal("dishonest monetary status")
		}
	}
}
func TestMoneyPrecision(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), -1, 0.0000001, 1e20} {
		if _, e := microUSD(v); e == nil {
			t.Fatalf("accepted %v", v)
		}
	}
	for _, v := range []float64{0, .1, .2, .3, 0.000001, 1.234567} {
		if _, e := microUSD(v); e != nil {
			t.Fatal(v, e)
		}
	}
}
