package main

import (
	"fmt"
	"strings"

	"github.com/justyn-clark/loopexec/internal/workflow"
)

func loadResumeBudget(path, mode string, prior *budgetBook) (*budgetBook, error) {
	var b budgetBook
	if prior == nil || workflow.Read(path, &b) != nil || b.Mode != mode || b.Mode != prior.Mode || b.Entries == nil || b.MicroUSD < 0 || b.Calls < 0 || b.Tokens < 0 || b.validate() != nil {
		return nil, fmt.Errorf("resume accounting missing or invalid")
	}
	for id, old := range prior.Entries {
		if value, ok := b.Entries[id]; !ok || workflow.JSONHash(value) != workflow.JSONHash(old) {
			return nil, fmt.Errorf("resume accounting regressed")
		}
	}
	if len(b.Series) < len(prior.Series) || len(prior.Series) > 0 && workflow.JSONHash(b.Series[:len(prior.Series)]) != workflow.JSONHash(prior.Series) {
		return nil, fmt.Errorf("resume accounting history regressed")
	}
	if prior.Pending != nil {
		if workflow.JSONHash(b.Pending) == workflow.JSONHash(prior.Pending) {
			if prior.DispatchPending && !b.DispatchPending || prior.Unsettled != nil && b.Unsettled == nil {
				return nil, fmt.Errorf("resume dispatch evidence regressed")
			}
		} else {
			if len(prior.Pending.Calls) == 0 {
				settled := false
				prefix := workflow.CallID(prior.Pending.RunID, prior.Pending.Iteration, prior.Pending.Phase, "")
				for id := range b.Entries {
					if _, old := prior.Entries[id]; !old && strings.HasPrefix(id, prefix) {
						settled = true
					}
				}
				if !settled {
					return nil, fmt.Errorf("resume unreserved phase disappeared without settlement")
				}
			}
			for _, reserved := range prior.Pending.Calls {
				if _, ok := b.Entries[reserved.ID]; !ok {
					return nil, fmt.Errorf("resume reservation disappeared without settlement")
				}
			}
			if prior.Unsettled != nil {
				for _, reported := range prior.Unsettled.Calls {
					settled, ok := b.Entries[reported.ID]
					if !ok || reported.Status != "skipped" && settled.Status == "skipped" || reported.Tokens != nil && (settled.Tokens == nil || *settled.Tokens < *reported.Tokens) || reported.MicroUSD != nil && (settled.MicroUSD == nil || *settled.MicroUSD < *reported.MicroUSD) {
						return nil, fmt.Errorf("resume observed usage regressed during settlement")
					}
				}
			}
		}
	}
	return &b, nil
}

func resumeIdentityMatches(cfg runConfig, prior loopState) bool {
	var model *modelPin
	var sampling *samplingPin
	if cfg.modelID != "" {
		model = &modelPin{Provider: cfg.modelProvider, ID: cfg.modelID, Version: cfg.modelVersion}
		sampling = &samplingPin{Temperature: cfg.temperature, Seed: cfg.seed, MaxTokens: cfg.maxTokens}
	}
	if (model == nil) != (prior.Model == nil) || (sampling == nil) != (prior.Sampling == nil) {
		return false
	}
	return (model == nil || *model == *prior.Model) && (sampling == nil || *sampling == *prior.Sampling)
}

func candidateResumeProgress(journal, prior candidateState, iteration int) bool {
	if !validCandidateID(journal.Initial) || journal.Initial != prior.Initial || workflow.JSONHash(journal.Protected) != workflow.JSONHash(prior.Protected) {
		return false
	}
	if journal.BestIteration < 0 || journal.BestIteration > iteration || journal.Best == "" && journal.BestIteration != 0 {
		return false
	}
	if prior.Best != "" {
		if journal.Best == "" || journal.BestIteration < prior.BestIteration || journal.BestIteration > iteration {
			return false
		}
		if journal.BestIteration == prior.BestIteration && (journal.Best != prior.Best || workflow.JSONHash(journal.BestFingerprint) != workflow.JSONHash(prior.BestFingerprint) || workflow.JSONHash(journal.Numeric) != workflow.JSONHash(prior.Numeric)) {
			return false
		}
	}
	return true
}
