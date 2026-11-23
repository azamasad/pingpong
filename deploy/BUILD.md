# Building Pingpong

## Linux (CI and Firmino)

```bash
make build
# binary: bin/pingpong
```

## macOS (Mane, Apple silicon)

On Mane or a cross-compile host:

```bash
GOOS=darwin GOARCH=arm64 make build
```

Copy `bin/pingpong` and a local config to Mane. Load
`deploy/com.pingpong.router.plist.example` into `~/Library/LaunchAgents/` with
absolute paths. Publish with `tailscale serve --bg 8787` (see SETUP-GUIDE).

Validate the plist:

```bash
plutil -lint deploy/com.pingpong.router.plist.example
```