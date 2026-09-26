# Release procedure

Audience: whoever publishes TotalPlug. Every version number a released build
carries comes from one place, a git tag, so nothing is edited by hand before
tagging, and `bin\totalplug*.exe` is produced only by the scripts below.

## Where the version comes from

`scripts/build.ps1` runs `git describe --tags --dirty` and strips the leading `v`:

| Consumer | How it gets the value |
| --- | --- |
| CLI `--version`, the About panel, the app's own release check | `-ldflags -X totalplug/internal/buildinfo.Version=…` |
| Windows file properties of the GUI exe (`FileVersion`, `ProductVersion`) | the same string written into `wails.json → info.productVersion` for the duration of `wails build`, then put back |

Three consequences worth knowing:

- With no reachable tag the version is the literal `dev`: the release check
  reports `comparable: false` and never claims an update. A hand-run `go build`
  produces the same `dev`.
- Commits after a tag produce `1.0.0-3-g2c1f9ab`, plus `-dirty` for uncommitted
  work. Comparison strips a `v` prefix and any `(...)` suffix before parsing the
  numbers, so a build ahead of the published release is never told it is
  outdated and a build behind it is.
- The tracked `1.0.0` in `wails.json` is only what a bare `wails dev` embeds.
  A scripted build overwrites that field and restores it whichever way the build
  ends, so leaving it behind the tag is harmless.

## 1. Pre-flight

```powershell
git status --porcelain      # must print nothing — a dirty tree stamps "-dirty"
go vet ./...
go test -race ./...
```

Run the GUI from the tree once and look at what is about to be published: the
About panel (version, data directory, links), the plugin table, the batch update
summary.

## 2. Tag

```powershell
git tag -a vX.Y.Z -m "vX.Y.Z"
git describe --tags         # must print exactly vX.Y.Z
```

Annotated tags are recommended, `--tags` reads either kind. Use the major for a
breaking change in the plugin or data format, the minor for new GUI sections and
the patch for fixes.

## 3. Build the artifacts

```powershell
scripts\release.cmd         # -> bin\totalplug-cli.exe
scripts\release-gui.cmd     # -> bin\totalplug.exe
```

Check the stamp landed in both:

```powershell
bin\totalplug-cli.exe --version
(Get-Item bin\totalplug.exe).VersionInfo.FileVersion   # also Properties → Details
```

Both must print `X.Y.Z` — not `dev`, not `…-dirty`. If they do not, delete the
tag (`git tag -d vX.Y.Z`), fix the tree and start again: an artifact and its tag
must never disagree.

## 4. Publish

```powershell
git push <public-remote> main
git push <public-remote> vX.Y.Z
gh release create vX.Y.Z bin/totalplug.exe bin/totalplug-cli.exe --title "TotalPlug vX.Y.Z" --generate-notes
```

`buildinfo.ReleaseRepo` is `goloveshko/totalplug` and that is the repository the
in-app release check queries, so the tag has to reach it — not only a local
mirror. Keep the two asset names exactly as above: the README Download section
names them and tells the reader which one to pick. The binaries are unsigned,
which SmartScreen will mention on first run; that is expected and documented
there, not something a release step fixes.

## 5. Smoke-test what was published

Download from the release page and run it where no toolchain is installed:
`--version` prints `X.Y.Z`, the About panel agrees, its release check says this
is the latest version, and a scan works with Total Commander closed. TotalPlug
does not replace its own executable, so a newer release reaches the user only as
a link back to the release page.

## Recovery

A published tag belongs to the release: prefer cutting the next patch version
over moving the tag, since moving needs a force-push and anyone who downloaded
the bad build would then never be told a newer one exists. If the release itself
was wrong, delete it and its tag together — otherwise `releases/latest` still
points at it and the app keeps offering a download that is gone.
