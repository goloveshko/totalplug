// Package buildinfo holds the values stamped into the binary at build time.
package buildinfo

// Version is set by scripts/build.ps1 through -ldflags -X from `git describe
// --tags --dirty`, which before the first tag falls back to the version tracked in
// wails.json. Only a build made without that script carries no stamp at all and so
// keeps the literal "dev", which is what tells the update check not to compare.
var Version = "dev"

// ReleaseRepo is the GitHub repository published builds come from.
const ReleaseRepo = "goloveshko/totalplug"

// ReleasesURL is where a user is sent to download a newer release; TotalPlug
// never replaces its own binary.
const ReleasesURL = "https://github.com/" + ReleaseRepo + "/releases"
