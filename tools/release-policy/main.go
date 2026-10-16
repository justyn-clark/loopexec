package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type releasePolicy struct {
	Version    string `json:"version"`
	Prerelease bool   `json:"prerelease"`
	Latest     bool   `json:"latest"`
}

var releaseTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$`)

func policyFor(tag string) (releasePolicy, error) {
	if len(tag) > 100 || !releaseTag.MatchString(tag) {
		return releasePolicy{}, fmt.Errorf("release tag must be vX.Y.Z or vX.Y.Z-rc.N with canonical numeric components")
	}
	pre := strings.Contains(tag, "-rc.")
	return releasePolicy{Version: strings.TrimPrefix(tag, "v"), Prerelease: pre, Latest: !pre}, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: release-policy <release-tag>")
		os.Exit(2)
	}
	policy, err := policyFor(os.Args[1])
	if err == nil && !policy.Prerelease {
		err = requireStableReadiness(".")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := json.NewEncoder(os.Stdout).Encode(policy); err != nil {
		os.Exit(1)
	}
}
