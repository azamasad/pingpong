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

Check a config and machines without starting the router:

```bash
./bin/pingpong doctor --config /path/to/pingpong.local.yaml
```

Copy `bin/pingpong` and a local config to Mane. Load
`deploy/com.pingpong.router.plist.example` into `~/Library/LaunchAgents/` with
absolute paths. Publish with `tailscale serve --bg 8787` (see SETUP-GUIDE).

On Firmino, `deploy/pingpong-hoststats.service.example` keeps hardware
counters running after logout. Replace `FIRMINO_IP` first (SETUP-GUIDE
section 2.8).

Validate the plist:

```bash
plutil -lint deploy/com.pingpong.router.plist.example
```