// audit-privacy rejects new host paths without rewriting original receipts.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var uri = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s\x22\x27\x60<>]+`)
var fileURI = regexp.MustCompile(`(?i)\bfile:`)
var hostRoot = regexp.MustCompile(`(?i)(^|[\s\x22\x27\x60=(:,\[])/(Users|home|private|tmp|var|Volumes|root|opt|usr|etc)/`)
var absolutePath = regexp.MustCompile("(?m)(^|[\\s\\x22\\x27\\x60=(:,\\[])(/[^/\\s\\x22\\x27\\x60;,)}\\]]|[a-zA-Z]:[/\\\\]|\\\\\\\\|//[^/\\s]|~[^/\\\\\\s\\x22\\x27\\x60]*[/\\\\])")

func hostPath(value string) bool {
	return fileURI.MatchString(value) || hostRoot.MatchString(value) || absolutePath.MatchString(uri.ReplaceAllString(value, "<uri>"))
}

func newPaths(current, previous any, at string) []string {
	if reflect.DeepEqual(current, previous) {
		return nil
	}
	var found []string
	switch value := current.(type) {
	case string:
		if hostPath(value) {
			found = append(found, at)
		}
	case map[string]any:
		old, _ := previous.(map[string]any)
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			location := at + "." + key
			if hostPath(key) {
				location = at + ".<key>"
				found = append(found, location)
			}
			found = append(found, newPaths(value[key], old[key], location)...)
		}
	case []any:
		old, _ := previous.([]any)
		for index, item := range value {
			var before any
			if index < len(old) {
				before = old[index]
			}
			found = append(found, newPaths(item, before, fmt.Sprintf("%s[%d]", at, index))...)
		}
	default:
		if current != nil && hostPath(fmt.Sprint(current)) {
			found = append(found, at)
		}
	}
	return found
}

func inspect(path string, current, previous []byte) ([]string, error) {
	if bytes.Equal(current, previous) {
		return nil, nil
	}
	if strings.HasPrefix(path, ".small-command-proofs/") {
		if previous != nil {
			return nil, errors.New("historical command proof was modified")
		}
		if hostPath(string(current)) {
			return []string{"command"}, nil
		}
		return nil, nil
	}
	var now, before map[string]any
	if err := yaml.Unmarshal(current, &now); err != nil {
		return nil, errors.New("invalid canonical YAML")
	}
	if previous != nil {
		if err := yaml.Unmarshal(previous, &before); err != nil {
			return nil, errors.New("invalid baseline YAML")
		}
	}
	if path == ".small/progress.small.yml" {
		old, _ := before["entries"].([]any)
		entries, _ := now["entries"].([]any)
		if len(entries) < len(old) {
			return nil, errors.New("historical progress was removed")
		}
		for i := range old {
			if !reflect.DeepEqual(entries[i], old[i]) {
				return nil, errors.New("historical progress was modified")
			}
		}
	}
	return newPaths(now, before, "record"), nil
}

func git(root string, args ...string) ([]byte, error) {
	return exec.Command("git", append([]string{"-C", root}, args...)...).Output()
}

func check(root, base string) error {
	commit, err := git(root, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return errors.New("audit baseline is not a local commit")
	}
	base = strings.TrimSpace(string(commit))
	if _, err := git(root, "merge-base", "--is-ancestor", base, "HEAD"); err != nil {
		return errors.New("audit baseline is not an ancestor")
	}
	paths, err := git(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".small", ".small-command-proofs")
	if err != nil {
		return errors.New("cannot enumerate canonical audit files")
	}
	baselinePaths, err := git(root, "ls-tree", "-r", "--name-only", "-z", base, "--", ".small", ".small-command-proofs")
	if err != nil {
		return errors.New("cannot enumerate historical audit files")
	}
	paths = append(paths, baselinePaths...)
	seen, failed := map[string]bool{}, false
	for _, path := range strings.Split(string(paths), "\x00") {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		info, err := os.Lstat(filepath.Join(root, path))
		if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
			return errors.New("canonical audit file missing, linked or oversized")
		}
		current, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return errors.New("cannot read canonical audit file")
		}
		previous, err := git(root, "show", base+":"+path)
		if err != nil {
			previous = nil
		}
		locations, err := inspect(path, current, previous)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", path, err)
			failed = true
		}
		for _, location := range locations {
			// Report only the record location, not the offending private value.
			fmt.Fprintf(os.Stderr, "%s: %s: non-portable host path\n", path, location)
			failed = true
		}
	}
	if failed {
		return errors.New("new audit records require relative paths or logical aliases")
	}
	return nil
}

func main() {
	root := flag.String("repo", ".", "repository to inspect")
	base := flag.String("base", "", "explicit historical baseline for a local check")
	flag.Parse()
	if *base == "" {
		data, err := os.ReadFile(filepath.Join(*root, "scripts/audit-privacy-policy.json"))
		var policy struct {
			LegacyBase string `json:"legacy_base"`
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		var extra any
		if err != nil || decoder.Decode(&policy) != nil || decoder.Decode(&extra) != io.EOF || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(policy.LegacyBase) {
			fmt.Fprintln(os.Stderr, "AUDIT_PRIVACY_FAILED: invalid explicit historical policy")
			os.Exit(1)
		}
		*base = policy.LegacyBase
	}
	if err := check(*root, *base); err != nil {
		fmt.Fprintln(os.Stderr, "AUDIT_PRIVACY_FAILED:", err)
		os.Exit(1)
	}
	fmt.Println("AUDIT_PRIVACY_OK")
}
