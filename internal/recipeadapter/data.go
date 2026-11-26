package recipeadapter

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type table struct {
	Fields []string
	Rows   []map[string]string
}

func readTable(path string) (table, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return table{}, err
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		var rows []map[string]any
		if err = json.Unmarshal(b, &rows); err != nil {
			return table{}, err
		}
		fields := map[string]bool{}
		out := table{}
		for _, row := range rows {
			converted := map[string]string{}
			for key, value := range row {
				fields[key] = true
				if value == nil {
					converted[key] = ""
				} else if s, ok := value.(string); ok {
					converted[key] = s
				} else {
					converted[key] = strings.TrimSpace(string(mustJSON(value)))
				}
			}
			out.Rows = append(out.Rows, converted)
		}
		for key := range fields {
			out.Fields = append(out.Fields, key)
		}
		sort.Strings(out.Fields)
		return out, nil
	}
	r := csv.NewReader(strings.NewReader(string(b)))
	fields, err := r.Read()
	if err != nil {
		return table{}, err
	}
	seen := map[string]bool{}
	for _, f := range fields {
		if seen[f] {
			return table{}, fmt.Errorf("duplicate table column")
		}
		seen[f] = true
	}
	out := table{Fields: fields}
	for {
		values, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
		row := map[string]string{}
		for i, value := range values {
			row[fields[i]] = value
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}
func loadSourceOutput(c Context) (table, table, error) {
	source, err := c.Path(c.String("source"))
	if err != nil {
		return table{}, table{}, err
	}
	key := "output"
	if c.Recipe == "etl-reconciliation" {
		key = "target"
	}
	output, err := c.Path(c.String(key))
	if err != nil {
		return table{}, table{}, err
	}
	a, err := readTable(source)
	if err != nil {
		return a, table{}, err
	}
	b, err := readTable(output)
	return a, b, err
}

type fieldRange struct {
	Field string   `json:"field"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

func cleaning(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"CSV/JSON acceptance validator"}}
	source, output, err := loadSourceOutput(c)
	if err != nil {
		return m, err
	}
	m.Metric("source_rows", float64(len(source.Rows)))
	m.Metric("output_rows", float64(len(output.Rows)))
	m.Add("row-count", len(output.Rows) <= len(source.Rows) && len(source.Rows)-len(output.Rows) <= int(c.Number("max_dropped_rows")), "explicit permitted row-loss bound")
	sourceIDs := map[string]bool{}
	for _, row := range source.Rows {
		sourceIDs[row[c.String("id_field")]] = true
	}
	seen := map[string]bool{}
	invalid := 0
	for i, row := range output.Rows {
		id := row[c.String("id_field")]
		m.Add(fmt.Sprintf("id:%d", i), id != "" && !seen[id] && sourceIDs[id], "unique source-derived row identity")
		seen[id] = true
		for _, field := range c.Strings("required_fields") {
			m.Add(fmt.Sprintf("required:%d:%s", i, field), strings.TrimSpace(row[field]) != "", "required value present")
		}
		for _, fieldSpec := range c.Strings("date_fields") {
			field, layout, ok := strings.Cut(fieldSpec, ":")
			if !ok {
				layout = "2006-01-02"
			}
			_, err := time.Parse(layout, row[field])
			m.Add(fmt.Sprintf("date:%d:%s", i, field), err == nil, "date parses with the declared layout")
			if err != nil {
				invalid++
			}
		}
	}
	var ranges []fieldRange
	if err := c.Option("ranges", &ranges); err != nil {
		return m, err
	}
	// Catalog range strings use field:min:max, with either end optionally empty.
	for _, spec := range c.Strings("ranges") {
		parts := strings.Split(spec, ":")
		if len(parts) != 3 {
			return m, fmt.Errorf("range syntax is field:min:max")
		}
		r := fieldRange{Field: parts[0]}
		if parts[1] != "" {
			n, e := parseNumber(parts[1])
			if e != nil {
				return m, e
			}
			r.Min = &n
		}
		if parts[2] != "" {
			n, e := parseNumber(parts[2])
			if e != nil {
				return m, e
			}
			r.Max = &n
		}
		ranges = append(ranges, r)
	}
	for i, row := range output.Rows {
		for _, r := range ranges {
			n, err := parseNumber(row[r.Field])
			pass := err == nil && (r.Min == nil || n >= *r.Min) && (r.Max == nil || n <= *r.Max)
			m.Add(fmt.Sprintf("range:%d:%s", i, r.Field), pass, "declared value range")
		}
	}
	m.Metric("invalid_dates", float64(invalid))
	m.Metric("violations", float64(len(m.Failures())))
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, "row-lineage.json"), mustJSON(output.Rows), 0600); err != nil {
		return m, err
	}
	return m, nil
}
func canonicalRows(rows []map[string]string, rename map[string]string) []string {
	out := []string{}
	for _, row := range rows {
		normalized := map[string]string{}
		for key, value := range row {
			if to := rename[key]; to != "" {
				key = to
			}
			normalized[key] = value
		}
		out = append(out, strings.TrimSpace(string(mustJSON(normalized))))
	}
	sort.Strings(out)
	return out
}
func reconciliation(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"exact decimal reconciliation"}}
	source, target, err := loadSourceOutput(c)
	if err != nil {
		return m, err
	}
	var rename map[string]string
	if err := c.Option("field_map", &rename); err != nil {
		return m, err
	}
	a, b := canonicalRows(source.Rows, rename), canonicalRows(target.Rows, nil)
	m.Add("row-count", len(a) == len(b), "source/target counts")
	m.Add("canonical-row-hash", hashBytes(mustJSON(a)) == hashBytes(mustJSON(b)), "unordered rows retain duplicates and exact field values")
	for _, key := range c.Strings("reconciliation_keys") {
		sourceKey := key
		for from, to := range rename {
			if to == key {
				sourceKey = from
			}
		}
		counts := func(rows []map[string]string, field string) map[string]int {
			m := map[string]int{}
			for _, row := range rows {
				m[row[field]]++
			}
			return m
		}
		m.Add("key-cardinality:"+key, string(mustJSON(counts(source.Rows, sourceKey))) == string(mustJSON(counts(target.Rows, key))), "per-key cardinality")
	}
	for _, spec := range c.Strings("totals") {
		field, toleranceText, _ := strings.Cut(spec, ":")
		tolerance := new(big.Rat)
		if toleranceText != "" {
			if _, ok := tolerance.SetString(toleranceText); !ok || tolerance.Sign() < 0 {
				return m, fmt.Errorf("invalid reconciliation tolerance")
			}
		}
		sourceField := field
		for from, to := range rename {
			if to == field {
				sourceField = from
			}
		}
		sum := func(rows []map[string]string, key string) (*big.Rat, error) {
			total := new(big.Rat)
			for _, row := range rows {
				v, ok := new(big.Rat).SetString(row[key])
				if !ok {
					return nil, fmt.Errorf("nonnumeric total field")
				}
				total.Add(total, v)
			}
			return total, nil
		}
		x, err := sum(source.Rows, sourceField)
		if err != nil {
			return m, err
		}
		y, err := sum(target.Rows, field)
		if err != nil {
			return m, err
		}
		difference := new(big.Rat).Sub(x, y)
		difference.Abs(difference)
		m.Add("total:"+field, difference.Cmp(tolerance) <= 0, "exact rational arithmetic with declared tolerance")
	}
	m.Metric("discrepancies", float64(len(m.Failures())))
	return m, nil
}
