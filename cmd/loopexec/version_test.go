package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionIdentity(t *testing.T) {
	defer func() { jsonOutput = false }()
	for _, args := range [][]string{{"version", "--json"}, {"--version"}} {
		cmd := newRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if args[0] == "version" {
			var got response
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Tool != toolName || got.Version != toolVersion || got.Status != "ok" {
				t.Fatalf("unexpected identity: %+v", got)
			}
		} else if !strings.Contains(buf.String(), toolVersion) {
			t.Fatalf("version flag missing identity: %q", buf.String())
		}
	}
}
