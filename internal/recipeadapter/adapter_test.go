package recipeadapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaRejectsWeakeningAndUnsupportedKeywords(t *testing.T) {
	root := map[string]any{}
	schema := map[string]any{"type": "object", "required": []any{"id"}, "properties": map[string]any{"id": map[string]any{"type": "integer", "minimum": float64(1)}}, "additionalProperties": false}
	for _, tc := range []struct {
		value map[string]any
		valid bool
	}{{map[string]any{"id": float64(2)}, true}, {map[string]any{}, false}, {map[string]any{"id": float64(0)}, false}, {map[string]any{"id": float64(2), "extra": true}, false}} {
		got, e := schemaValid(root, schema, tc.value, 0)
		if e != nil || got != tc.valid {
			t.Fatalf("%v -> %v %v", tc.value, got, e)
		}
	}
	for _, key := range []string{"format", "uniqueItems", "not", "if"} {
		_, e := schemaValid(root, map[string]any{key: true}, nil, 0)
		if e == nil {
			t.Fatal("silently ignored", key)
		}
	}
	_, e := schemaValid(root, map[string]any{"$ref": "https://invalid.example/schema"}, nil, 0)
	if e == nil {
		t.Fatal("accepted remote ref")
	}
	valid, e := schemaValid(root, map[string]any{"type": "number", "minimum": float64(1), "exclusiveMinimum": true}, float64(1), 0)
	if e != nil || valid {
		t.Fatal("ignored exclusive bound")
	}
}
func TestCommandWordsLiteralAndFailClosed(t *testing.T) {
	a, e := CommandWords("tool 'value with spaces' '$(touch bad)'")
	if e != nil || len(a) != 3 || a[2] != "$(touch bad)" {
		t.Fatalf("%v %v", a, e)
	}
	for _, s := range []string{"tool | other", "tool > out", "'unterminated", ""} {
		if _, e := CommandWords(s); e == nil {
			t.Fatal("accepted shell syntax", s)
		}
	}
}
func TestCleaningRejectsDuplicatesAndLineageLoss(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(root, "source.csv"), []byte("id,date,value\na,2026-10-27,10\nb,2026-10-27,20\n"), 0600)
	c := Context{Root: root, EvidenceDir: t.TempDir(), Inputs: map[string]any{"source": "source.csv", "output": "out.csv", "required_fields": []any{"id", "date"}, "id_field": "id", "date_fields": []any{"date:2006-01-02"}, "ranges": []any{"value:0:100"}, "max_dropped_rows": float64(0)}}
	for _, data := range []string{"id,date,value\na,2026-10-27,10\na,2026-10-27,10\n", "id,date,value\na,invalid,200\n", "id,date,value\nc,2026-10-27,10\nb,2026-10-27,20\n"} {
		os.WriteFile(filepath.Join(root, "out.csv"), []byte(data), 0600)
		m, e := cleaning(context.Background(), c)
		if e != nil || m.Passed() {
			t.Fatalf("accepted dirty data %s: %+v %v", data, m, e)
		}
	}
}
func TestManifestRejectsSymlinkAndBindsInventory(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(root, "a"), []byte("a"), 0600)
	a, e := Manifest(context.Background(), root, []string{"a"})
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(root, "a"), []byte("b"), 0600)
	b, _ := Manifest(context.Background(), root, []string{"a"})
	if a.Files[0].SHA256 == b.Files[0].SHA256 {
		t.Fatal("unbound candidate")
	}
	os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "link"))
	if _, e = Manifest(context.Background(), root, []string{"link"}); e == nil {
		t.Fatal("accepted symlink")
	}
}
func TestStructuredMissingOracleAndReportRejection(t *testing.T) {
	c := Context{Root: t.TempDir(), EvidenceDir: t.TempDir(), Recipe: "godot-performance", TimeoutSeconds: 2}
	if _, e := structured(context.Background(), c); e == nil || !strings.Contains(e.Error(), "oracle") {
		t.Fatal(e)
	}
	var r OracleReport
	if json.Unmarshal([]byte("{"), &r) == nil {
		t.Fatal("accepted malformed report")
	}
}
