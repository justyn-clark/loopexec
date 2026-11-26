package recipeadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func readResource(ctx context.Context, c Context, resource string) ([]byte, error) {
	if !strings.HasPrefix(resource, "https://") && !strings.HasPrefix(resource, "http://") {
		p, err := c.Path(resource)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(p)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", resource, nil)
	if err != nil {
		return nil, err
	}
	client := http.Client{Timeout: timeSeconds(c.TimeoutSeconds)}
	r, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return nil, fmt.Errorf("readback HTTP status %d", r.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 4<<20+1))
	if len(b) > 4<<20 {
		return nil, fmt.Errorf("resource too large")
	}
	return b, err
}
func openapi(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"HTTP/OpenAPI schema validator"}}
	var external []string
	if err := c.Option("contract_command", &external); err != nil {
		return m, err
	}
	if len(external) > 0 {
		err := commandCheck(ctx, c, &m, "external-contract", external, c.Root)
		return m, err
	}
	path, err := c.Path(c.String("openapi"))
	if err != nil {
		return m, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	var spec map[string]any
	if err = json.Unmarshal(data, &spec); err != nil {
		if err = yaml.Unmarshal(data, &spec); err != nil {
			return m, err
		}
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		return m, fmt.Errorf("OpenAPI paths missing")
	}
	base, err := url.Parse(c.String("base_url"))
	if err != nil || base.Host == "" {
		return m, fmt.Errorf("valid HTTP base_url required")
	}
	client := http.Client{Timeout: timeSeconds(c.TimeoutSeconds)}
	for _, test := range c.Strings("cases") {
		fields := strings.Fields(test)
		if len(fields) != 2 {
			return m, fmt.Errorf("contract case syntax: METHOD /path")
		}
		method, route := strings.ToUpper(fields[0]), fields[1]
		pathSpec, ok := paths[route].(map[string]any)
		if !ok {
			return m, fmt.Errorf("case path missing in contract")
		}
		operation, ok := pathSpec[strings.ToLower(method)].(map[string]any)
		if !ok {
			return m, fmt.Errorf("case method missing in contract")
		}
		responses, ok := operation["responses"].(map[string]any)
		if !ok {
			return m, fmt.Errorf("response contract missing")
		}
		endpoint := base.ResolveReference(&url.URL{Path: route})
		req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), nil)
		if err != nil {
			return m, err
		}
		response, err := client.Do(req)
		if err != nil {
			return m, err
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
		response.Body.Close()
		if err != nil || len(body) > 1<<20 {
			return m, fmt.Errorf("API body unavailable or oversized")
		}
		contract, ok := responses[fmt.Sprint(response.StatusCode)].(map[string]any)
		m.Add(test+":status", ok, fmt.Sprint(response.StatusCode))
		if !ok {
			continue
		}
		if headers, ok := contract["headers"].(map[string]any); ok {
			for name := range headers {
				m.Add(test+":header:"+name, response.Header.Get(name) != "", "required response header")
			}
		}
		if content, ok := contract["content"].(map[string]any); ok {
			for media, v := range content {
				m.Add(test+":content-type", strings.HasPrefix(response.Header.Get("Content-Type"), media), "actual Content-Type")
				entry, ok := v.(map[string]any)
				if !ok {
					return m, fmt.Errorf("invalid media contract")
				}
				schema, ok := entry["schema"].(map[string]any)
				if !ok {
					return m, fmt.Errorf("response schema missing")
				}
				var value any
				if err = json.Unmarshal(body, &value); err != nil {
					m.Add(test+":schema", false, "invalid JSON")
					continue
				}
				valid, err := schemaValid(spec, schema, value, 0)
				if err != nil {
					return m, err
				}
				m.Add(test+":schema", valid, "body validated against frozen schema")
			}
		}
	}
	m.Metric("failing_cases", float64(len(m.Failures())))
	return m, nil
}
func schemaValid(root, schema map[string]any, value any, depth int) (bool, error) {
	allowed := map[string]bool{"$ref": true, "type": true, "nullable": true, "allOf": true, "anyOf": true, "oneOf": true, "enum": true, "required": true, "properties": true, "additionalProperties": true, "items": true, "minItems": true, "maxItems": true, "minLength": true, "maxLength": true, "pattern": true, "minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true, "title": true, "description": true, "example": true, "default": true, "readOnly": true, "writeOnly": true, "deprecated": true}
	for key := range schema {
		if !allowed[key] && !strings.HasPrefix(key, "x-") {
			return false, fmt.Errorf("unsupported OpenAPI validation keyword %s; bind a complete external verifier", key)
		}
	}
	if _, ok := schema["additionalProperties"].(map[string]any); ok {
		return false, fmt.Errorf("schema-valued additionalProperties requires external contract verifier")
	}
	if depth > 64 {
		return false, fmt.Errorf("schema nesting exceeded")
	}
	if ref, ok := schema["$ref"].(string); ok {
		if !strings.HasPrefix(ref, "#/") {
			return false, fmt.Errorf("only local schema references supported")
		}
		var target any = root
		for _, part := range strings.Split(ref[2:], "/") {
			object, ok := target.(map[string]any)
			if !ok {
				return false, fmt.Errorf("invalid reference")
			}
			target = object[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
		}
		resolved, ok := target.(map[string]any)
		if !ok {
			return false, fmt.Errorf("reference missing")
		}
		return schemaValid(root, resolved, value, depth+1)
	}
	if nullable, _ := schema["nullable"].(bool); nullable && value == nil {
		return true, nil
	}
	for _, kind := range []string{"allOf", "anyOf", "oneOf"} {
		if branches, ok := schema[kind].([]any); ok {
			matches := 0
			for _, branch := range branches {
				child, ok := branch.(map[string]any)
				if !ok {
					return false, fmt.Errorf("invalid schema branch")
				}
				valid, err := schemaValid(root, child, value, depth+1)
				if err != nil {
					return false, err
				}
				if valid {
					matches++
				}
			}
			if kind == "allOf" && matches != len(branches) || kind == "anyOf" && matches == 0 || kind == "oneOf" && matches != 1 {
				return false, nil
			}
		}
	}
	if choices, ok := schema["enum"].([]any); ok {
		match := false
		for _, choice := range choices {
			if string(mustJSON(choice)) == string(mustJSON(value)) {
				match = true
			}
		}
		if !match {
			return false, nil
		}
	}
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return false, nil
		}
		if required, ok := schema["required"].([]any); ok {
			for _, key := range required {
				name, ok := key.(string)
				if !ok {
					return false, fmt.Errorf("invalid required schema name")
				}
				if _, ok := object[name]; !ok {
					return false, nil
				}
			}
		}
		props, _ := schema["properties"].(map[string]any)
		for key, v := range object {
			if child, ok := props[key].(map[string]any); ok {
				valid, err := schemaValid(root, child, v, depth+1)
				if err != nil || !valid {
					return valid, err
				}
			} else if extra, ok := schema["additionalProperties"].(bool); ok && !extra {
				return false, nil
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return false, nil
		}
		if n, ok := schema["minItems"].(float64); ok && float64(len(array)) < n {
			return false, nil
		}
		if n, ok := schema["maxItems"].(float64); ok && float64(len(array)) > n {
			return false, nil
		}
		if child, ok := schema["items"].(map[string]any); ok {
			for _, item := range array {
				valid, err := schemaValid(root, child, item, depth+1)
				if err != nil || !valid {
					return valid, err
				}
			}
		}
	case "string":
		s, ok := value.(string)
		if !ok {
			return false, nil
		}
		if n, ok := schema["minLength"].(float64); ok && float64(utf8.RuneCountInString(s)) < n {
			return false, nil
		}
		if n, ok := schema["maxLength"].(float64); ok && float64(utf8.RuneCountInString(s)) > n {
			return false, nil
		}
		if pattern, ok := schema["pattern"].(string); ok {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false, err
			}
			if !re.MatchString(s) {
				return false, nil
			}
		}
	case "integer", "number":
		n, ok := value.(float64)
		if !ok {
			return false, nil
		}
		if schema["type"] == "integer" && math.Trunc(n) != n {
			return false, nil
		}
		if min, ok := schema["minimum"].(float64); ok && n < min {
			return false, nil
		}
		if max, ok := schema["maximum"].(float64); ok && n > max {
			return false, nil
		}
		if flag, ok := schema["exclusiveMinimum"].(bool); ok && flag {
			if min, ok := schema["minimum"].(float64); ok && n <= min {
				return false, nil
			}
		}
		if flag, ok := schema["exclusiveMaximum"].(bool); ok && flag {
			if max, ok := schema["maximum"].(float64); ok && n >= max {
				return false, nil
			}
		}
		if min, ok := schema["exclusiveMinimum"].(float64); ok && n <= min {
			return false, nil
		}
		if max, ok := schema["exclusiveMaximum"].(float64); ok && n >= max {
			return false, nil
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return false, nil
		}
	case "null":
		if value != nil {
			return false, nil
		}
	case nil:
	default:
		return false, fmt.Errorf("unsupported schema type")
	}
	return true, nil
}

func timeSeconds(n int) time.Duration { return time.Duration(n) * time.Second }
