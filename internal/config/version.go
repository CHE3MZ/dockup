package config

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Go stamps local VCS builds with a pseudo-version such as
// v1.2.2-0.20261020155638-f53f85ea879b+dirty. Show the commit instead.
var pseudoVersion = regexp.MustCompile(`-\d+\.\d{14}-[0-9a-f]{7,}`)

// Only real vX.Y.Z tags carry image assets; pseudo-versions and SHAs do not.
var imageReleaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z][0-9A-Za-z.-]*)?$`)

// resolveVersion fills in Version when the binary was not stamped with
// -ldflags -X. `go install pkg@version` records the module version in the
// build info, and a local `go build` records the VCS revision.
func resolveVersion(stamped string) string {
	stamped = strings.TrimSpace(stamped)
	if stamped != "" && stamped != "dev" {
		return stamped
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return stamped
	}
	main := strings.TrimSpace(info.Main.Version)
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if main != "" && main != "(devel)" && !pseudoVersion.MatchString(main) {
		return main
	}
	if len(rev) >= 7 {
		rev = rev[:7]
		if dirty {
			rev += "-dirty"
		}
		return rev
	}
	if main != "" && main != "(devel)" {
		return main
	}
	return stamped
}
