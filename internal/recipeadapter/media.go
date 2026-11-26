package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type mediaInfo struct {
	Width, Height int
	Duration      float64
	Streams       int
}

func probeMedia(ctx context.Context, c Context, path string) (mediaInfo, error) {
	r, err := c.Run(ctx, c.Root, c.Tool("ffprobe"), "-v", "error", "-show_streams", "-show_format", "-of", "json", path)
	if err != nil || r.ExitCode != 0 {
		return mediaInfo{}, fmt.Errorf("media probe failed")
	}
	var report struct {
		Streams []struct {
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			CodecType string `json:"codec_type"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err = json.Unmarshal([]byte(r.Stdout), &report); err != nil {
		return mediaInfo{}, err
	}
	info := mediaInfo{Streams: len(report.Streams)}
	for _, s := range report.Streams {
		if s.CodecType == "video" && info.Width == 0 {
			info.Width = s.Width
			info.Height = s.Height
		}
	}
	if report.Format.Duration != "" {
		info.Duration, _ = strconv.ParseFloat(report.Format.Duration, 64)
	}
	if info.Width == 0 || info.Height == 0 {
		return info, fmt.Errorf("no video/image dimensions")
	}
	return info, nil
}
func media(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"ffmpeg", "ffprobe"}}
	source, err := c.Path(c.String("source"))
	if err != nil {
		return m, err
	}
	output, err := c.Path(c.String("output"))
	if err != nil {
		return m, err
	}
	stat, err := os.Stat(output)
	if err != nil {
		return m, err
	}
	a, err := probeMedia(ctx, c, source)
	if err != nil {
		return m, err
	}
	b, err := probeMedia(ctx, c, output)
	if err != nil {
		return m, err
	}
	m.Metric("output_bytes", float64(stat.Size()))
	m.Add("size-target", float64(stat.Size()) <= c.Number("max_bytes"), "actual output bytes")
	same := a.Width == b.Width && a.Height == b.Height
	m.Add("dimensions-preserved", same, "actual decoded dimensions")
	m.Add("stream-inventory", a.Streams == b.Streams, "input/output stream count")
	if a.Duration > 0 && b.Duration > 0 {
		m.Add("duration-preserved", abs(a.Duration-b.Duration) < 0.05, "container duration")
	}
	if !same {
		return m, nil
	}
	switch c.String("quality_metric") {
	case "ssim":
		r, err := c.Run(ctx, c.Root, c.Tool("ffmpeg"), "-hide_banner", "-nostats", "-i", source, "-i", output, "-lavfi", "[0:v][1:v]ssim", "-f", "null", "-")
		if err != nil || r.ExitCode != 0 {
			return m, fmt.Errorf("SSIM evaluation failed")
		}
		os.WriteFile(filepath.Join(c.EvidenceDir, "ssim.txt"), []byte(r.Stderr), 0600)
		matches := regexp.MustCompile(`All:([0-9.]+)`).FindAllStringSubmatch(r.Stderr, -1)
		if len(matches) == 0 {
			return m, fmt.Errorf("SSIM metric missing")
		}
		score, err := finiteNumber(matches[len(matches)-1][1])
		if err != nil {
			return m, err
		}
		m.Metric("ssim", score)
		m.Add("quality-floor", score >= c.Number("quality_floor"), "FFmpeg SSIM against immutable input")
	case "vmaf":
		log := filepath.Join(c.EvidenceDir, "vmaf.json")
		r, err := c.Run(ctx, c.Root, c.Tool("ffmpeg"), "-hide_banner", "-nostats", "-i", output, "-i", source, "-lavfi", "[0:v][1:v]libvmaf=log_fmt=json:log_path="+log, "-f", "null", "-")
		if err != nil || r.ExitCode != 0 {
			return m, fmt.Errorf("VMAF evaluator unavailable or failed")
		}
		var report struct {
			Pooled map[string]struct {
				Mean float64 `json:"mean"`
			} `json:"pooled_metrics"`
		}
		data, err := os.ReadFile(log)
		if err != nil {
			return m, err
		}
		if err = json.Unmarshal(data, &report); err != nil {
			return m, err
		}
		score, ok := report.Pooled["vmaf"]
		if !ok {
			return m, fmt.Errorf("VMAF metric missing")
		}
		m.Metric("vmaf", score.Mean)
		m.Add("quality-floor", score.Mean >= c.Number("quality_floor"), "FFmpeg VMAF")
	default:
		return m, fmt.Errorf("quality_metric must be ssim or vmaf")
	}
	return m, nil
}
func CompressMedia(ctx context.Context, c Context) error {
	source, err := c.Path(c.String("source"))
	if err != nil {
		return err
	}
	output, err := c.Path(c.String("output"))
	if err != nil {
		return err
	}
	if source == output {
		return fmt.Errorf("source/output collision")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	var steps []int
	if err := c.Option("compression_steps", &steps); err != nil {
		return err
	}
	owned := false
	for _, p := range c.Adapter.Artifacts {
		full, e := c.Path(p)
		if e == nil && (full == output || within(full, output)) {
			owned = true
		}
	}
	if !owned {
		return fmt.Errorf("media output is outside declared artifacts")
	}
	image := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true, ".tiff": true}[strings.ToLower(filepath.Ext(source))]
	if len(steps) == 0 {
		if image {
			steps = []int{2, 4, 8, 12}
		} else {
			steps = []int{18, 24, 28, 32}
		}
	}
	step := steps[min(max(0, c.Iteration-1), len(steps)-1)]
	argv := []string{c.Tool("ffmpeg"), "-hide_banner", "-nostats", "-y", "-i", source}
	if image {
		argv = append(argv, "-frames:v", "1", "-q:v", fmt.Sprint(step))
	} else {
		argv = append(argv, "-map", "0", "-c:v", "libx264", "-crf", fmt.Sprint(step), "-preset", "fast", "-c:a", "aac")
	}
	argv = append(argv, output)
	r, err := c.Run(ctx, c.Root, argv...)
	if err != nil || r.ExitCode != 0 {
		return fmt.Errorf("media encoder failed")
	}
	return nil
}
func abs(n float64) float64 {
	if n < 0 {
		return -n
	}
	return n
}
