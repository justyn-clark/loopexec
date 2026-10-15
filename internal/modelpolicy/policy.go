// Package modelpolicy resolves explicit, provider-neutral model assignments.
package modelpolicy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type Identity struct {
	Provider        string `json:"provider"`
	ModelID         string `json:"model_id"`
	Version         string `json:"version,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type Profile struct {
	Identity
	Capabilities []string `json:"capabilities,omitempty"`
}

type Assignment struct {
	Profile  string   `json:"profile"`
	Reason   string   `json:"reason"`
	Requires []string `json:"requires,omitempty"`
}

type Task struct {
	Roles   map[string]Assignment `json:"roles"`
	Command []string              `json:"command,omitempty"`
}

type Policy struct {
	Mode     string             `json:"mode"`
	Task     string             `json:"task"`
	Profiles map[string]Profile `json:"profiles"`
	Tasks    map[string]Task    `json:"tasks"`
}

type Role struct {
	Identity
	Role         string   `json:"role"`
	Profile      string   `json:"profile"`
	Reason       string   `json:"reason"`
	Capabilities []string `json:"capabilities,omitempty"`
	Requires     []string `json:"requires,omitempty"`
}

type Selection struct {
	Mode      string `json:"mode"`
	Task      string `json:"task"`
	Assurance string `json:"assurance"`
	Roles     []Role `json:"roles"`
}

var name = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var identity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,199}$`)

func safeText(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 500 && !strings.ContainsFunc(s, unicode.IsControl)
}

func names(values []string) bool {
	seen := map[string]bool{}
	for _, s := range values {
		if !name.MatchString(s) || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}

// Resolve checks every configured task, not just the selected one. No I/O occurs.
func Resolve(p *Policy) (*Selection, error) {
	if p == nil {
		return nil, nil
	}
	if (p.Mode != "advisory" && p.Mode != "explicit") || !name.MatchString(p.Task) || len(p.Profiles) == 0 || len(p.Profiles) > 64 || len(p.Tasks) == 0 || len(p.Tasks) > 64 {
		return nil, fmt.Errorf("invalid model policy mode, task or profile/task count")
	}
	for id, profile := range p.Profiles {
		if !name.MatchString(id) || !identity.MatchString(profile.Provider) || !identity.MatchString(profile.ModelID) || (profile.Version != "" && !identity.MatchString(profile.Version)) || (profile.ReasoningEffort != "" && !name.MatchString(profile.ReasoningEffort)) || !names(profile.Capabilities) {
			return nil, fmt.Errorf("invalid model profile fields")
		}
	}
	var selected *Selection
	for taskID, task := range p.Tasks {
		if !name.MatchString(taskID) || len(task.Roles) == 0 || len(task.Roles) > 16 {
			return nil, fmt.Errorf("invalid model task roles")
		}
		s := &Selection{Mode: p.Mode, Task: taskID, Assurance: "guidance_only", Roles: []Role{}}
		if p.Mode == "explicit" {
			s.Assurance = "adapter_report_required"
		}
		for role, assignment := range task.Roles {
			profile, ok := p.Profiles[assignment.Profile]
			if !ok || !name.MatchString(role) || !safeText(assignment.Reason) || !names(assignment.Requires) {
				return nil, fmt.Errorf("invalid model assignment or missing profile")
			}
			for _, required := range assignment.Requires {
				found := false
				for _, capability := range profile.Capabilities {
					found = found || capability == required
				}
				if !found {
					return nil, fmt.Errorf("model profile lacks a required declared capability")
				}
			}
			s.Roles = append(s.Roles, Role{Identity: profile.Identity, Role: role, Profile: assignment.Profile, Reason: assignment.Reason, Capabilities: profile.Capabilities, Requires: assignment.Requires})
		}
		sort.Slice(s.Roles, func(i, j int) bool { return s.Roles[i].Role < s.Roles[j].Role })
		if p.Mode == "explicit" {
			if _, err := Command(task.Command, s); err != nil {
				return nil, err
			}
		}
		if taskID == p.Task {
			selected = s
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("selected model task does not exist")
	}
	return selected, nil
}

// Command substitutes whole argv elements, never shell text. Each assigned model
// and configured effort must be bound, so explicit mode cannot be metadata-only.
func Command(argv []string, s *Selection) ([]string, error) {
	if len(argv) == 0 || len(argv) > 256 {
		return nil, fmt.Errorf("explicit model task requires a bounded argv command")
	}
	values := map[string]string{}
	required := map[string]bool{}
	for _, role := range s.Roles {
		prefix := "{" + role.Role + "."
		values[prefix+"provider}"] = role.Provider
		values[prefix+"model}"] = role.ModelID
		values[prefix+"version}"] = role.Version
		values[prefix+"reasoning}"] = role.ReasoningEffort
		required[prefix+"model}"] = false
		if role.ReasoningEffort != "" {
			required[prefix+"reasoning}"] = false
		}
	}
	result := append([]string(nil), argv...)
	for i, arg := range result {
		if arg == "" || len(arg) > 8192 || strings.ContainsRune(arg, 0) {
			return nil, fmt.Errorf("invalid model command argument")
		}
		if strings.ContainsAny(arg, "{}") {
			value, ok := values[arg]
			if !ok || i == 0 || value == "" {
				return nil, fmt.Errorf("model placeholders must be known, nonempty whole arguments after the executable")
			}
			result[i] = value
			if _, ok := required[arg]; ok {
				required[arg] = true
			}
		}
	}
	for _, bound := range required {
		if !bound {
			return nil, fmt.Errorf("explicit model command must bind every role's model and configured reasoning")
		}
	}
	return result, nil
}

func (s *Selection) Find(role string) *Role {
	for i := range s.Roles {
		if s.Roles[i].Role == role {
			return &s.Roles[i]
		}
	}
	return nil
}

func (r *Role) Matches(actual *Identity) bool {
	return actual != nil && r.Provider == actual.Provider && r.ModelID == actual.ModelID && r.ReasoningEffort == actual.ReasoningEffort && (r.Version == "" || r.Version == actual.Version)
}
