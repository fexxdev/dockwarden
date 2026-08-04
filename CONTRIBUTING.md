# Contributing

Thanks for helping improve `dockwarden`.

## Before you submit a change

Use Go 1.22 or newer. Run:

```sh
GOCACHE=/tmp/dockwarden-go-cache gofmt -w ./cmd ./internal
GOCACHE=/tmp/dockwarden-go-cache go test ./...
GOCACHE=/tmp/dockwarden-go-cache go vet ./...
```

Build the Linux target when changing platform code:

```sh
GOOS=linux GOARCH=amd64 go build ./cmd/dockwarden
```

Build the native macOS target on macOS when changing IOKit code:

```sh
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build ./cmd/dockwarden
```

## Firmware safety

Do not add tests that write to a physical dock. Use fake HTTP, command, and
HID interfaces. Keep `update` read-only and require `update --apply` for every
firmware write.

Do not commit firmware blobs, private data, credentials, or Dell Windows
executables. Validate all firmware downloads against an official Dell URL and
SHA-256 value.

## Pull requests

Keep each pull request focused. Explain the user-visible change, test result,
platform scope, and any physical hardware verification. Update
`CHANGELOG.md` when the change affects users.
