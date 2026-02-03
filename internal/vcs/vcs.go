package vcs

import "runtime/debug"

// Version returns the version number of our build
func Version() string {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		return bi.Main.Version
	}
	return ""
}
