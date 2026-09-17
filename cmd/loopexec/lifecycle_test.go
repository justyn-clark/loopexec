package main

import (
	"context"
	"github.com/justyn-clark/loopexec/internal/subprocess"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectorsFailClosed(t *testing.T) {
	for _, v := range []subprocess.Result{
		{ExitCode: 1, Stdout: ""},
		{ExitCode: 0, Stderr: "collector error"},
		{ExitCode: 0, Stdout: "{invalid"},
		{ExitCode: 0, Stdout: `{"ids":null}`},
		{ExitCode: 0, Stdout: `{"ids":[],"extra":true}`},
		{ExitCode: 0, Stdout: "a", Truncated: true},
		{ExitCode: 0, Cause: "deadline_exceeded"},
	} {
		if _, _, e := collectorSet(v); e == nil {
			t.Fatalf("accepted %+v", v)
		}
	}
	for _, s := range []string{"", `{"ids":[]}`, "A\nA\nB"} {
		set, _, e := collectorSet(subprocess.Result{Stdout: s})
		if e != nil {
			t.Fatal(e)
		}
		if _, ok := set["must not become identity"]; ok {
			t.Fatal(set)
		}
	}
}
func runForTest(t *testing.T, cfg runConfig) (int, loopState) {
	t.Helper()
	c := &cobra.Command{}
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	code := exitCode(executeRun(c, cfg))
	st, e := readState(statePath(cfg.workdir))
	if e != nil {
		t.Fatal(e)
	}
	return code, st
}
func TestHookDeadlinesLeaveFinalState(t *testing.T) {
	for _, phase := range []string{"exec", "check", "integrity", "failures"} {
		t.Run(phase, func(t *testing.T) {
			cfg := runConfig{runID: "timeout", workdir: t.TempDir(), check: "true", maxIterations: 2, commandTimeout: 30 * time.Millisecond, grace: 10 * time.Millisecond, totalTimeout: time.Second}
			switch phase {
			case "exec":
				cfg.execCmd = "sleep 30"
			case "check":
				cfg.check = "sleep 30"
			case "integrity":
				cfg.integrityCmd = "sleep 30"
			case "failures":
				cfg.failuresCmd = "sleep 30"
			}
			code, st := runForTest(t, cfg)
			if code == 10 || st.Phase != "halted" {
				t.Fatalf("%d %+v", code, st)
			}
			b, e := os.ReadFile(filepath.Join(cfg.workdir, ".loopexec", "run-timeout.jsonl"))
			if e != nil || !strings.Contains(string(b), `"event":"command_timeout"`) || !strings.Contains(string(b), `"event":"halt"`) {
				t.Fatalf("%s %v", b, e)
			}
		})
	}
}
func TestControlledCancellationFinalState(t *testing.T) {
	cfg := runConfig{runID: "cancel", workdir: t.TempDir(), check: "sleep 30", maxIterations: 2, grace: 10 * time.Millisecond}
	c := &cobra.Command{}
	c.SetOut(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	c.SetContext(ctx)
	time.AfterFunc(30*time.Millisecond, cancel)
	defer cancel()
	if exitCode(executeRun(c, cfg)) != 40 {
		t.Fatal("cancellation did not fail")
	}
	st, e := readState(statePath(cfg.workdir))
	if e != nil || st.Phase != "halted" || st.FailureCause != "cancelled" {
		t.Fatalf("%+v %v", st, e)
	}
}
func TestCollectorGuardDominatesGreen(t *testing.T) {
	for _, s := range []string{"echo error >&2; exit 0", "echo error >&2; exit 2", "printf '{bad'", "yes x | head -c 1100000"} {
		cfg := runConfig{runID: "collector", workdir: t.TempDir(), check: "true", failuresCmd: s, maxIterations: 1}
		code, st := runForTest(t, cfg)
		if code != 13 || st.HaltReason != "objective_unverified" {
			t.Fatalf("%d %+v", code, st)
		}
	}
}
