package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func uniqueKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("evidence nesting too deep")
		}
		tok, e := d.Token()
		if e != nil {
			return e
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate evidence key")
				}
				seen[name] = true
				if e = value(depth + 1); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = value(depth + 1); e != nil {
					return e
				}
			}
		default:
			return fmt.Errorf("invalid evidence container")
		}
		_, e = d.Token()
		return e
	}
	return value(0)
}
func (s *Score) UnmarshalJSON(b []byte) error {
	var keys map[string]json.RawMessage
	if e := json.Unmarshal(b, &keys); e != nil {
		return e
	}
	for _, key := range []string{"schema_version", "run_id", "iteration", "candidate_id", "reviewed", "technical"} {
		if v, ok := keys[key]; !ok || string(v) == "null" {
			return fmt.Errorf("missing required score evidence")
		}
	}
	type plain Score
	return Decode(b, (*plain)(s))
}
