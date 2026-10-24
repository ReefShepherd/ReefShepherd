// Package version reports the build version of the binary.
package version

import (
	"runtime"
	"runtime/debug"
)

// version is set at link time with -ldflags "-X .../internal/version.version=v1.2.3".
var version = "dev"

// Version is the release version, or "dev" for an unversioned build.
func Version() string {
	return version
}

// String is the version plus the commit, Go version and platform, for `reefshepherd version`.
func String() string {
	out := version
	if commit := commit(); commit != "" {
		out += " (commit " + commit + ")"
	}
	return out + " " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
}

func commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var revision string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if revision != "" && dirty {
		revision += "-dirty"
	}
	return revision
}
