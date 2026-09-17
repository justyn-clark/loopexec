//go:build darwin || linux

package subprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestOwnedParentAndChildCancelled(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() {
		done <- Run(ctx, Options{Dir: dir, Argv: []string{"sh", "-c", "trap '' TERM; sleep 30 & echo $! > child; echo $$ > parent; wait"}, Grace: 30 * time.Millisecond})
	}()
	deadline := time.After(2 * time.Second)
	var pids []int
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
ready:
	for {
		select {
		case <-deadline:
			t.Fatal("subprocess failed to signal readiness")
		case <-tick.C:
			a, e1 := os.ReadFile(filepath.Join(dir, "parent"))
			b, e2 := os.ReadFile(filepath.Join(dir, "child"))
			if e1 == nil && e2 == nil && len(a) > 0 && len(b) > 0 {
				for _, v := range []string{string(a), string(b)} {
					p, _ := strconv.Atoi(strings.TrimSpace(v))
					if p <= 1 {
						t.Fatal(v)
					}
					pids = append(pids, p)
				}
				break ready
			}
		}
	}
	cancelledAt := time.Now()
	cancel()
	r := <-done
	if r.Cause != "cancelled" || r.Duration > time.Second {
		t.Fatalf("%+v", r)
	}
	// SIGKILL delivery to descendants is asynchronous, and they can disappear
	// between kill(0) and reading /proc. Still require every owned process to
	// stop within one second of cancellation, including Run's cleanup time.
	// Linux init can briefly retain a dead adopted descendant as a zombie.
	for _, pid := range pids {
		for {
			err := syscall.Kill(pid, 0)
			if errors.Is(err, syscall.ESRCH) {
				break
			}
			if err != nil {
				t.Fatalf("inspect owned pid %d: %v", pid, err)
			}
			if runtime.GOOS == "linux" {
				b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
				if errors.Is(err, os.ErrNotExist) || (err == nil && strings.Contains(string(b), ") Z ")) {
					break
				}
				if err != nil {
					t.Fatalf("inspect owned pid %d state: %v", pid, err)
				}
			}
			if time.Since(cancelledAt) >= time.Second {
				t.Fatalf("owned pid %d remains running after cancellation bound", pid)
			}
			time.Sleep(time.Millisecond)
		}
	}
}
