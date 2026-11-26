package recipeadapter

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
)

//go:embed browser.mjs
var browserScript []byte

func BrowserCapture(ctx context.Context, c Context) (Measurement, error) {
	script := filepath.Join(c.EvidenceDir, "browser.mjs")
	if err := os.WriteFile(script, browserScript, 0600); err != nil {
		return Measurement{}, err
	}
	input := filepath.Join(c.EvidenceDir, "browser-request.json")
	data, err := json.Marshal(c)
	if err != nil {
		return Measurement{}, err
	}
	if err = os.WriteFile(input, data, 0600); err != nil {
		return Measurement{}, err
	}
	r, err := c.Run(ctx, c.Root, c.Tool("node"), script, input)
	if err != nil || r.ExitCode != 0 {
		os.WriteFile(filepath.Join(c.EvidenceDir, "browser-error.txt"), []byte(r.Stderr), 0600)
		return Measurement{}, fmt.Errorf("browser measurement failed; inspect private browser-error.txt")
	}
	var measured Measurement
	if err = json.Unmarshal([]byte(r.Stdout), &measured); err != nil {
		return measured, err
	}
	if len(measured.Checks) == 0 {
		return measured, fmt.Errorf("browser emitted no checks")
	}
	return measured, nil
}
func browser(ctx context.Context, c Context) (Measurement, error) {
	m, err := BrowserCapture(ctx, c)
	if err != nil {
		return m, err
	}
	if c.Recipe == "frontend-visual" {
		reference, err := c.Path(c.String("reference"))
		if err != nil {
			return m, err
		}
		actual := filepath.Join(c.EvidenceDir, "page-000.png")
		difference, err := pixelDifference(reference, actual)
		if err != nil {
			return m, err
		}
		m.Metric("visual_difference", difference)
		m.Add("visual-reference", difference <= c.Number("max_visual_difference"), fmt.Sprintf("changed pixel ratio %g", difference))
	}
	return m, nil
}
func pixelDifference(a, b string) (float64, error) {
	load := func(p string) (image.Image, error) {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		return img, err
	}
	x, err := load(a)
	if err != nil {
		return 0, err
	}
	y, err := load(b)
	if err != nil {
		return 0, err
	}
	if x.Bounds().Dx() != y.Bounds().Dx() || x.Bounds().Dy() != y.Bounds().Dy() {
		return 1, nil
	}
	changed, total := 0, 0
	for j := 0; j < x.Bounds().Dy(); j++ {
		for i := 0; i < x.Bounds().Dx(); i++ {
			a, b, c, d := x.At(i, j).RGBA()
			e, f, g, h := y.At(i, j).RGBA()
			if math.Abs(float64(a)-float64(e)) > 256 || math.Abs(float64(b)-float64(f)) > 256 || math.Abs(float64(c)-float64(g)) > 256 || d != h {
				changed++
			}
			total++
		}
	}
	if total == 0 {
		return 0, fmt.Errorf("empty comparison image")
	}
	return float64(changed) / float64(total), nil
}
