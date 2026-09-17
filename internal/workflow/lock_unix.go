//go:build darwin || linux

package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func Lock(dir string) (func(), error) {
	f, e := os.OpenFile(filepath.Join(dir, "run.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("another governor owns this runtime directory")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
