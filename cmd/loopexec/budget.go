package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/justyn-clark/loopexec/internal/workflow"
)

type budgetBook struct {
	Mode          string                   `json:"mode"`
	MicroUSD      int64                    `json:"microusd"`
	MonetaryKnown bool                     `json:"monetary_known"`
	Calls         int64                    `json:"calls"`
	Tokens        int64                    `json:"tokens"`
	Entries       map[string]workflow.Call `json:"entries"`
	Pending       *workflow.Preflight      `json:"pending,omitempty"`
	Series        []float64                `json:"phase_costs_usd,omitempty"`
}

func newBudget(mode string) *budgetBook {
	return &budgetBook{Mode: mode, MonetaryKnown: mode != "unmetered", Entries: map[string]workflow.Call{}}
}
func microUSD(v float64) (int64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > float64(math.MaxInt64)/1e6-1 {
		return 0, fmt.Errorf("invalid money")
	}
	scaled := v * 1e6
	r := math.Round(scaled)
	if math.Abs(scaled-r) > 1e-6 {
		return 0, fmt.Errorf("money precision exceeds six decimals")
	}
	return int64(r), nil
}
func safeAdd(a, b int64) (int64, error) {
	if a < 0 || b < 0 || b > math.MaxInt64-a {
		return 0, fmt.Errorf("invalid or overflowing usage")
	}
	return a + b, nil
}
func (b *budgetBook) reserve(p workflow.Preflight, req workflow.Request, pol workflow.MeterPolicy, cap int64) string {
	if b.Pending != nil {
		return "cost_anomaly"
	}
	if p.SchemaVersion != 1 || p.RunID != req.RunID || p.Iteration != req.Iteration || p.Phase != req.Phase || p.Calls == nil {
		return "cost_anomaly"
	}
	if pol.Mode == "strict" && !p.Enforced {
		return "cost_anomaly"
	}
	sum, tokens := int64(0), int64(0)
	seen := map[string]bool{}
	for _, c := range p.Calls {
		if c.Phase == "" || !strings.HasPrefix(c.ID, workflow.CallID(req.RunID, req.Iteration, req.Phase, "")) || seen[c.ID] || c.MaxMicroUSD < 0 || c.MaxTokens < 0 {
			return "cost_anomaly"
		}
		if _, ok := b.Entries[c.ID]; ok {
			return "cost_anomaly"
		}
		seen[c.ID] = true
		var e error
		sum, e = safeAdd(sum, c.MaxMicroUSD)
		if e != nil {
			return "cost_anomaly"
		}
		tokens, e = safeAdd(tokens, c.MaxTokens)
		if e != nil {
			return "cost_anomaly"
		}
	}
	if cap > 0 && (b.MicroUSD >= cap || sum > cap-b.MicroUSD) {
		return "budget_exceeded"
	}
	if pol.MaxCalls > 0 && int64(len(p.Calls)) > pol.MaxCalls-b.Calls {
		return "budget_exceeded"
	}
	if pol.MaxTokens > 0 && tokens > pol.MaxTokens-b.Tokens {
		return "budget_exceeded"
	}
	b.Pending = &p
	return ""
}
func (b *budgetBook) reconcile(u workflow.Usage, req workflow.Request, pol workflow.MeterPolicy, cap int64) string {
	if u.SchemaVersion != 1 || u.RunID != req.RunID || u.Iteration != req.Iteration || u.Phase != req.Phase || u.Calls == nil {
		return "cost_anomaly"
	}
	allowed := map[string]workflow.Reservation{}
	if b.Pending != nil {
		if b.Pending.RunID != u.RunID || b.Pending.Iteration != u.Iteration || b.Pending.Phase != u.Phase {
			return "cost_anomaly"
		}
		for _, p := range b.Pending.Calls {
			allowed[p.ID] = p
		}
	}
	// Validate everything against a copy before atomically committing the batch.
	next := *b
	next.Entries = map[string]workflow.Call{}
	for id, c := range b.Entries {
		next.Entries[id] = c
	}
	seen := map[string]bool{}
	phaseTotal := int64(0)
	breach := false
	for _, c := range u.Calls {
		if c.Phase == "" || seen[c.ID] || !strings.HasPrefix(c.ID, workflow.CallID(req.RunID, req.Iteration, req.Phase, "")) {
			return "cost_anomaly"
		}
		seen[c.ID] = true
		old, exists := b.Entries[c.ID]
		if exists {
			if workflow.JSONHash(old) != workflow.JSONHash(c) {
				return "cost_anomaly"
			}
			continue
		}
		p, reserved := allowed[c.ID]
		if b.Pending != nil && b.Pending.Calls != nil && (!reserved || c.Phase != p.Phase) {
			return "cost_anomaly"
		}
		if c.Status != "actual" && c.Status != "unknown" && c.Status != "skipped" {
			return "cost_anomaly"
		}
		if c.Status == "actual" && c.MicroUSD == nil {
			return "cost_anomaly"
		}
		if c.Status == "unknown" && (pol.Mode != "unmetered" || c.MicroUSD != nil) {
			return "cost_anomaly"
		}
		if c.Status == "skipped" && (c.MicroUSD != nil && *c.MicroUSD != 0 || c.Tokens != nil && *c.Tokens != 0) {
			return "cost_anomaly"
		}
		usd, tok := int64(0), int64(0)
		if c.MicroUSD != nil {
			usd = *c.MicroUSD
		}
		if c.Tokens != nil {
			tok = *c.Tokens
		}
		if usd < 0 || tok < 0 {
			return "cost_anomaly"
		}
		if c.Status != "skipped" && pol.MaxTokens > 0 && c.Tokens == nil {
			return "cost_anomaly"
		}
		if reserved && (usd > p.MaxMicroUSD || tok > p.MaxTokens) {
			breach = true
		}
		var e error
		next.MicroUSD, e = safeAdd(next.MicroUSD, usd)
		if e != nil {
			return "cost_anomaly"
		}
		next.Tokens, e = safeAdd(next.Tokens, tok)
		if e != nil {
			return "cost_anomaly"
		}
		phaseTotal, e = safeAdd(phaseTotal, usd)
		if e != nil {
			return "cost_anomaly"
		}
		if c.Status != "skipped" {
			next.Calls++
		}
		if c.Status == "unknown" {
			next.MonetaryKnown = false
		}
		next.Entries[c.ID] = c
	}
	for id := range allowed {
		if !seen[id] {
			return "cost_anomaly"
		}
	}
	next.Pending = nil
	if len(next.Entries) > len(b.Entries) {
		next.Series = append(append([]float64{}, b.Series...), float64(phaseTotal)/1e6)
	}
	*b = next
	if breach {
		return "cost_anomaly"
	}
	if cap > 0 && b.MicroUSD > cap || pol.MaxCalls > 0 && b.Calls > pol.MaxCalls || pol.MaxTokens > 0 && b.Tokens > pol.MaxTokens {
		return "budget_exceeded"
	}
	sigma := pol.Sigma
	if sigma == 0 {
		sigma = 3
	}
	if len(rollingAnomalies(b.Series, sigma, 3)) > 0 {
		return "cost_anomaly"
	}
	return ""
}
func loadBudget(path, mode string) (*budgetBook, error) {
	var b budgetBook
	e := workflow.Read(path, &b)
	if os.IsNotExist(e) {
		return newBudget(mode), nil
	}
	if e != nil || b.Mode != mode || b.Entries == nil || b.MicroUSD < 0 || b.Calls < 0 || b.Tokens < 0 {
		return nil, fmt.Errorf("invalid saved accounting")
	}
	if e := b.validate(); e != nil {
		return nil, e
	}
	return &b, nil

}
func (b *budgetBook) validate() error {
	usd, tokens, calls := int64(0), int64(0), int64(0)
	known := b.Mode != "unmetered"
	for id, c := range b.Entries {
		if id != c.ID || c.Phase == "" || c.Status != "actual" && c.Status != "unknown" && c.Status != "skipped" {
			return fmt.Errorf("saved usage invalid")
		}
		if c.Status == "actual" && c.MicroUSD == nil || c.Status == "unknown" && (b.Mode != "unmetered" || c.MicroUSD != nil) {
			return fmt.Errorf("saved usage unavailable")
		}
		if c.Status == "unknown" {
			known = false
		}
		if c.Status != "skipped" {
			calls++
		}
		if c.Status == "skipped" && (c.MicroUSD != nil && *c.MicroUSD != 0 || c.Tokens != nil && *c.Tokens != 0) {
			return fmt.Errorf("invalid skipped usage")
		}
		var e error
		if c.MicroUSD != nil {
			usd, e = safeAdd(usd, *c.MicroUSD)
			if e != nil {
				return e
			}
		}
		if c.Tokens != nil {
			tokens, e = safeAdd(tokens, *c.Tokens)
			if e != nil {
				return e
			}
		}
	}
	if usd != b.MicroUSD || tokens != b.Tokens || calls != b.Calls || known != b.MonetaryKnown {
		return fmt.Errorf("saved usage totals conflict")
	}
	return nil
}
