# Production go-live (V1)

Status: for the owner. Automated CI covers unit and integration tests; this is
what remains on the real machines before calling V1 production.

## 1. Prerequisites

- Phase 0 complete: [SETUP-GUIDE.md](SETUP-GUIDE.md)
- Tailscale ACL applied: [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md)
- Models installed on Mane and Firmino per [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md)

## 2. Build and install on Mane

```bash
git clone https://github.com/azamasad/pingpong.git
cd pingpong
make build-darwin-arm64
```

Copy `bin/pingpong-darwin-arm64` to a permanent path (for example
`/usr/local/bin/pingpong`). Copy `deploy/config.example.yaml` to a path **outside**
the repo (for example `~/pingpong.local.yaml`) and fill in real addresses only
there.

Install the launch agent from `deploy/com.pingpong.router.plist.example`
with absolute paths. Load it at login.

Publish:

```bash
tailscale serve --bg 8787
tailscale serve status
```

## 3. Verify

1. `curl -H 'Host: localhost' http://127.0.0.1:8787/api/health` on Mane
2. Open the page from the MacBook Air over Tailscale
3. Run manual acceptance tests in [TESTING.md](TESTING.md) section 4 (M10)
4. Confirm router RSS under 50 MB on Mane (M9b)

## 4. Rollback

- Stop the launch agent
- `tailscale serve reset` if needed
- Firmino and Mane Ollama keep running independently

## 5. After go-live

- Record measured speeds in HARDWARE-AND-MODELS section 8
- Tune `first_token_timeout` in local config if Firmino is slower than 5 tok/s