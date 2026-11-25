# Production runbook (V1 to V7, live)

Status: **live.** The router (V1 to V7 plus the Stress tab) runs on Mane and
serves the page over Tailscale. Mane and Firmino both pass
`pingpong doctor --chat`, so the real Ollama stream contract is verified
(D-052 is closed). This document is both the procedure that worked and the
checklist to repeat after a rebuild. Real addresses are not recorded here.

## 1. Where things stand

| Item | State |
|---|---|
| Router on Mane | Running as a launch agent. Listens on localhost, published by Tailscale Serve (tailnet only) |
| Mane model | `llama3.1:8b`, 100 percent GPU, about 21 tokens/s |
| Firmino model | `qwen3:30b-a3b-instruct-2507-q4_K_M`, CPU, about 12 tokens/s (D-073) |
| Mane to Firmino path | Firmino's Tailscale address (D-072) |
| Saved requests | On. SQLite in `storage.data_dir` on Mane |
| Router memory | About 24 MB resident after 24 jobs (budget 50 MB) |
| Page access | Checked by the owner from off the tailnet: it does not load (D-080) |
| Owner checks still open | Tailscale ACL for Firmino's port 11434 (ready, D-081), soak test, reboot and power-cut tests, `gpt-oss:20b` comparison (runbooks in [OPERATIONS.md](OPERATIONS.md) sections 11 to 13) |
| Known tech debt | The Ollama desktop app is still installed on Mane (TD-1, D-082). Check the port 11434 listener after every reboot |

## 2. Prerequisites

- Phase 0 complete: [SETUP-GUIDE.md](SETUP-GUIDE.md)
- Models installed on Mane and Firmino per [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md)
- Tailscale on both machines. Tailscale ACL ([NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md)) recommended

## 3. Build and install on Mane

```bash
git clone https://github.com/azamasad/pingpong.git
cd pingpong
make build-darwin-arm64
```

Copy `bin/pingpong-darwin-arm64` to a permanent path (for example
`/usr/local/bin/pingpong`). Copy `deploy/config.example.yaml` to a path **outside**
the repo (for example `~/pingpong.local.yaml`) and fill in real addresses only
there. Set Firmino's `address` to its Tailscale address.

### Saved requests (V2 and later)

Jobs, retries, the batch queue, history and the Stress tab need a data folder.
They are off until `storage.data_dir` is set.

- Use an **absolute path** that exists on Mane's internal disk, for example
  `/Users/<you>/Library/Application Support/pingpong`. Do not rely on `~`.
- Keep it out of the repository and out of any synced folder.
- Saved requests hold prompts and outputs, so the folder is private data.
- The router creates it if missing and keeps a SQLite file in it.

Profiles and aliases need nothing to start: `fast`, `balanced` and `deep` exist
by default. Add `profiles:` or `aliases:` blocks only to change them.

### Launch agents

Two user launch agents, both with absolute paths: `com.ollama.serve` (runs
`/opt/homebrew/bin/ollama serve` with the environment from
[SETUP-GUIDE.md](SETUP-GUIDE.md) section 3.3) and `com.pingpong.router` (from
`deploy/com.pingpong.router.plist.example`).

**Before loading them, make sure the Ollama desktop app is not running.**
Ideally it is also removed from Login Items. The owner has left it installed for
now (tech debt TD-1, D-082), so it may start at login and grab the port again. It holds port 11434, the launch agent then stays at "spawn
scheduled", and its settings are never applied (D-074). Check with
`lsof -nP -iTCP:11434 -sTCP:LISTEN`: the listener must be the launch agent's
`ollama`, not `Ollama.app`.

The router checks its machines at start and then every 30 seconds, so if
Ollama is a few seconds late, requests may fail for up to one health interval
and then work without a restart.

Publish:

```bash
tailscale serve --bg 8787
tailscale serve status
```

`serve status` must say "(tailnet only)". Never run `tailscale funnel`.

## 4. Verify (repeat after any rebuild)

### 4.1 Basics

1. `pingpong doctor --chat` on Mane. Expect ok for config, storage, and both
   machines, including the chat check. The speeds it prints are 2-token
   samples and are not benchmarks (D-077).
