package recipeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type loudness struct {
	I         string `json:"input_i"`
	TP        string `json:"input_tp"`
	LRA       string `json:"input_lra"`
	Threshold string `json:"input_thresh"`
	Offset    string `json:"target_offset"`
}

func finiteNumber(s string) (float64, error) {
	n, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("nonfinite tool measurement")
	}
	return n, nil
}
func analyzeLoudness(ctx context.Context, c Context, path string, target, peak float64) (loudness, error) {
	filter := fmt.Sprintf("loudnorm=I=%g:TP=%g:LRA=11:print_format=json", target, peak)
	r, err := c.Run(ctx, c.Root, c.Tool("ffmpeg"), "-hide_banner", "-nostats", "-i", path, "-af", filter, "-f", "null", "-")
	if err != nil || r.ExitCode != 0 {
		return loudness{}, fmt.Errorf("ffmpeg loudness analysis failed")
	}
	if e := os.WriteFile(filepath.Join(c.EvidenceDir, "loudness.txt"), []byte(r.Stderr), 0600); e != nil {
		return loudness{}, e
	}
	start := strings.LastIndex(r.Stderr, "{")
	end := strings.LastIndex(r.Stderr, "}")
	if start < 0 || end < start {
		return loudness{}, fmt.Errorf("missing loudness measurements")
	}
	var measured loudness
	if err = json.Unmarshal([]byte(r.Stderr[start:end+1]), &measured); err != nil {
		return measured, err
	}
	return measured, nil
}
func NormalizeAudio(ctx context.Context, c Context) error {
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
	owned := false
	for _, p := range c.Adapter.Artifacts {
		full, e := c.Path(p)
		if e == nil && (full == output || strings.HasPrefix(output, full+string(filepath.Separator))) {
			owned = true
		}
	}
	if !owned {
		return fmt.Errorf("audio output is outside declared artifacts")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	first := c
	first.EvidenceDir = filepath.Join(c.EvidenceDir, "source-analysis")
	if err := os.MkdirAll(first.EvidenceDir, 0700); err != nil {
		return err
	}
	measured, err := analyzeLoudness(ctx, first, source, c.Number("target_lufs"), c.Number("max_true_peak_dbtp"))
	if err != nil {
		return err
	}
	for _, value := range []string{measured.I, measured.TP, measured.LRA, measured.Threshold, measured.Offset} {
		if _, err := finiteNumber(value); err != nil {
			return err
		}
	}
	filter := fmt.Sprintf("loudnorm=I=%g:TP=%g:LRA=11:measured_I=%s:measured_TP=%s:measured_LRA=%s:measured_thresh=%s:offset=%s:linear=true", c.Number("target_lufs"), c.Number("max_true_peak_dbtp"), measured.I, measured.TP, measured.LRA, measured.Threshold, measured.Offset)
	r, err := c.Run(ctx, c.Root, c.Tool("ffmpeg"), "-hide_banner", "-nostats", "-y", "-i", source, "-af", filter, "-ar", "48000", output)
	if e := os.WriteFile(filepath.Join(c.EvidenceDir, "normalization.txt"), []byte(r.Stderr), 0600); e != nil {
		return e
	}
	if err != nil || r.ExitCode != 0 {
		return fmt.Errorf("ffmpeg normalization failed")
	}
	return nil
}
func audio(ctx context.Context, c Context) (Measurement, error) {
	m := Measurement{Tools: []string{"ffmpeg", "ffprobe"}}
	key := "output"
	if c.Recipe == "music-mix" {
		key = "render"
	}
	file, err := c.Path(c.String(key))
	if err != nil {
		return m, err
	}
	if _, e := os.Stat(file); os.IsNotExist(e) {
		m.Add("audio-output-exists", false, "audio output is missing")
		return m, nil
	}
	target, peak := c.Number("target_lufs"), c.Number("max_true_peak_dbtp")
	if c.Recipe == "music-mix" {
		target = -14
	}
	measured, err := analyzeLoudness(ctx, c, file, target, peak)
	if err != nil {
		return m, err
	}
	integrated, err := finiteNumber(measured.I)
	if err != nil {
		return m, err
	}
	tp, err := finiteNumber(measured.TP)
	if err != nil {
		return m, err
	}
	lra, err := finiteNumber(measured.LRA)
	if err != nil {
		return m, err
	}
	m.Metric("integrated_lufs", integrated)
	m.Metric("true_peak_dbtp", tp)
	m.Metric("loudness_range_lu", lra)
	m.Add("true-peak-ceiling", tp <= peak+0.01, fmt.Sprintf("%g dBTP <= %g", tp, peak))
	if c.Recipe == "audio-mastering" {
		m.Add("integrated-loudness", math.Abs(integrated-target) <= c.Number("lufs_tolerance"), fmt.Sprintf("%g LUFS, target %g", integrated, target))
		m.Add("minimum-loudness-range", lra >= c.Number("min_dynamic_range"), "dynamic-range contract uses EBU loudness range in LU")
		source, err := c.Path(c.String("source"))
		if err != nil {
			return m, err
		}
		sourceInfo, err := probeAudio(ctx, c, source)
		if err != nil {
			return m, err
		}
		resultInfo, err := probeAudio(ctx, c, file)
		if err != nil {
			return m, err
		}
		m.Add("duration-preserved", math.Abs(sourceInfo.Duration-resultInfo.Duration) < 0.02, "decoded input/output duration")
		m.Add("channels-preserved", sourceInfo.Channels == resultInfo.Channels, "input/output channel count")
	}
	if c.Recipe == "music-mix" {
		if err := mixMeasurements(ctx, c, &m, file); err != nil {
			return m, err
		}
	}
	return m, nil
}

type audioInfo struct {
	Duration   float64
	Channels   int
	SampleRate int
}

func probeAudio(ctx context.Context, c Context, path string) (audioInfo, error) {
	var report struct {
		Streams []struct {
			Channels   int    `json:"channels"`
			SampleRate string `json:"sample_rate"`
			Duration   string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	r, err := c.Run(ctx, c.Root, c.Tool("ffprobe"), "-v", "error", "-select_streams", "a:0", "-show_streams", "-show_format", "-of", "json", path)
	if err != nil || r.ExitCode != 0 {
		return audioInfo{}, fmt.Errorf("ffprobe failed")
	}
	if err = json.Unmarshal([]byte(r.Stdout), &report); err != nil || len(report.Streams) != 1 {
		return audioInfo{}, fmt.Errorf("expected one analyzable audio stream")
	}
	duration, err := finiteNumber(report.Format.Duration)
	if err != nil {
		return audioInfo{}, err
	}
	rate, err := strconv.Atoi(report.Streams[0].SampleRate)
	if err != nil {
		return audioInfo{}, err
	}
	return audioInfo{duration, report.Streams[0].Channels, rate}, nil
}
