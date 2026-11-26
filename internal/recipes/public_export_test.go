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
	result, e := ExportWithOptions(c, out, ExportOptions{PackageVersion: "0.1.0", Public: true, RuntimeVersion: "0.5.0-rc.4"})
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
	if p["name"] != "@justynclark/loopexec-recipes" || p["private"] != false || p["version"] != "0.1.0" || p["license"] != "MIT" {
		t.Fatal("bad public metadata")
	}
	if b, e = os.ReadFile(filepath.Join(out, "LICENSE")); e != nil || len(b) < 100 {
		t.Fatal("missing license")
	}
	err := filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, b := range body {
			if b > 127 {
				t.Errorf("non-ASCII pack file: %s", path)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	exported, err := os.ReadFile(filepath.Join(out, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip Catalog
	if err = json.Unmarshal(exported, &roundTrip); err != nil {
		t.Fatal(err)
	}
	for i, r := range c.Recipes {
		if roundTrip.Recipes[i].SourceIdea != r.SourceIdea {
			t.Fatal("source idea changed")
		}
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
