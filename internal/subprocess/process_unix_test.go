//go:build darwin || linux

package subprocess

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	cancel()
	r := <-done
	if r.Cause != "cancelled" || r.Duration > time.Second {
		t.Fatalf("%+v", r)
	}
	// Linux init can briefly retain a dead adopted descendant as a zombie.
	for _, pid := range pids {
		if err := syscall.Kill(pid, 0); err == nil {
			b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
			if e != nil || !strings.Contains(string(b), ") Z ") {
				t.Fatalf("owned pid %d remains running", pid)
			}
		}
	}
}
