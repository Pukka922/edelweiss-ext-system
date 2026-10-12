# edelweiss-ext-system

> **Note:** This project is vibe coded, for lack of time, and made only for my personal use: to manage my Raspberry Pi
> more comfortably. Anyone is welcome to use it, but at their own risk.

The "System overview" extension of [edelweiss](https://github.com/Pukka922/edelweiss), the web administration
interface for Alpine Linux. It shows the host name (changeable in admin mode), the operating system, kernel, CPU,
memory and uptime, and live CPU, memory, swap, load, file system and network usage.

It is one static binary with a Go backend (`main.go`, `internal/`) and a SolidJS interface (`ui/`), built with the
edelweiss SDKs. It has the same layout as the template
[edelweiss-ext-hello](https://github.com/Pukka922/edelweiss-ext-hello), with its backend in `internal/`. For a new
extension, start from edelweiss-ext-hello; see
[Developing edelweiss extensions](https://github.com/Pukka922/edelweiss/blob/main/docs/extension-development.md).

## Installation

On the edelweiss host, as root:

```sh
edelweissd ext install Pukka922/edelweiss-ext-system
```

or in the web interface under **Settings → Extensions → Install from GitHub** (admin mode). edelweiss shows the
release, the file and its SHA-256 together with a warning, and installs only after you confirm. Updates:
`edelweissd ext update system`, or the extensions page.

## Building

Requirements: Go 1.26, Node.js 22 with npm, make and a C compiler (`gcc`: `make test` runs the Go tests with the race
detector, which needs cgo).

```sh
make build                    # the UI, then static binaries for amd64 and arm64 in build/
make test                     # Go (race detector), vitest and the test of the release guards (scripts/)
make lint                     # go vet, gofmt, Biome, TypeScript, actionlint
make release VERSION=0.1.0    # release files in dist/: edelweiss-ext-system_0.1.0_linux_<amd64|arm64>, checksums.txt
```

The Go module requires `github.com/Pukka922/edelweiss/sdk`, and `ui/package.json` names the JS SDK by its download
URL at the edelweiss release. After the first publication both come from GitHub (the Go module proxy and
`npm ci`). Before that, build with `make dev-sdk` (next section).

## Development

Develop against an edelweiss checkout next to this repository (`../edelweiss`; `EDELWEISS_DIR=<path>` for another
place):

```sh
make dev-sdk
make build test lint
```

`make dev-sdk`:

- writes a `go.work` (git-ignored) whose `replace` points the Go SDK to `../edelweiss/sdk`;
- builds the JS SDK tarball there (`make sdk-pack`);
- installs it into `ui/node_modules` with `npm install --no-save --no-package-lock`, so `ui/package.json` stays
  unchanged and no `package-lock.json` is written.

None of this needs github.com.

- To switch back to the published SDKs, run `rm -f go.work go.work.sum && rm -rf ui/node_modules`. The next `make`
  runs `npm ci`, but only after the first publication: before it there is no `ui/package-lock.json`, and `make` asks
  for `make dev-sdk` again.
- Do not run `go mod tidy` or a plain `npm install` in `ui/` while the SDK version you require is unpublished: both
  would look it up on GitHub.

In the edelweiss dev environment, `make dev` (in `../edelweiss`) builds this repository with `make dev-sdk` and
`make build`. It then runs the extension in its container on http://localhost:9090.

- `cd ui && npm run watch` rebuilds the UI on change; the container serves `ui/dist` directly, so reload the page.
- After a change to the Go code, run `make dev-build` in `../edelweiss`.
- edelweiss's `make test-service` installs the release file built here by `make release`.

Files have LF line endings (`.gitattributes`, `.editorconfig`), also in a Windows checkout.

## Releasing

1. Set the new version in `manifest.json` and commit. It has to be a plain `x.y.z` or `x.y.z-pre` by the rules of Go (no
   leading zeros, no empty pre-release identifier, no `+build` metadata).
2. Check it with `make release-check VERSION=<version>`.
3. Push `main`, then the tag: `git push origin main`, then `git tag v<version> && git push origin v<version>`.

The release workflow (`.github/workflows/release.yml`) has two jobs, so that the build cannot use the write token.
`build` has a read-only token and keeps no credentials in the checkout. It checks the tag name (`v` and then only
letters, digits, `.`, `+` and `-`), then runs three make targets and hands `dist/` on as a workflow artifact:

- `make release-check`: the tag must match the manifest's version, a plain `x.y.z` or `x.y.z-pre`;
- `make lint test`: the checks of the CI workflow, so a tag on a commit that fails them publishes nothing;
- `make release`: the release files in `dist/`.

`publish` runs when `build` has succeeded. It holds the write token (gh needs it), builds nothing, needs neither Go nor
Node and keeps no credentials in its checkout either; only the `gh-release` step gets the token, through `GH_TOKEN`. It
downloads the artifact into `dist/` and runs `make gh-release`: it checks that `dist/` holds exactly `checksums.txt` and
the two binaries of this version, that the files match `checksums.txt` and that it lists exactly them, and creates the
GitHub release `v<version>` with `edelweiss-ext-system_<version>_linux_amd64`,
`edelweiss-ext-system_<version>_linux_arm64` and `checksums.txt`. A version with a `-` (`0.2.0-rc.1`) is published as a
GitHub pre-release: the installer takes the latest release, which leaves pre-releases out, so an rc is not offered as an
update.

A compromised build could still hand over altered files under the expected names: `checksums.txt` shows that the files
are consistent, not where they come from.

**Published versions are immutable.** Once a release may have been fetched, never move or delete the tag `v<version>`
and never replace or delete the files of the release:

- edelweiss shows users the SHA-256 of the file and refuses a release that changed after they looked at it
  (`release_changed`);
- sum.golang.org records every version of this Go module that has been fetched, so a moved tag makes `go get` fail with
  "checksum mismatch" for everyone, new fetchers included, not only for those who fetched the old one.

A broken release is fixed with a new patch version.

Rerunning a failed workflow is fine as long as the tag stays on the same commit. "Re-run failed jobs" repeats only the
job that failed, and "Re-run all jobs" builds again (the upload of the artifact replaces the one of the first attempt).
`gh release create` fails if the release already exists, so if the workflow had already created the GitHub release,
delete only the release (never the tag) before the rerun. Do that only for a release that nobody can have downloaded
yet; otherwise publish a new patch version.

The CI workflow (`.github/workflows/ci.yml`) runs `make lint test build` on every push and pull request.

## First publication

Until edelweiss publishes its SDKs, this repository has no `ui/package-lock.json`, and `go.sum` has no entry for
`github.com/Pukka922/edelweiss/sdk`. Its workflows fail until then. The order:

1. Publish edelweiss `v0.1.0` (its README, "Releasing"). Its release workflow attaches `edelweiss-sdk-0.1.0.tgz` and
   sets the tag `sdk/v0.1.0`.
2. Here, switch to the published SDKs and commit the lock data:

   ```sh
   rm -f go.work go.work.sum
   rm -rf ui/node_modules
   go get github.com/Pukka922/edelweiss/sdk@v0.1.0
   go mod tidy
   (cd ui && npm install)
   make lint test build
   git add go.mod go.sum ui/package-lock.json
   git commit -m "build: use the published edelweiss sdks v0.1.0"
   ```

   If `go get` reports an unknown revision right after the edelweiss release, the module proxy has not seen the tag
   yet. Wait a few minutes, or run `GOPROXY=direct go get github.com/Pukka922/edelweiss/sdk@v0.1.0`.
3. Create the public repository `Pukka922/edelweiss-ext-system` on GitHub. Recommended before the first tag: a tag
   ruleset (Settings → Rules → Rulesets, target: tags) that blocks deleting and updating the tags `v*`, so that a
   published version cannot be moved by mistake (see Releasing above). Then push `main` and the tag:

   ```sh
   git remote add origin https://github.com/Pukka922/edelweiss-ext-system.git
   git push -u origin main
   git tag v0.1.0
   git push origin v0.1.0
   ```

   When the release workflow has finished, `edelweissd ext install Pukka922/edelweiss-ext-system` installs the
   extension.

## License

MIT, see LICENSE.
