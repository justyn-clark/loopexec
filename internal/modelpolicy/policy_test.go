package modelpolicy

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func policyFixture() *Policy {
	return &Policy{Mode: "explicit", Task: "studio-review", Profiles: map[string]Profile{
		"visual":  {Identity: Identity{Provider: "offline", ModelID: "fixture-vision", ReasoningEffort: "high"}, Capabilities: []string{"vision"}},
		"routine": {Identity: Identity{Provider: "offline", ModelID: "fixture-small", ReasoningEffort: "low"}},
	}, Tasks: map[string]Task{"studio-review": {Roles: map[string]Assignment{
		"critic":  {Profile: "visual", Reason: "Reference-image comparison requires vision", Requires: []string{"vision"}},
		"builder": {Profile: "routine", Reason: "Bounded routine edits"},
	}, Command: []string{"adapter", "--critic-model", "{critic.model}", "--critic-effort", "{critic.reasoning}", "--builder-model", "{builder.model}", "--builder-effort", "{builder.reasoning}"}}}}
}

func TestResolveAndCommand(t *testing.T) {
	p := policyFixture()
	s, err := Resolve(p)
	if err != nil || len(s.Roles) != 2 || s.Roles[0].Role != "builder" || s.Find("critic").ModelID != "fixture-vision" {
		t.Fatalf("%+v %v", s, err)
	}
	argv, err := Command(p.Tasks[p.Task].Command, s)
	if err != nil || !reflect.DeepEqual(argv, []string{"adapter", "--critic-model", "fixture-vision", "--critic-effort", "high", "--builder-model", "fixture-small", "--builder-effort", "low"}) {
		t.Fatalf("%v %v", argv, err)
	}
	if p.Tasks[p.Task].Command[2] != "{critic.model}" {
		t.Fatal("resolution mutated pinned input")
	}
	p.Mode = "advisory"
	task := p.Tasks[p.Task]
	task.Command = nil
	p.Tasks[p.Task] = task
	s, err = Resolve(p)
	if err != nil || s.Assurance != "guidance_only" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestModelPolicyRejectsInvalidAssignments(t *testing.T) {
	for name, mutate := range map[string]func(*Policy){
		"unknown mode": func(p *Policy) { p.Mode = "automatic" },
		"missing task": func(p *Policy) { p.Task = "missing" },
		"unknown profile": func(p *Policy) {
			p.Tasks[p.Task].Roles["critic"] = Assignment{Profile: "missing", Reason: "No fallback"}
		},
		"capability": func(p *Policy) { v := p.Profiles["visual"]; v.Capabilities = nil; p.Profiles["visual"] = v },
		"control text": func(p *Policy) {
			v := p.Tasks[p.Task].Roles["critic"]
			v.Reason = "bad\x1b[2J"
			p.Tasks[p.Task].Roles["critic"] = v
		},
		"shell model":             func(p *Policy) { v := p.Profiles["visual"]; v.ModelID = "bad; exit 0"; p.Profiles["visual"] = v },
		"partial placeholder":     func(p *Policy) { p.Tasks[p.Task].Command[2] = "--model={critic.model}" },
		"unknown placeholder":     func(p *Policy) { p.Tasks[p.Task].Command[2] = "{critic.secret}" },
		"unbound model":           func(p *Policy) { p.Tasks[p.Task].Command[2] = "hardcoded-other-model" },
		"unbound effort":          func(p *Policy) { p.Tasks[p.Task].Command[4] = "low" },
		"placeholder executable":  func(p *Policy) { p.Tasks[p.Task].Command[0] = "{critic.model}" },
		"invalid unselected task": func(p *Policy) { p.Tasks["later"] = Task{} },
	} {
		t.Run(name, func(t *testing.T) {
			p := policyFixture()
			mutate(p)
			if _, err := Resolve(p); err == nil {
				t.Fatal("unsafe or ambiguous assignment accepted")
			}
		})
	}
}

func TestLegacyAndStableSelection(t *testing.T) {
	if s, err := Resolve(nil); s != nil || err != nil {
		t.Fatalf("legacy changed: %+v %v", s, err)
	}
	var previous string
	for i := 0; i < 30; i++ {
		s, _ := Resolve(policyFixture())
		b, _ := json.Marshal(s)
		if i > 0 && string(b) != previous {
			t.Fatal("selection order is nondeterministic")
		}
		previous = string(b)
	}
	if strings.Contains(previous, "command") {
		t.Fatal("selection exposes invocation text")
	}
	s, _ := Resolve(policyFixture())
	critic := s.Find("critic")
	actual := critic.Identity
	if !critic.Matches(&actual) {
		t.Fatal("matching identity rejected")
	}
	actual.ReasoningEffort = "low"
	if critic.Matches(&actual) || critic.Matches(nil) {
		t.Fatal("mismatched or absent identity accepted")
	}
}
