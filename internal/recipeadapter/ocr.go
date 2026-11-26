package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func ocr(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"tesseract"}}
	if c.String("ocr_engine") != "tesseract" {
		return m, fmt.Errorf("native OCR adapter supports ocr_engine=tesseract; bind another engine explicitly")
	}
	var imagePath string
	if err := c.Option("image", &imagePath); err != nil {
		return m, err
	}
	if imagePath == "" {
		imagePath = c.String("source")
	}
	path, err := c.Path(imagePath)
	if err != nil {
		return m, err
	}
	r, err := c.Run(ctx, c.Root, c.Tool("tesseract"), path, "stdout", "--psm", "6")
	if err != nil || r.ExitCode != 0 {
		return m, fmt.Errorf("OCR engine failed")
	}
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, "ocr.txt"), []byte(r.Stdout), 0600); err != nil {
		return m, err
	}
	truthPath, err := c.Path(c.String("truth"))
	if err != nil {
		return m, err
	}
	data, err := os.ReadFile(truthPath)
	if err != nil {
		return m, err
	}
	var truth map[string]string
	if err = json.Unmarshal(data, &truth); err != nil {
		return m, err
	}
	var patterns map[string]string
	if err := c.Option("patterns", &patterns); err != nil {
		return m, err
	}
	correct := 0
	fields := c.Strings("required_fields")
	actual := map[string]string{}
	for _, field := range fields {
		pattern, ok := patterns[field]
		if !ok {
			return m, fmt.Errorf("missing field extraction pattern")
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return m, err
		}
		match := re.FindStringSubmatch(r.Stdout)
		value := ""
		if len(match) > 1 {
			value = strings.TrimSpace(match[1])
		}
		actual[field] = value
		pass := value != "" && value == truth[field]
		if pass {
			correct++
		}
		m.Add("field:"+field, value != "", "extracted field compared with frozen ground truth")
	}
	accuracy := float64(correct) / float64(len(fields))
	m.Metric("field_accuracy", accuracy)
	m.Add("accuracy-target", accuracy >= c.Number("min_field_accuracy"), "known-field comparison, not self-reported confidence")
	if err := os.WriteFile(filepath.Join(c.EvidenceDir, "fields.json"), mustJSON(actual), 0600); err != nil {
		return m, err
	}
	return m, nil
}
