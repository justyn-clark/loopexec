package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/justyn-clark/loopexec/internal/workflow"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justyn-clark/loopexec/internal/subprocess"
)

type commandRunner struct {
	ctx            context.Context
	dir            string
	timeout, grace time.Duration
	receipt        *receiptWriter
}

func (r commandRunner) run(iter int, phase, command string) subprocess.Result {
	r.receipt.command(iter, "command_start", phase, subprocess.Result{})
	result := subprocess.Run(r.ctx, subprocess.Options{Dir: r.dir, Argv: []string{"sh", "-c", command}, Timeout: r.timeout, Grace: r.grace})
	event := "command_end"
	if result.Cause == "deadline_exceeded" || result.Cause == "pipe_timeout" {
		event = "command_timeout"
	}
	if result.Cause == "cancelled" {
		event = "command_cancelled"
	}
	r.receipt.command(iter, event, phase, result)
	return result
}
func (w *receiptWriter) command(iter int, event, phase string, r subprocess.Result) {
	if w == nil || w.f == nil {
		return
	}
	ev := receiptEvent{OutputTruncated: r.Truncated, TS: nowFunc().Unix(), RunID: w.runID, Iteration: iter, Event: event, Phase: phase, DurationMS: r.Duration.Milliseconds(), Cause: r.Cause}
	if event != "command_start" {
		ev.ExitCode = &r.ExitCode
		if ev.Cause == "" {
			ev.Cause = "completed"
			if r.ExitCode != 0 {
				ev.Cause = "nonzero_exit"
			}
		}

	}
	line, _ := json.Marshal(ev)
	if _, e := w.f.Write(append(line, '\n')); e != nil {
		w.err = e
	}
	if event != "command_start" {
		dir := filepath.Join(filepath.Dir(w.f.Name()), w.runID, "diagnostics")
		if e := os.MkdirAll(dir, 0700); e != nil {
			w.err = e
			return
		}
		for suffix, content := range map[string]string{"stdout": r.Stdout, "stderr": r.Stderr} {
			if len(content) > 16*1024 {
				content = content[len(content)-16*1024:]
			}
			name := filepath.Join(dir, fmt.Sprintf("%06d-%s.%s.tail", iter, phase, suffix))
			if e := os.WriteFile(name, []byte(content), 0600); e != nil {
				w.err = e
			}
		}
	}

}
func decodeStrict(data string, target any) error {
	return workflow.Decode([]byte(data), target)
}

// Legacy identity lines remain valid, including genuinely empty stdout.
// JSON is opt-in; malformed JSON-looking output cannot become identities.
func collectorSet(r subprocess.Result) (map[string]struct{}, []string, error) {
	if r.ExitCode != 0 || r.Cause != "" || r.Truncated || strings.TrimSpace(r.Stderr) != "" {
		return nil, nil, fmt.Errorf("collector_failed")
	}
	s := strings.TrimSpace(r.Stdout)
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		var v struct {
			IDs []string `json:"ids"`
		}
		if err := decodeStrict(s, &v); err != nil || v.IDs == nil {
			return nil, nil, fmt.Errorf("collector_invalid")
		}
		for _, id := range v.IDs {
			if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\r\n") {
				return nil, nil, fmt.Errorf("collector_invalid")
			}
		}
		s = strings.Join(v.IDs, "\n")
	}
	set, order := parseFailures(s)
	return set, order, nil
}

func causeOfContext(ctx context.Context) string {
	if ctx.Err() == context.DeadlineExceeded {
		return "deadline_exceeded"
	}
	return "cancelled"
}
func readBounded(r io.Reader, limit int) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, int64(limit+1)))
	if e != nil {
		return nil, e
	}
	if len(b) > limit {
		return nil, fmt.Errorf("evidence oversized")
	}
	return b, nil
}
