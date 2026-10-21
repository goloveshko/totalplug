# Release procedure

Audience: whoever publishes TotalPlug. Every version number a released build
carries comes from one place, a git tag, so nothing is edited by hand before
tagging, and `bin\totalplug*.exe` is produced only by the scripts below.

## Where the version comes from

`scripts/build.ps1` runs `git describe --tags --dirty` and strips the leading `v`,
or reads `wails.json → info.productVersion` when no tag is reachable:

| Consumer                                                                 | How it gets the value                                                                                                            |
| ------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| CLI `--version`, the About panel, the app's own release check            | `-ldflags -X github.com/goloveshko/totalplug/internal/buildinfo.Version=…`                                                       |
| Windows file properties of the GUI exe (`FileVersion`, `ProductVersion`) | the numeric head of that string written into `wails.json → info.productVersion` for the duration of `wails build`, then put back |

Three consequences worth knowing:

- With no reachable tag the version is the one tracked in `wails.json`, so an
  untagged build compares like any other and will offer an update as soon as one is
  published. Only a binary built by hand with `go build` carries no stamp, reports
  `dev`, and leaves the comparison off.
- Commits after a tag produce `1.0.0-3-g2c1f9ab`, plus `-dirty` for uncommitted
  work. The About panel shows that whole string; the file properties cannot, because
  a `VERSIONINFO` resource takes only dotted numbers, so there it is stamped as its
  numeric head `1.0.0`. Comparison strips a `v` prefix and any `(...)` suffix before
  parsing the numbers, so a build ahead of the published release is never told it is
  outdated and a build behind it is.
- The tracked `1.0.0.0` in `wails.json` is a placeholder, not a version to
  maintain: a scripted build overwrites that field and restores it whichever way the
  build ends. Its fourth digit exists because a `VERSIONINFO` wants numbers, and the
  About panel drops a trailing `.0` so the placeholder reads as `1.0.0`.

## 1. Pre-flight

```powershell
git status --porcelain      # must print nothing — a dirty tree stamps "-dirty"
go vet ./...
go test -race ./...
```

`-race` needs cgo, so that last command dies with
`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1` on a machine where
`CGO_ENABLED=0` or no C compiler is in `PATH`. Enable it for the one run, naming
the compiler if it is not `gcc`:

```powershell
$env:CGO_ENABLED = "1"; $env:CC = "clang"; go test -race ./...
```

The linker then prints a `LNK4217` warning about `_errno` per test package; it is
noise, and the suite still has to end in `ok` for every package. Plain
`go test ./...` runs without cgo and is what CI-less checking falls back to — but
it does not detect races, so the release build gets the `-race` run.

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

## 3. Tag and push

```powershell
git push <public-remote> main
git push <public-remote> vX.Y.Z
```

The tag starts `.github/workflows/release.yml`. Its Windows job checks out the tag
with the full history, installs the Wails CLI at the version `go.mod` pins, and runs
`scripts\build.ps1 package` — the same script, payload and archive name the by-hand
path below uses. Before uploading it reads both stamps back (the CLI must print
`totalplug X.Y.Z`, the GUI's `FileVersion` must carry the numeric head) and fails
otherwise, because a build that fell back to `wails.json` would publish a `1.0.0`
archive under a new tag. It then opens a **draft** release for the tag with the zip
and the `.sha256` attached and generated notes.

Two things the workflow depends on: `fetch-depth: 0` (a shallow checkout has no
tags, so `git describe` fails and the script silently stamps the fallback) and the
Wails version in the workflow matching `go.mod`.

If the job fails after the draft exists, `gh release create` refuses to run again;
delete the draft (`gh release delete vX.Y.Z`) and re-run the job. A failure before
that point leaves no release at all.

### Building by hand instead

The same package without CI, from the same script:

```powershell
scripts\package.cmd
```

That one run builds both binaries in the release profile and packs them into
`build\artifacts\totalplug-vX.Y.Z-win-x64.zip`, with the archive name coming from
the same stamped version the binaries carry. It also writes the `.sha256` beside
it. `build\artifacts\` is gitignored, so a stale archive from an earlier run can
never be mistaken for a tracked file, but do check you are uploading the one this
run printed.

Check the stamp landed in both:

```powershell
bin\totalplug-cli.exe --version
(Get-Item bin\totalplug.exe).VersionInfo.FileVersion   # also Properties → Details
```

Both must print `X.Y.Z`, not `…-dirty`. Since the untagged fallback now carries the
same number as the tag, that check alone no longer proves where the stamp came from:
confirm it with

```powershell
git describe --tags --exact-match HEAD   # must print vX.Y.Z, not vX.Y.(Z-1)-N-g…
```

If either says otherwise, delete the tag (`git tag -d vX.Y.Z`), fix the tree and
start again: an artifact and its tag must never disagree. A build that is not
exactly on the tag still packages, and its archive is named after what
`git describe` said, which is how a `…-3-g2c1f9ab` build announces itself.

## 4. Publish

Open the draft the job created, replace the generated notes with the bilingual
body, and publish. Publishing is what moves `releases/latest`; until that click the
in-app release check keeps offering the previous version.

The by-hand path creates the release itself, and that publishes immediately:

```powershell
gh release create vX.Y.Z build/artifacts/totalplug-vX.Y.Z-win-x64.zip build/artifacts/totalplug-vX.Y.Z-win-x64.zip.sha256 --title "TotalPlug vX.Y.Z" --generate-notes
```

`buildinfo.ReleaseRepo` is `goloveshko/totalplug` and that is the repository the
in-app release check queries, so the tag has to reach it — not only a local
mirror. The archive name is what the README Download section tells a reader to
expect, so it comes from the script rather than from the release form. The
binaries are unsigned, which SmartScreen will mention on first run; that is
expected and documented there, not something a release step fixes.

## 5. Smoke-test what was published

Download the archive from the release page, verify it against the published
`.sha256`, extract it and run it where no toolchain is installed:
`--version` prints `X.Y.Z`, the About panel agrees, its release check says this
is the latest version, and a scan works with Total Commander closed. TotalPlug
does not replace its own executable, so a newer release reaches the user only as
a link back to the release page.

A clean Windows 10 without the WebView2 runtime is worth one run of its own
(`scripts\sandbox.cmd -NoRuntime`): the GUI then opens with Wails' own "Missing
Requirements" prompt before any window of ours exists, and **OK** downloads and
installs the runtime. That dialog is not ours to translate — it is shown before
the interface language is even read — so what the check is really for is that the
path out of it works. Other sandbox runs install the runtime themselves instead,
from the offline Evergreen Standalone Installer that the script keeps in
`bin\totalplug-sandbox`, so an ordinary run is not waiting on that download.

## Recovery

A published tag belongs to the release: prefer cutting the next patch version
over moving the tag, since moving needs a force-push and anyone who downloaded
the bad build would then never be told a newer one exists. If the release itself
was wrong, delete it and its tag together — otherwise `releases/latest` still
points at it and the app keeps offering a download that is gone.
