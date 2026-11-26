package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func batch(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"file inventory/format validators"}}
	source, err := c.Path(c.String("source"))
	if err != nil {
		return m, err
	}
	output, err := c.Path(c.String("output"))
	if err != nil {
		return m, err
	}
	var names map[string]string
	if err := c.Option("names", &names); err != nil {
		return m, err
	}
	var checksums map[string]string
	if err := c.Option("checksums", &checksums); err != nil {
		return m, err
	}
	count := 0
	destinations := map[string]bool{}
	err = filepath.WalkDir(source, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink rejected")
		}
		if count >= 10000 {
			return fmt.Errorf("batch inventory too large")
		}
		count++
		rel, e := filepath.Rel(source, path)
		if e != nil {
			return e
		}
		name := strings.TrimSuffix(rel, filepath.Ext(rel)) + "." + strings.TrimPrefix(c.String("format"), ".")
		if replacement := names[filepath.ToSlash(rel)]; replacement != "" {
			name = replacement
		}
		dest := filepath.Join(output, name)
		if !within(output, dest) {
			return fmt.Errorf("batch destination escapes output root")
		}
		m.Add("output-collision:"+rel, !destinations[dest], "one output per source item")
		destinations[dest] = true
		data, e := os.ReadFile(dest)
		if os.IsNotExist(e) {
			m.Add("output:"+rel, false, "missing output")
			return nil
		}
		if e != nil {
			return e
		}
		m.Add("output:"+rel, true, "source item has an output")
		for _, rule := range c.Strings("validation_rules") {
			kind, value, _ := strings.Cut(rule, ":")
			pass := false
			switch kind {
			case "utf8":
				pass = utf8.Valid(data)
			case "json":
				var v any
				pass = json.Unmarshal(data, &v) == nil
			case "name":
				re, e := regexp.Compile(value)
				if e != nil {
					return e
				}
				pass = re.MatchString(filepath.Base(name))
			case "checksum":
				want, ok := checksums[filepath.ToSlash(name)]
				if !ok {
					return fmt.Errorf("checksum contract missing output hash")
				}
				pass = hashBytes(data) == want
			case "width", "height":
				f, e := os.Open(dest)
				if e != nil {
					return e
				}
				config, _, e := image.DecodeConfig(f)
				f.Close()
				if e != nil {
					return e
				}
				expected, e := strconv.Atoi(value)
				if e != nil {
					return e
				}
				actual := config.Width
				if kind == "height" {
					actual = config.Height
				}
				pass = actual == expected
			case "metadata":
				key, want, ok := strings.Cut(value, "=")
				if !ok {
					return fmt.Errorf("metadata rule syntax is metadata:key=value")
				}
				r, e := c.Run(ctx, c.Root, c.Tool("ffprobe"), "-v", "error", "-show_entries", "format_tags", "-of", "json", dest)
				if e != nil || r.ExitCode != 0 {
					return fmt.Errorf("metadata probe failed")
				}
				var result struct {
					Format struct {
						Tags map[string]string `json:"tags"`
					} `json:"format"`
				}
				if e = json.Unmarshal([]byte(r.Stdout), &result); e != nil {
					return e
				}
				pass = result.Format.Tags[key] == want
			default:
				return fmt.Errorf("unsupported batch rule: %s", kind)
			}
			m.Add("rule:"+rel+":"+rule, pass, "actual output validator")
		}
		return nil
	})
	if err != nil {
		return m, err
	}
	if count == 0 {
		return m, fmt.Errorf("empty source inventory")
	}
	m.Metric("source_files", float64(count))
	m.Metric("invalid_rules", float64(len(m.Failures())))
	return m, nil
}
func within(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
