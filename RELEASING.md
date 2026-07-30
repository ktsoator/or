# Release

The Go packages and Coding share one `vX.Y.Z` version. A tag publishes the Go
module and creates a GitHub Release with Coding installers for Apple Silicon and
Intel Macs and Windows x64.

## Prepare

Add `.github/release-notes/v0.6.0.md` with the user-visible changes and any
known limitations. Then update the Coding package versions:

```sh
node scripts/set-version.mjs v0.6.0
```

Run the checks:

```sh
go test ./...
go vet ./...

cd coding/client
bun install --frozen-lockfile
bun run lint
bun run test
bun run build

cd ../desktop
bun install --frozen-lockfile
bun run build:main
bun run build:sidecar
```

Commit the version change, merge it to `main`, and wait for CI to pass.

## Publish

Tag the release commit and push the tag:

```sh
git tag -a v0.6.0 -m "or v0.6.0"
git push origin v0.6.0
```

The release workflow checks the package versions, runs the tests, builds the
macOS and Windows installers, generates `SHA256SUMS`, and creates the GitHub
Release.

Do not move a published tag. Publish a new patch version instead.

## macOS signing

The workflow ad-hoc signs the macOS bundle and verifies its internal signature
before uploading it. The installers are not signed with an Apple Developer ID
certificate and are not notarized. Configure both before public distribution.

## Windows

The workflow creates an unsigned x64 NSIS installer. Running Coding on Windows
requires Git for Windows.