2. `curl -H 'Host: localhost' http://127.0.0.1:8787/api/health` on Mane. Expect
   `"ok":true` and `"jobs":true`.
3. `curl -H 'Host: localhost' http://127.0.0.1:8787/api/machines`. Both machines
   should show `reachable: true` and list their models.
4. Open the page from the MacBook Air over Tailscale. Check the Status tab.
5. Send one auto request and one `fast` request. Both stream text and end with a
   speed line (tokens per second).
6. Send one request to a model you have not installed. Expect
   `MODEL_NOT_INSTALLED`, not a hang.

### 4.2 Behaviours

1. **Survives the client.** Submit a long job, close the page, reopen it and see
   the result in history.
2. **Restart recovery.** Submit a job to Firmino and restart the router while it
   runs. It is stopped on purpose (`ROUTER_RESTARTING`), counts toward the
   3-hour window, and resumes after the restart from the beginning, with the
   earlier partial output kept.
3. **Retry.** Stop Ollama on Firmino, submit a job, and confirm it shows
   `retrying`. Start Ollama again and confirm it finishes.
4. **Batch.** `pingpong batch submit` with two items, then `pingpong batch get`
   until `pending` is 0.
5. **Lanes.** While a batch runs on a machine, send an interactive request. It
   should go next.
6. **Profiles and aliases.** Choose `fast` in the page. The route line should
   show Mane.
7. **Cancel.** Cancel a running job. It must stay cancelled and free the
   machine.

### 4.3 Soak and measure

Use the Stress tab to run both machines for 30 to 120 minutes, then read the
done and failed counts and speeds. Use `deploy/bench/` for single-model speed
and quality comparisons. See [OPERATIONS.md](OPERATIONS.md) section 9.

### 4.4 Memory

Confirm router RSS under 50 MB on Mane (`ps -o rss= -p <pid>`, or the Status
tab).

## 5. Backups

Run `pingpong backup` on Mane (same config path as the router). It copies the
SQLite store safely while the router runs, and the copy is private data. Never
put it in the repository. See [V6-OPERABILITY.md](V6-OPERABILITY.md).

## 6. Rollback

Before installing any V7.1 to V7.9 build, run `pingpong backup` and keep the copy
private. New versions only add tables and columns, so an older binary still
opens the file, but it does not know the new rules: after V7.5 an older binary
would sweep thread jobs at 90 days, and after V7.7 it ignores schedules. Roll
back only to a build from before the version you want to leave, or restore the
backup.

- Stop the launch agent
- `tailscale serve reset` if needed
- Firmino and Mane Ollama keep running independently
- The data folder can stay. A later start picks up where it left off

## 7. Lessons from the first go-live

| Problem | Cause | Fix |
|---|---|---|
| Mane could not reach Firmino (`prohibited`) | Different subnets, gateway blocks traffic between them | Ollama on Firmino bound to its Tailscale address, config uses it (D-072) |
| Ollama on Firmino listed no models | The service runs as `ollama` and reads `/usr/share/ollama/.ollama/models`, while models were pulled as root | Install with the official script, set the override file, pull again |
| `ollama.service` did not exist | Ollama had been run by hand, not installed as a service | Official install script |
| Mane's Ollama settings had no effect | Ollama desktop app held port 11434, launch agent stuck at "spawn scheduled" | Quit the app, kickstart the agent (D-074) |
| Benchmark failed with `connection refused` | It was run on Firmino, but the router is on Mane | Run it on Mane |
| Shell errors on pasted commands | `<` and `>` in placeholders are redirection, `set -e` ends an SSH session | Replace placeholders without brackets, avoid `set -e` in interactive shells |
| `doctor --chat` speeds looked wrong | They come from a 2-token reply | Use the bench scripts (D-077) |
| Dense 14B on Firmino was slow (2.1 tokens/s) | CPU memory bandwidth with mixed memory | Mixture-of-experts 30B model: 11.9 tokens/s (D-073) |

## 8. After go-live, ongoing

- Re-measure after any hardware, model or Ollama version change, and update
  [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 8.
- Read the router log now and then ([OPERATIONS.md](OPERATIONS.md) section 10).
- Consider lowering `first_token_timeout`, now that the measured prompt reading
  speed on Firmino is about 30 tokens/s.
