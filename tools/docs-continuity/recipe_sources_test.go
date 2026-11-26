package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecipeProgramsBoundToSource(t *testing.T) {
	for _, name := range []string{"recipe.mjs", "asset.py", "game.gd"} {
		t.Run(name, func(t *testing.T) {
			root := fixture(t)
			before, e := makeManifest(root)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(root, "internal", name), []byte("native program"), 0644); e != nil {
				t.Fatal(e)
			}
			after, e := makeManifest(root)
			if e != nil {
				t.Fatal(e)
			}
			if before.SourceSHA256 == after.SourceSHA256 {
				t.Fatal("program excluded from source identity")
			}
		})
	}
}
