package main

import (
	"context"
	"time"
)

type runFailure struct {
	Reason string `json:"halt_reason"`
	Cause  string `json:"cause"`
}

type accountingSummary struct {
	TotalCalls                *int64 `json:"total_calls"`
	TotalTokens               *int64 `json:"total_tokens"`
	TotalMicroUSD             *int64 `json:"total_microusd"`
	SettledCalls              int64  `json:"settled_calls"`
	SettledTokens             int64  `json:"settled_tokens"`
	SettledMicroUSD           int64  `json:"settled_microusd"`
	UnresolvedReservations    int    `json:"unresolved_reservations"`
	DispatchAwaitingUsage     bool   `json:"dispatch_awaiting_usage"`
	ReportedExecutedUnsettled int    `json:"reported_executed_unsettled"`
	CallsKnown                bool   `json:"calls_known"`
	TokensKnown               bool   `json:"tokens_known"`
	MoneyKnown                bool   `json:"money_known"`
}

func (b *budgetBook) accounting() *accountingSummary {
	if b == nil {
		return nil
	}
	a := &accountingSummary{SettledCalls: b.Calls, SettledTokens: b.Tokens, SettledMicroUSD: b.MicroUSD, CallsKnown: b.Pending == nil, TokensKnown: b.Pending == nil, MoneyKnown: b.Pending == nil && b.MonetaryKnown, DispatchAwaitingUsage: b.Pending != nil && b.DispatchPending}
	for _, call := range b.Entries {
		if call.Status != "skipped" && call.Tokens == nil {
			a.TokensKnown = false
		}
	}
	if b.Pending != nil {
		a.UnresolvedReservations = len(b.Pending.Calls)
	}
	if b.Unsettled != nil {
		for _, call := range b.Unsettled.Calls {
			if call.Status == "actual" || call.Status == "unknown" {
				a.ReportedExecutedUnsettled++
			}
		}
	}
	if a.CallsKnown {
		v := b.Calls
		a.TotalCalls = &v
	}
	if a.TokensKnown {
		v := b.Tokens
		a.TotalTokens = &v
	}
	if a.MoneyKnown {
		v := b.MicroUSD
		a.TotalMicroUSD = &v
	}
	return a
}

func recoveryBound(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return 2 * time.Minute
	}
	return timeout
}

func restoreCandidate(c *candidateManager, timeout time.Duration, rw *receiptWriter, iteration int) error {
	ctx, cancel := context.WithTimeout(context.Background(), recoveryBound(timeout))
	defer cancel()
	return restoreCandidateContext(c, ctx, rw, iteration)
}

func restoreCandidateContext(c *candidateManager, ctx context.Context, rw *receiptWriter, iteration int) error {
	previous := c.ctx
	c.recoveryDeadline, _ = ctx.Deadline()
	c.ctx = ctx
	defer func() { c.ctx = previous }()
	rw.emit(iteration, "restoration_start", "independent recovery deadline", nil)
	err := c.restore()
	if err != nil {
		cause := "recovery_io_or_verification_failed"
		if ctx.Err() != nil {
			cause = causeOfContext(ctx)
		}
		if c.state.Restoration == nil {
			c.state.Restoration = &restoreProgress{Target: c.state.Best, Phase: "preserving_attempt", DeadlineNS: c.recoveryDeadline.UnixNano()}
		}
		c.state.Restoration.FailureCause = cause
		if saveErr := c.save(); saveErr != nil {
			rw.emit(iteration, "restoration_journal_failed", "state_write_failed", nil)
		}
		rw.emit(iteration, "restoration_failed", cause, nil)
	} else {
		rw.emit(iteration, "candidate_restored", c.state.Restored, nil)
	}
	return err
}
