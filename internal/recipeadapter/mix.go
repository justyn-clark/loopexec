package recipeadapter

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func mixMeasurements(ctx context.Context, c Context, m *Measurement, path string) error {
	info, err := probeAudio(ctx, c, path)
	if err != nil {
		return err
	}
	if info.Channels != 2 {
		return fmt.Errorf("mix phase/balance analysis requires stereo audio")
	}
	raw := filepath.Join(c.EvidenceDir, "mix.f32")
	r, err := c.Run(ctx, c.Root, c.Tool("ffmpeg"), "-v", "error", "-y", "-i", path, "-f", "f32le", "-acodec", "pcm_f32le", "-ac", "2", raw)
	if err != nil || r.ExitCode != 0 {
		return fmt.Errorf("mix PCM decode failed")
	}
	stat, err := os.Stat(raw)
	if err != nil {
		return err
	}
	if stat.Size() > 256<<20 {
		return fmt.Errorf("decoded audio exceeds 256 MiB analysis bound")
	}
	data, err := os.ReadFile(raw)
	if err != nil {
		return err
	}
	if len(data) > 256<<20 {
		return fmt.Errorf("decoded audio exceeds 256 MiB analysis bound")
	}
	if len(data)%8 != 0 || len(data) == 0 {
		return fmt.Errorf("invalid stereo PCM")
	}
	sumL, sumR, sumLR := 0.0, 0.0, 0.0
	silent, maxSilent := 0, 0
	threshold := 0.0001
	for i := 0; i < len(data); i += 8 {
		l := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[i : i+4])))
		right := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[i+4 : i+8])))
		if math.IsNaN(l) || math.IsInf(l, 0) || math.IsNaN(right) || math.IsInf(right, 0) {
			return fmt.Errorf("invalid decoded sample")
		}
		sumL += l * l
		sumR += right * right
		sumLR += l * right
		if math.Abs(l) < threshold && math.Abs(right) < threshold {
			silent++
			maxSilent = max(maxSilent, silent)
		} else {
			silent = 0
		}
	}
	correlation := 0.0
	if sumL > 0 && sumR > 0 {
		correlation = sumLR / math.Sqrt(sumL*sumR)
	}
	balance := 120.0
	if sumL > 0 && sumR > 0 {
		balance = math.Abs(10 * math.Log10(sumL/sumR))
	}
	silence := float64(maxSilent) / float64(info.SampleRate)
	m.Metric("phase_correlation", correlation)
	m.Metric("channel_imbalance_db", balance)
	m.Metric("max_silence_seconds", silence)
	m.Add("phase-correlation", correlation >= c.Number("min_phase_correlation"), "actual PCM channel cross-correlation")
	m.Add("channel-balance", balance <= c.Number("max_channel_imbalance_db"), "channel RMS difference in dB")
	m.Add("silence", silence <= c.Number("max_silence_seconds"), "longest near-silent PCM run")
	stems, err := c.Path(c.String("stems"))
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(stems)
	if err != nil {
		return err
	}
	var expected []string
	if err := c.Option("expected_stems", &expected); err != nil {
		return err
	}
	if len(expected) == 0 {
		return fmt.Errorf("expected_stems inventory is required")
	}
	actual := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			actual[entry.Name()] = true
		}
	}
	for _, stem := range expected {
		if filepath.Base(stem) != stem {
			return fmt.Errorf("invalid stem identity")
		}
		m.Add("stem-present:"+stem, actual[stem], "frozen stem inventory")
		if !actual[stem] {
			continue
		}
		stemInfo, err := probeAudio(ctx, c, filepath.Join(stems, stem))
		if err != nil {
			return err
		}
		m.Add("stem-duration:"+stem, math.Abs(stemInfo.Duration-info.Duration) < 0.02, "stem/mix duration alignment")
		m.Add("stem-sample-rate:"+stem, stemInfo.SampleRate == info.SampleRate, "matching sample rate")
	}

	return nil
}
