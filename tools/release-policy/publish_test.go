package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicationChannelAndReadback(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		fail bool
	}{{"v0.5.0", false}, {"v0.5.0-rc.1", false}, {"v0.5.0-rc.1", true}} {
		t.Run(tc.tag+map[bool]string{true: "-readback-failure"}[tc.fail], func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, data string, mode os.FileMode) {
				t.Helper()
				p := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(data), mode); err != nil {
					t.Fatal(err)
				}
			}
			policy, _ := policyFor(tc.tag)
			encoded, _ := json.Marshal(policy)
			write("bin/go", "#!/bin/sh\nprintf 'go %s\\n' \"$*\" >> \"$COMMAND_LOG\"\nshift\ncase \"$1\" in\n./tools/release-policy) printf '%s\\n' '"+string(encoded)+"';;\n./tools/docs-continuity) if [ \"$2\" = verify-site ]; then\n if [ -f \"$SITE_SEEN\" ] && [ \"$FAIL_SECOND\" = 1 ]; then exit 1; fi\n touch \"$SITE_SEEN\"\n fi;;\nesac\n", 0755)
			write("bin/gh", "#!/bin/sh\nprintf 'gh %s\\n' \"$*\" >> \"$COMMAND_LOG\"\n", 0755)
			for _, name := range []string{"dist/checksums.txt", "dist/loopexec-test.tar.gz", "dist/loopexec-test.zip", "docs/releases/" + tc.tag + ".md"} {
				write(name, "fixture", 0644)
			}
			script, err := filepath.Abs("../../scripts/publish-release.sh")
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", script, tc.tag, "dist")
			cmd.Dir = dir
			fail := "0"
			if tc.fail {
				fail = "1"
			}
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "COMMAND_LOG="+filepath.Join(dir, "commands"), "SITE_SEEN="+filepath.Join(dir, "site-seen"), "FAIL_SECOND="+fail)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v, output=%s", err, out)
			}
			data, err := os.ReadFile(filepath.Join(dir, "commands"))
			if err != nil {
				t.Fatal(err)
			}
			log := string(data)
			create := strings.Index(log, "gh release create")
			first := strings.Index(log, "go run ./tools/docs-continuity verify-site")
			last := strings.LastIndex(log, "go run ./tools/docs-continuity verify-site")
			edit := strings.Index(log, "gh release edit")
			if first < 0 || create <= first || last <= create {
				t.Fatalf("missing readback gates: %s", log)
			}
			if tc.fail && edit >= 0 || !tc.fail && edit <= last {
				t.Fatalf("publication not gated: %s", log)
			}
			flags := "--prerelease=false --latest=true"
			if policy.Prerelease {
				flags = "--prerelease=true --latest=false"
			}
			for _, line := range strings.Split(log, "\n") {
				if strings.HasPrefix(line, "gh release ") && !strings.Contains(line, flags) {
					t.Fatalf("wrong channel: %s", line)
				}
			}
		})
	}
}
