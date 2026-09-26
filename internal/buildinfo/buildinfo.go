// Package buildinfo holds the values stamped into the binary at build time.
package buildinfo

// Version is set by scripts/build.ps1 through -ldflags -X from `git describe
// --tags --dirty`. It stays "dev" for a plain `go build` and for a tree with no
// reachable tag, which is what tells the update check not to compare at all.
var Version = "dev"

// ReleaseRepo is the GitHub repository published builds come from.
const ReleaseRepo = "goloveshko/totalplug"

// ReleasesURL is where a user is sent to download a newer release; TotalPlug
// never replaces its own binary.
const ReleasesURL = "https://github.com/" + ReleaseRepo + "/releases"
