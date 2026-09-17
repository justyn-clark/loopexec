// Package subprocess owns bounded output and subprocess lifetimes.
package subprocess

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const DefaultLimit = 1 << 20

type Options struct {
	Stdin   string
	Dir     string
	Argv    []string
	Timeout time.Duration
	Grace   time.Duration
	Limit   int
	// JoinGroup is for nested adapter hooks: the governor owns their group.
	JoinGroup bool
}
type Result struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Combined  string
	Truncated bool
	Duration  time.Duration
	Cause     string
}
type tail struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (b *tail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(b.data)+n > b.limit {
		b.truncated = true
	}
	if n >= b.limit {
		b.data = append(b.data[:0], p[n-b.limit:]...)
	} else {
		drop := len(b.data) + n - b.limit
		if drop > 0 {
			b.data = b.data[drop:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

type tee struct{ own, combined *tail }

func (w tee) Write(p []byte) (int, error) { w.combined.Write(p); return w.own.Write(p) }

// Run never emits command/output text. Callers may consume bounded diagnostic
// tails privately, but must not copy arbitrary tool output into receipts.
func Run(parent context.Context, o Options) Result {
	start := time.Now()
	r := Result{ExitCode: -1}
	if len(o.Argv) == 0 {
		r.Cause = "start_failed"
		return r
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx := parent
	var cancel context.CancelFunc
	if o.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, o.Timeout)
		defer cancel()
	}
	if ctx.Err() != nil {
		r.Cause = cause(ctx)
		return r
	}
	if o.Grace <= 0 {
		o.Grace = 250 * time.Millisecond
	}
	if o.Limit <= 0 {
		o.Limit = DefaultLimit
	}
	out, errout, both := &tail{limit: o.Limit}, &tail{limit: o.Limit}, &tail{limit: o.Limit}
	c := exec.Command(o.Argv[0], o.Argv[1:]...)
	c.Dir = o.Dir
	c.Stdin = strings.NewReader(o.Stdin)
	c.Stdout = tee{out, both}
	c.Stderr = tee{errout, both}
	configure(c, o.JoinGroup)
	// Bound pipe copies even if an escaped descendant retains a descriptor.
	c.WaitDelay = o.Grace
	if err := c.Start(); err != nil {
		r.Cause = "start_failed"
		r.Duration = time.Since(start)
		return r
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	var err error
	select {
	case err = <-done:
		// Background descendants are owned work, not a detached service.
		if !o.JoinGroup {
			kill(c)
		}
	case <-ctx.Done():
		r.Cause = cause(ctx)
		terminate(c)
		timer := time.NewTimer(o.Grace)
		<-timer.C
		kill(c)
		err = <-done
	}
	if c.ProcessState != nil {
		r.ExitCode = c.ProcessState.ExitCode()
	}
	if errors.Is(err, exec.ErrWaitDelay) && r.Cause == "" {
		r.Cause = "pipe_timeout"
	}
	r.Stdout = string(out.data)
	r.Stderr = string(errout.data)
	r.Combined = string(both.data)
	r.Truncated = out.truncated || errout.truncated || both.truncated
	r.Duration = time.Since(start)
	return r
}
func cause(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	return "cancelled"
}
