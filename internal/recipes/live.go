package recipes

import (
	"encoding/json"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"strings"
)

// Adapter binds one work invocation to a separately implemented domain oracle.
type Adapter struct {
	ReviewScope   string                     `json:"review_scope,omitempty"`
	Score         *workflow.ScorePolicy      `json:"score,omitempty"`
	ScopeStore    string                     `json:"scope_store,omitempty"`
	SchemaVersion int                        `json:"schema_version"`
	Checks        []Probe                    `json:"checks,omitempty"`
	Work          []string                   `json:"work,omitempty"`
	Artifacts     []string                   `json:"artifacts"`
	Protected     []string                   `json:"protected,omitempty"`
	Inventory     []string                   `json:"inventory,omitempty"`
	Tools         map[string]string          `json:"tools,omitempty"`
	Options       map[string]json.RawMessage `json:"options,omitempty"`
	Candidate     *workflow.CandidatePolicy  `json:"candidate,omitempty"`
}

func ValidateAdapter(a *Adapter) error {
	if a == nil {
		return nil
	}
	if a.SchemaVersion != 1 || len(a.Artifacts) == 0 {
		return fmt.Errorf("adapter requires schema_version 1 and nonempty artifacts")
	}
	for _, p := range append(append(append([]string{}, a.Artifacts...), a.Protected...), a.Inventory...) {
		if !workflow.CleanRelative(p) {
			return fmt.Errorf("adapter paths must be clean relative paths")
		}
		for _, part := range strings.Split(p, "/") {
			if part == ".git" || part == ".small" || part == ".loopexec" || part == ".env" {
				return fmt.Errorf("reserved adapter path")
			}
		}
	}
	for _, arg := range a.Work {
		if !printable(arg) {
			return fmt.Errorf("invalid adapter argv")
		}
	}
	for key, value := range a.Tools {
		if !printable(key) || !printable(value) {
			return fmt.Errorf("invalid tool mapping")
		}
	}
	for _, probe := range a.Checks {
		if !identifier.MatchString(probe.ID) || len(probe.Argv) == 0 {
			return fmt.Errorf("invalid extra check")
		}
	}
	if a.Candidate != nil {
		return workflow.ValidateManifestPolicy(*a.Candidate)
	}
	return nil
}

type Verification struct {
	Recipe          string               `json:"recipe"`
	ExpectedHalt    string               `json:"expected_halt"`
	HaltReason      string               `json:"halt_reason"`
	ReceiptVerified bool                 `json:"receipt_verified"`
	OraclePassed    bool                 `json:"oracle_passed"`
	AcceptanceMet   bool                 `json:"acceptance_met"`
	BaselinePassed  bool                 `json:"baseline_passed"`
	ReplayVerified  *bool                `json:"replay_verified,omitempty"`
	Evidence        string               `json:"evidence"`
	Measurements    []map[string]float64 `json:"measurements,omitempty"`
}

type Probe struct {
	ID   string   `json:"id"`
	Argv []string `json:"argv"`
}
