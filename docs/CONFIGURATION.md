# Configuration

Status: draft v1. A specification of what is configurable and the defaults.
The exact file format is chosen at implementation, but the setting names and
meanings below should be kept stable. An illustrative example is included at
the end.

Related: [ARCHITECTURE.md](ARCHITECTURE.md), [ROUTING.md](ROUTING.md),
[JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md).

## 1. Principles

- One small config file on Mane. No config service, no database.
- Sensible defaults. The file lists only what differs.
- Machines are data in this file, not hard-coded (constitution P12, P13).
- Nothing secret is needed. Do not store keys in it. (The repo is public.)
- The real config file with real addresses is **not committed**. Commit only an
  example with placeholders.

## 2. Settings

### Router

| Setting | Default | Meaning |
|---|---|---|
| `listen` | `127.0.0.1` and a configurable port | Where the router accepts connections. Localhost only. Tailscale Serve publishes it to the tailnet (see [SETUP-GUIDE.md](SETUP-GUIDE.md) section 4), so it is never open to the LAN |
| `allowed_hosts` | Mane's tailnet name and `localhost` | Accepted `Host` headers (see API security notes) |
| `max_body_bytes` | modest limit, for example a few MB | Largest accepted request body |
| `log_level` | `info` | Logging detail |

### Machines (one entry per machine)

| Setting | Meaning |
|---|---|
| `name` | Registry name, for example `mane`, `firmino`. Used in pins |
| `address` | Base URL of that machine's Ollama. For Mane itself this is localhost. For other machines it is their LAN address |
| `roles` | List of roles, for example `fast`, `heavy` |
| `default_model` | Model used when none is pinned |
| `max_concurrent` | Requests in flight at once on this machine. Default `1` |
| `num_ctx` | Optional context window size passed to Ollama with each request. Unset means the worker's own setting (`OLLAMA_CONTEXT_LENGTH`, 8192 in the setup guide). Ollama's built-in default is only 4096, which silently cuts long prompts |

### Routing

| Setting | Default | Meaning |
|---|---|---|
| `auto_role` | `heavy` | Role that auto requests go to |
| `fast_role` | `fast` | Role that `fast` requests go to |

### Health and registry

| Setting | Default | Meaning |
|---|---|---|
| `health_interval` | 30 seconds | How often each machine is checked |
| `health_failures_to_mark_down` | 2 | Consecutive failed checks before a machine is marked down. A request-time connection failure marks it down immediately |
| `registry_refresh_interval` | 5 minutes | How often installed models are re-read |
| `refresh_on_model_miss` | on | If a pinned model is not found, refresh once before failing |

### Timeouts

These are starting values. CPU inference on Firmino can take minutes before the
first token on long prompts, so they are deliberately generous. Tune after
measuring (see [SETUP-GUIDE.md](SETUP-GUIDE.md) section 5).

Rule of thumb for `first_token_timeout`: it must exceed the time to read the
largest allowed prompt on the slowest machine, that is the context limit divided
by the measured prompt speed, plus model load time, plus margin. At an 8,192
token limit and an assumed 8 tokens per second that is about 17 minutes, which
is why the default is 20.

| Setting | Default | Meaning |
|---|---|---|
| `connect_timeout` | 5 seconds | Opening a connection to a machine |
| `first_token_timeout` | 20 minutes | Wait for the first token, covering model load and prompt processing |
| `idle_token_timeout` | 5 minutes | Longest gap between tokens once streaming |
| `max_wait` | none | Longest a request may wait for a busy machine. None means it waits as long as the client stays (V1) |

### Retry (V2)

| Setting | Default | Meaning |
|---|---|---|
| `retry_interval` | 2 minutes | Time between attempts |
| `retry_window` | 3 hours | Total time from the first failure before giving up. Not reset by later failures |

### Storage and retention (V2)

| Setting | Default | Meaning |
|---|---|---|
| `data_dir` | A directory on Mane's disk | Where saved requests live |
| `retention_days` | 90 | Finished results older than this are removed |
| `retention_sweep_interval` | Daily | How often removal runs |
| `output_flush_interval` | About 1 second | How often streamed output is saved, to limit disk writes |

## 3. Environment on the workers (Ollama)

These are settings on each machine's Ollama, not the router. They come from
Ollama's own documentation (verified November 2026). See
[SETUP-GUIDE.md](SETUP-GUIDE.md) for how to apply them on each machine.

| Setting | Firmino | Mane | Why |
|---|---|---|---|
| `OLLAMA_HOST` | `0.0.0.0:11434`, with a firewall allowing only Mane | `127.0.0.1:11434` | Firmino must be reachable by Mane. Mane's Ollama is only for the router |
| `OLLAMA_NUM_PARALLEL` | `1` | `1` | Matches `max_concurrent = 1`. Set explicitly because on Apple GPUs Ollama may choose its own value |
| `OLLAMA_MAX_LOADED_MODELS` | `1` | `2` | Mane holds the 8B and the 3B. Firmino holds the 14B |
| `OLLAMA_KEEP_ALIVE` | `30m` | `30m` | Avoids reloading between requests. Default is 5 minutes. Tune |
| `OLLAMA_CONTEXT_LENGTH` | `8192` | `8192` | Explicit. Default is 4096 |
| `OLLAMA_NO_CLOUD` | `1` | `1` | Keeps everything local |
| `OLLAMA_MAX_QUEUE` | leave default | leave default | The router's own gate does the queuing, so this should rarely matter |

Notes:

- On macOS, set these in the service definition that launches Ollama. Using
  `launchctl setenv` does not survive a reboot.
- On Linux, set them with a systemd override so they survive upgrades.

## 4. Illustrative example

Placeholders for addresses only. Do not commit real addresses. Model names
are the starting models from [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md).

```yaml
router:
  listen: "127.0.0.1:PORT"
  allowed_hosts: ["MANE_TAILNET_NAME", "localhost"]

machines:
  - name: mane
    address: "http://127.0.0.1:11434"
    roles: [fast]
    default_model: "llama3.1:8b"
    max_concurrent: 1

  - name: firmino
    address: "http://FIRMINO_LAN_ADDRESS:11434"
    roles: [heavy]
    default_model: "qwen2.5:14b"
    max_concurrent: 1

routing:
  auto_role: heavy
  fast_role: fast

retry:            # V2
  retry_interval: 2m
  retry_window: 3h

storage:          # V2
  data_dir: "/path/on/mane"
  retention_days: 90
```

## 5. Rules for changes

- Adding a setting means documenting it here with a default.
- A setting must have a reason. Do not add knobs for their own sake (P2).
