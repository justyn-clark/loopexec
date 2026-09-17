package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}
type Manifest struct {
	Files []Artifact `json:"files"`
}

func CleanRelative(p string) bool {
	return p != "" && p != "." && !filepath.IsAbs(p) && filepath.Clean(p) == p && p != ".." && !strings.HasPrefix(p, ".."+string(filepath.Separator))
}
func Within(root, path string) bool {
	r, e := filepath.Rel(root, path)
	return e == nil && (r == "." || CleanRelative(r))
}
func SafePath(root, rel string) (string, error) {
	if !CleanRelative(rel) {
		return "", fmt.Errorf("invalid manifest path")
	}
	root, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	actual, e := filepath.EvalSymlinks(root)
	if e != nil || actual != root {
		return "", fmt.Errorf("candidate root must not traverse symlinks")
	}
	p := root
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		p = filepath.Join(p, seg)
		i, e := os.Lstat(p)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return "", e
		}
		if i.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink in candidate path")
		}
	}
	return p, nil
}
func excluded(p string, ex []string) bool {
	for _, x := range ex {
		if p == x || strings.HasPrefix(p, x+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
func ValidateManifestPolicy(p CandidatePolicy) error {
	if !CleanRelative(p.Root) || len(p.Include) == 0 {
		return fmt.Errorf("candidate root/include required")
	}
	for _, part := range strings.Split(p.Root, string(filepath.Separator)) {
		if part == ".git" || part == ".small" || part == ".loopexec" || part == ".env" {
			return fmt.Errorf("reserved candidate root")
		}
	}
	all := append(append([]string{}, p.Include...), p.Exclude...)
	for _, x := range all {
		if !CleanRelative(x) {
			return fmt.Errorf("invalid candidate manifest")
		}
	}
	for i, a := range p.Include {
		for j, b := range p.Include {
			if i != j && (a == b || strings.HasPrefix(a, b+string(filepath.Separator))) {
				return fmt.Errorf("overlapping candidate includes")
			}
		}
	}
	for _, x := range p.Protected {
		if !CleanRelative(x) {
			return fmt.Errorf("invalid protected path")
		}
	}
	return nil
}
func digest(ctx context.Context, path string) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("hash target must be a regular file")
	}
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 64*1024)
	var size int64
	for {
		if e := ctx.Err(); e != nil {
			return "", 0, e
		}
		n, e := f.Read(buf)
		if n > 0 {
			size += int64(n)
			if size > 1<<30 {
				return "", 0, fmt.Errorf("candidate file exceeds 1 GiB")
			}
			_, _ = h.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", 0, e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
func Capture(ctx context.Context, root string, p CandidatePolicy) (Manifest, error) {
	m := Manifest{Files: []Artifact{}}
	for _, inc := range p.Include {
		path, e := SafePath(root, inc)
		if e != nil {
			return m, e
		}
		e = filepath.WalkDir(path, func(path string, d fs.DirEntry, e error) error {
			if os.IsNotExist(e) {
				return nil
			}
			if e != nil {
				return e
			}
			if e = ctx.Err(); e != nil {
				return e
			}
			rel, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			if excluded(rel, p.Exclude) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("candidate symlinks forbidden")
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("candidate special file forbidden")
			}
			sum, size, e := digest(ctx, path)
			if e != nil {
				return e
			}
			m.Files = append(m.Files, Artifact{rel, sum, size, uint32(info.Mode().Perm())})
			if len(m.Files) > 10000 {
				return fmt.Errorf("candidate manifest exceeds 10000 files")
			}
			return nil
		})
		if e != nil {
			return m, e
		}
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	return m, nil
}
func CopyArtifact(ctx context.Context, src, dst string, a Artifact) error {
	from, e := SafePath(src, a.Path)
	if e != nil {
		return e
	}
	to, e := SafePath(dst, a.Path)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(to), 0700); e != nil {
		return e
	}
	in, e := os.Open(from)
	if e != nil {
		return e
	}
	defer in.Close()
	tmp, e := os.CreateTemp(filepath.Dir(to), ".candidate-")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	buf := make([]byte, 64*1024)
	var size int64
	for {
		if e = ctx.Err(); e != nil {
			break
		}
		var n int
		n, e = in.Read(buf)
		if n > 0 {
			size += int64(n)
			if size > a.Size {
				e = fmt.Errorf("candidate changed during snapshot")
				break
			}
			_, _ = h.Write(buf[:n])
			if _, we := tmp.Write(buf[:n]); we != nil {
				e = we
				break
			}
		}
		if e == io.EOF {
			e = nil
			break
		}
		if e != nil {
			break
		}
	}
	if e == nil && (size != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256) {
		e = fmt.Errorf("candidate changed during snapshot")
	}
	if e == nil {
		e = tmp.Chmod(os.FileMode(a.Mode))
	}
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp.Name(), to)
}

// FileHash streams a bounded regular file with cancellation checks.
func FileHash(ctx context.Context, path string) (string, int64, error) { return digest(ctx, path) }

// SyncDirectories makes staged directory entries durable before publication.
func SyncDirectories(ctx context.Context, root string) error {
	var dirs []string
	if err := ctx.Err(); err != nil {
		return err
	}
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	if e != nil {
		return e
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := SyncDirectory(dirs[i]); e != nil {
			return e
		}
	}
	return nil
}
