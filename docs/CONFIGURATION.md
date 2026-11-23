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
| `listen` | `127.0.0.1:8787` (the port is configurable) | Where the router accepts connections. Localhost only. Tailscale Serve publishes it to the tailnet (see [SETUP-GUIDE.md](SETUP-GUIDE.md) section 4), so it is never open to the LAN |
| `config_path` | `--config` flag, else the `PINGPONG_CONFIG` environment variable | Where to read the config file. Required, no hidden default |
| `allowed_hosts` | Full tailnet name (`mane.<tailnet>.ts.net`), `localhost`, `127.0.0.1` | Accepted `Host` names, and the host part of `Origin` (see API.md section 5). The short name `mane` is not what browsers send, so list the full name |
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
| `num_ctx` | Context window size, in tokens. Default `8192`. The router sends it with **every** request, so the machine never silently falls back to Ollama's built-in 4096, and so the router can detect when a prompt filled the window. Keep it equal to the machine's `OLLAMA_CONTEXT_LENGTH` |

### Routing

| Setting | Default | Meaning |
|---|---|---|
| `auto_role` | `heavy` | Role that auto requests go to |
| `fast_role` | `fast` | Role that `fast` requests go to |

### Health and registry

| Setting | Default | Meaning |
|---|---|---|
| `health_interval` | 30 seconds | How often each machine is checked |
| `health_timeout` | 10 seconds | How long one check may take. Generous because a busy CPU-bound machine answers slowly |
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
token limit, 8 tokens per second takes about 17 minutes and 5 tokens per second
about 27 minutes, which is why the default is 30. The prompt speed on Firmino is
an estimate until measured, so re-check this after the measurements.

| Setting | Default | Meaning |
|---|---|---|
| `connect_timeout` | 5 seconds | Opening a connection to a machine |
| `first_token_timeout` | 30 minutes | Wait for the first chunk of any kind from Ollama, covering model load and reading the prompt |
| `idle_token_timeout` | 5 minutes | Longest gap between chunks once streaming |
| `max_run_time` | 2 hours | Longest a single request may run, so one stuck request cannot block a machine forever |
| `heartbeat_interval` | 15 seconds | While a stream is open and nothing else was sent, send a `ping` event this often. Keeps proxies and browsers from closing a quiet connection, and detects dead clients |
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

These are settings on each machine's Ollama, not the router. (`OLLAMA_HOST` on
Firmino is the LAN address only, because Tailscale lets tailnet traffic past the
Linux firewall, so the bind address is what keeps Ollama off the tailnet.) They come from
Ollama's own documentation (verified November 2026). See
[SETUP-GUIDE.md](SETUP-GUIDE.md) for how to apply them on each machine.

| Setting | Firmino | Mane | Why |
|---|---|---|---|
| `OLLAMA_HOST` | `<FIRMINO_LAN_IP>:11434` (its LAN address only, not `0.0.0.0`), plus a firewall allowing only Mane | `127.0.0.1:11434` | Firmino must be reachable by Mane and by nothing else. Binding to the LAN address means it does not listen on the tailnet address at all. Mane's Ollama is only for the router |
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
- On Firmino, the Ollama command line also talks to `OLLAMA_HOST`, so run
  `export OLLAMA_HOST=<FIRMINO_LAN_IP>:11434` in your shell before using `ollama pull`
  or `ollama ps` there.
- A fixed LAN address matters even more for this setting: if the address
  changes, Ollama cannot start. Keep the DHCP reservation.

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

## 5. Validation (the router refuses to start on any of these)

- Machine names are unique and non-empty.
- Each `address` is a valid `http` URL.
- Each machine has a non-empty `default_model`, `max_concurrent` of at least 1,
  and `num_ctx` above 0.
- `auto_role` and `fast_role` each name a role that at least one machine has. One
  machine may have both roles.
- If several machines share a role, V1 uses the first in file order and logs a
  warning.
- Durations parse, and `first_token_timeout` is not shorter than
  `idle_token_timeout`.

## 6. Rules for changes

- Adding a setting means documenting it here with a default.
- A setting must have a reason. Do not add knobs for their own sake (P2).
