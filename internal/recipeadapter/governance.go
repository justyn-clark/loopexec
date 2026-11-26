package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Condition struct {
	ID     string          `json:"id"`
	Key    string          `json:"key"`
	Equals json.RawMessage `json:"equals"`
}

func governance(ctx context.Context, c Context) (Measurement, error) {
	var m Measurement
	if c.Recipe == "metric-gaming" {
		var policy struct {
			Cases  string `json:"cases"`
			Values string `json:"values"`
		}
		if err := c.Option("suite", &policy); err != nil {
			return m, err
		}
		if policy.Cases == "" || policy.Values == "" {
			return m, fmt.Errorf("metric-gaming requires suite.cases and suite.values")
		}
		var values map[string]any
		data, err := os.ReadFile(filepath.Join(c.Root, policy.Values))
		if err != nil {
			return m, err
		}
		if err = json.Unmarshal(data, &values); err != nil {
			return m, err
		}
		dir, err := c.Path(policy.Cases)
		if err != nil {
			return m, err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return m, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			var test Condition
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return m, err
			}
			if err = json.Unmarshal(data, &test); err != nil {
				return m, err
			}
			actual, _ := json.Marshal(values[test.Key])
			m.Add(test.ID, string(actual) == string(test.Equals), "frozen case evaluated against actual candidate value")
		}
		m.Metric("measured_cases", float64(len(m.Checks)))
		if len(m.Checks) == 0 {
			return m, fmt.Errorf("empty measured suite")
		}
		return m, nil
	}
	if c.Recipe == "cost-budget" {
		var marker string
		if err := c.Option("completion_marker", &marker); err != nil {
			return m, err
		}
		if marker == "" {
			return m, fmt.Errorf("completion_marker required")
		}
		p, err := c.Path(marker)
		if err != nil {
			return m, err
		}
		_, err = os.Stat(p)
		var log string
		if e := c.Option("launch_log", &log); e != nil {
			return m, e
		}
		if log != "" {
			var launches []any
			b, e := os.ReadFile(filepath.Join(c.Root, log))
			if e != nil {
				return m, e
			}
			if e = json.Unmarshal(b, &launches); e != nil {
				return m, e
			}
			m.Metric("child_launches", float64(len(launches)))
		}
		m.Add("objective-complete", err == nil, "objective completion is independent of the call allowance")
		return m, nil
	}
	var conditions []Condition
	if err := c.Option("conditions", &conditions); err != nil {
		return m, err
	}
	if len(conditions) < 2 {
		return m, fmt.Errorf("at least two conditions required")
	}
	var path string
	if err := c.Option("state_file", &path); err != nil {
		return m, err
	}
	p, err := c.Path(path)
	if err != nil {
		return m, err
	}
	var state map[string]any
	b, err := os.ReadFile(p)
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(b, &state); err != nil {
		return m, err
	}
	if value, ok := state["value"].(float64); ok {
		m.Metric("state_value", value)
	}
	for _, condition := range conditions {
		actual, _ := json.Marshal(state[condition.Key])
		m.Add(condition.ID, string(actual) == string(condition.Equals), "actual state compared with pinned condition")
	}
	return m, nil
}
