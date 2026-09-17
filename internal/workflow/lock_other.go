//go:build !darwin && !linux

package workflow

import (
	"os"
	"path/filepath"
)

func Lock(dir string) (func(), error) {
	path := filepath.Join(dir, "run.lock")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	return func() { _ = f.Close(); _ = os.Remove(path) }, nil
}
