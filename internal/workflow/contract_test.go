package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStrictEvidenceJSON(t *testing.T) {
	for _, s := range []string{`{"x":1,"x":2}`, `{"x":{"a":1,"a":2}}`, `{"x":[],"unexpected":1}`, `{"x":1} {}`} {
		var v struct {
			X any `json:"x"`
		}
		if Decode([]byte(s), &v) == nil {
			t.Fatal("accepted", s)
		}
	}
	var s Score
	if Decode([]byte(`{"schema_version":1,"run_id":"r","iteration":1,"candidate_id":"c"}`), &s) == nil {
		t.Fatal("missing score evidence accepted")
	}
	if Decode([]byte(`{"schema_version":1,"run_id":"r","iteration":1,"candidate_id":"c","reviewed":false,"technical":false}`), &s) != nil {
		t.Fatal("explicit unreviewed rejected")
	}
}
func TestRuntimeLock(t *testing.T) {
	dir := t.TempDir()
	release, e := Lock(dir)
	if e != nil {
		t.Fatal(e)
	}
	if other, e := Lock(dir); e == nil {
		other()
		t.Fatal("second governor acquired lock")
	}
	release()
	again, e := Lock(dir)
	if e != nil {
		t.Fatal(e)
	}
	again()
}
func TestReadEvidenceRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("{}"), 0600)
	if e := os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "b")); e != nil {
		t.Skip(e)
	}
	if Read(filepath.Join(dir, "b"), new(any)) == nil {
		t.Fatal("symlink accepted")
	}
}
