package main

import "testing"

func TestReleaseChannels(t *testing.T) {
	for _, tag := range []string{"v0.4.1", "v0.5.0", "v1.0.0"} {
		got, err := policyFor(tag)
		if err != nil || got.Prerelease || !got.Latest || "v"+got.Version != tag {
			t.Fatalf("stable policy: %+v %v", got, err)
		}
	}
	for _, tag := range []string{"v0.5.0-rc.1", "v1.0.0-rc.12"} {
		got, err := policyFor(tag)
		if err != nil || !got.Prerelease || got.Latest || "v"+got.Version != tag {
			t.Fatalf("RC must not replace stable latest: %+v %v", got, err)
		}
	}
	for _, tag := range []string{"", "main", "v0.5.0-dev", "v0.5.0-beta.1", "v0.5.0-rc.0", "v0.5.0-rc.01", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3;echo", "v1.2.3\n"} {
		if _, err := policyFor(tag); err == nil {
			t.Fatalf("invalid publish tag accepted: %q", tag)
		}
	}
}
