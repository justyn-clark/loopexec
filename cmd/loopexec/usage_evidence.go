package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/justyn-clark/loopexec/internal/workflow"
)

func (w *workflowRuntime) preserveUsage(req workflow.Request, raw string, complete bool) (string, error) {
	if !validRunID(req.RunID) || !validRunID(req.Phase) || len(raw) > workflow.MaxEvidence {
		return "", fmt.Errorf("invalid usage evidence identity")
	}
	relative := filepath.Join(".loopexec", req.RunID, "usage-reports")
	dir, err := workflow.SafePath(w.workdir, relative)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err = workflow.SyncDirectory(filepath.Dir(dir)); err != nil {
		return "", err
	}
	extension := ".json"
	if !complete {
		extension = ".diagnostic-tail"
	}
	data := []byte(raw)
	relative = filepath.Join(relative, fmt.Sprintf("%06d-%s-%s%s", req.Iteration, req.Phase, sha256hex(data), extension))
	name, err := workflow.SafePath(w.workdir, relative)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		info, statErr := os.Lstat(name)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
			return "", fmt.Errorf("usage evidence changed")
		}
		old, readErr := os.Open(name)
		if readErr != nil {
			return "", readErr
		}
		defer old.Close()
		opened, statErr := old.Stat()
		if statErr != nil || !os.SameFile(info, opened) {
			return "", fmt.Errorf("usage evidence changed")
		}
		prior, readErr := io.ReadAll(io.LimitReader(old, workflow.MaxEvidence+1))
		if readErr != nil || !bytes.Equal(prior, data) {
			return "", fmt.Errorf("usage evidence changed")
		}
		if err := old.Sync(); err != nil {
			return "", err
		}
		return relative, workflow.SyncDirectory(dir)
	}
	if err != nil {
		return "", err
	}
	n, writeErr := f.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return relative, workflow.SyncDirectory(dir)
}
