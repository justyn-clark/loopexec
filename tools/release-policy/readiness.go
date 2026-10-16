package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var requiredGates = []string{"output_integrity", "accounting", "restoration", "continuation", "independent_review_delivery", "authorized_live_pilot"}

func requireStableReadiness(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "scripts/release-readiness.json"))
	if err != nil {
		return fmt.Errorf("stable publication held: readiness evidence unavailable")
	}
	var policy struct {
		Schema      int  `json:"schema_version"`
		StableReady bool `json:"stable_ready"`
		Gates       []struct {
			ID       string   `json:"id"`
			Status   string   `json:"status"`
			Evidence []string `json:"evidence"`
		} `json:"gates"`
	}
	if json.Unmarshal(data, &policy) != nil || policy.Schema != 1 || !policy.StableReady {
		return fmt.Errorf("stable publication held: explicit release acceptance is incomplete")
	}
	seen := map[string]bool{}
	for _, gate := range policy.Gates {
		if seen[gate.ID] || gate.Status != "verified" || len(gate.Evidence) == 0 {
			return fmt.Errorf("stable publication held: invalid or incomplete gate")
		}
		for _, proof := range gate.Evidence {
			if proof == "" {
				return fmt.Errorf("stable publication held: empty evidence")
			}
		}
		seen[gate.ID] = true
	}
	for _, id := range requiredGates {
		if !seen[id] {
			return fmt.Errorf("stable publication held: required gate missing")
		}
	}
	return nil
}
