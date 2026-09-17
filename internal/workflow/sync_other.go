//go:build !darwin && !linux

package workflow

// File data is synced before rename; portable Go has no directory-fsync
// guarantee on the other supported build targets.
func SyncDirectory(path string) error { return nil }
