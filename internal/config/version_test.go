package config

import "testing"

func TestResolveVersionKeepsStamp(t *testing.T) {
	for _, v := range []string{"v1.2.3", "v1.2.3-rc1", "f53f85e-dirty"} {
		if got := resolveVersion(v); got != v {
			t.Fatalf("resolveVersion(%q) = %q", v, got)
		}
	}
}

func TestPseudoVersion(t *testing.T) {
	for _, v := range []string{
		"v1.2.2-0.20261020155638-f53f85ea879b",
		"v1.2.2-0.20261020155638-f53f85ea879b+dirty",
	} {
		if !pseudoVersion.MatchString(v) {
			t.Fatalf("%q not detected as a pseudo-version", v)
		}
	}
	for _, v := range []string{"v1.2.1", "v1.2.1-rc1", "f53f85e"} {
		if pseudoVersion.MatchString(v) {
			t.Fatalf("%q wrongly detected as a pseudo-version", v)
		}
	}
}
