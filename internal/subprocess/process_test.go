package subprocess

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBoundedOutput(t *testing.T) {
	r := Run(context.Background(), Options{Argv: []string{"sh", "-c", "i=0; while [ $i -lt 500 ]; do printf 'abcdefghij'; printf 'error' >&2; i=$((i+1)); done"}, Limit: 100})
	if r.ExitCode != 0 || !r.Truncated || len(r.Stdout) != 100 || strings.Contains(r.Stdout, "error") || len(r.Stderr) != 100 {
		t.Fatalf("%+v", r)
	}
}
func TestAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Run(ctx, Options{Argv: []string{"sh", "-c", "exit 0"}})
	if r.Cause != "cancelled" || r.ExitCode == 0 {
		t.Fatalf("%+v", r)
	}
}
func TestDeadline(t *testing.T) {
	r := Run(context.Background(), Options{Argv: []string{"sh", "-c", "sleep 30"}, Timeout: 50 * time.Millisecond, Grace: 20 * time.Millisecond})
	if r.Cause != "deadline_exceeded" || r.Duration > time.Second {
		t.Fatalf("%+v", r)
	}
}
