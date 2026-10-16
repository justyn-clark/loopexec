package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStableReadinessFailsClosed(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "scripts"), 0700)
	if requireStableReadiness(root) == nil {
		t.Fatal("missing evidence accepted")
	}
	gates := []map[string]any{}
	for _, id := range requiredGates {
		gates = append(gates, map[string]any{"id": id, "status": "verified", "evidence": []string{"approved-source-bound-proof"}})
	}
	for _, ready := range []bool{false, true} {
		data, _ := json.Marshal(map[string]any{"schema_version": 1, "stable_ready": ready, "gates": gates})
		os.WriteFile(filepath.Join(root, "scripts/release-readiness.json"), data, 0600)
		if (requireStableReadiness(root) == nil) != ready {
			t.Fatal("stable hold not honored")
		}
	}
	gates[0]["status"] = "offline_verified"
	data, _ := json.Marshal(map[string]any{"schema_version": 1, "stable_ready": true, "gates": gates})
	os.WriteFile(filepath.Join(root, "scripts/release-readiness.json"), data, 0600)
	if requireStableReadiness(root) == nil {
		t.Fatal("offline-only gate accepted for stable")
	}
}
