package recipes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicPackMetadataAndRefusal(t *testing.T) {
	c, e := Load()
	if e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "public")
	result, e := ExportWithOptions(c, out, ExportOptions{PackageVersion: "0.1.0", Public: true, RuntimeVersion: "0.5.0-rc.3"})
	if e != nil {
		t.Fatal(e)
	}
	if result.Skills != 33 || result.Prompts != 33 {
		t.Fatal("incomplete pack")
	}
	b, e := os.ReadFile(filepath.Join(out, "package.json"))
	if e != nil {
		t.Fatal(e)
	}
	var p map[string]any
	if e = json.Unmarshal(b, &p); e != nil {
		t.Fatal(e)
	}
	if p["private"] != false || p["version"] != "0.1.0" || p["license"] != "MIT" {
		t.Fatal("bad public metadata")
	}
	if b, e = os.ReadFile(filepath.Join(out, "LICENSE")); e != nil || len(b) < 100 {
		t.Fatal("missing license")
	}
	for _, version := range []string{"", "bad", "../escape", "01.2.3"} {
		target := filepath.Join(t.TempDir(), "invalid")
		_, e = ExportWithOptions(c, target, ExportOptions{PackageVersion: version, Public: true})
		if e == nil {
			t.Fatal("accepted bad distribution", version)
		}
		if _, e = os.Stat(target); !os.IsNotExist(e) {
			t.Fatal("invalid export created files")
		}
	}
}
