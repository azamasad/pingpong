# Configuration

Status: v2, all keys built. A specification of what is configurable and the defaults.
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
| `address` | Base URL of that machine's Ollama. For Mane itself this is localhost. For other machines it is their private address: the LAN address on a shared subnet, or the Tailscale address (used for Firmino, D-072) |
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
about 27 minutes, which is why the default is 30. The measured prompt speed on
Firmino with the 30B model is about 30 to 35 tokens per second, so 8,192 tokens
take 4 to 5 minutes, and the default of 30 minutes is generous. It can be
lowered (one run measured, re-check first).

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
| `data_dir` | A directory on Mane's disk (a leading `~/` is expanded to the home folder) | Where saved requests live |
| `retention_days` | 90 | Finished results older than this are removed |
| `retention_sweep_interval` | Daily | How often removal runs |
| `output_flush_interval` | About 1 second | How often streamed output is saved, to limit disk writes |

## 3. Environment on the workers (Ollama)

These are settings on each machine's Ollama, not the router. (`OLLAMA_HOST` on
Firmino is its Tailscale address, because Mane and Firmino are on different
subnets. Tailscale lets tailnet traffic past the Linux firewall, so a Tailscale
ACL is what limits who can reach it, D-072.) They come from
Ollama's own documentation (verified November 2026). See
[SETUP-GUIDE.md](SETUP-GUIDE.md) for how to apply them on each machine.

| Setting | Firmino | Mane | Why |
|---|---|---|---|
| `OLLAMA_HOST` | `<FIRMINO_TAILSCALE_IP>:11434` (its Tailscale address, not `0.0.0.0`) | `127.0.0.1:11434` | Mane must reach Firmino, and nothing else should. Binding to the Tailscale address keeps it off the home LAN. The ACL then limits the tailnet to Mane. Mane's Ollama is only for the router |
| `OLLAMA_NUM_PARALLEL` | `1` | `1` | Matches `max_concurrent = 1`. Set explicitly because on Apple GPUs Ollama may choose its own value |
| `OLLAMA_MAX_LOADED_MODELS` | `1` | `2` | Mane holds the 8B and the 3B. Firmino holds the 30B mixture-of-experts model |
| `OLLAMA_KEEP_ALIVE` | `30m` | `30m` | Avoids reloading between requests. Default is 5 minutes. Tune |
| `OLLAMA_CONTEXT_LENGTH` | `8192` | `8192` | Explicit. Default is 4096 |
| `OLLAMA_NO_CLOUD` | `1` | `1` | Keeps everything local |
| `OLLAMA_MAX_QUEUE` | leave default | leave default | The router's own gate does the queuing, so this should rarely matter |

Notes:

- On macOS, set these in the service definition that launches Ollama. Using
  `launchctl setenv` does not survive a reboot.
- On Linux, set them with a systemd override so they survive upgrades.
- On Firmino, the Ollama command line also talks to `OLLAMA_HOST`, so run
  `export OLLAMA_HOST=<FIRMINO_TAILSCALE_IP>:11434` in your shell before using `ollama pull`
  or `ollama ps` there.
- A stable address matters for this setting: if the address changes, Ollama
  cannot start. A Tailscale address is stable for a machine. A LAN address
  needs a DHCP reservation.

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
    address: "http://FIRMINO_TAILSCALE_ADDRESS:11434"
    roles: [heavy]
    default_model: "qwen3:30b-a3b-instruct-2507-q4_K_M"
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
- If several machines share a role, the router logs a warning and V4 chooses
  the least loaded machine that is up (see ROUTING).
- Durations parse, and `first_token_timeout` is not shorter than
  `idle_token_timeout`.
- `router.listen` is a valid `host:port` whose host is `127.0.0.1` or `localhost`.
- Not checked today: that a profile's `machine` names a real machine. A typo
  fails only when a request uses that profile. V7 adds this check.

Also built: `router.shutdown_grace` (V6, default `10s`) and `aliases` (V7). See
[V6-OPERABILITY.md](V6-OPERABILITY.md) and [V7-ALIASES.md](V7-ALIASES.md).

Planned keys, not built (full contract in
[NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md)). All are optional,
and all except `openai` and `notify` need `storage.data_dir`. Schedules, templates
and threads are stored in the database, never in the config file.

| Key | Version | Default | Meaning |
|---|---|---|---|
| `batches.max_items` | V7.1 | `500` | Most items in one batch (default 500 after D-118 memory gate; may set `1000` after Mane RSS check) |
| `templates.max_bytes` | V7.3 | `16384` | Largest template body |
| `templates.max_count` | V7.3 | `200` | Most templates |
| `threads.max_turns` | V7.5 | `200` | Most turns in one thread |
| `files.max_bytes` | V7.6 | `2097152` | Largest uploaded text file (the upload endpoint has its own body limit of this plus 64 KB) |
| `files.chunk_chars` | V7.6 | `6000` | Default chunk size in characters (a request may use 500 to 20000) |
| `schedules.max_count` | V7.7 | `50` | Most schedules |
| `schedules.grace` | V7.7 | `1h` | How late a missed run may still run once (`missed: run_once`) |
| `openai.enabled` | V7.8 | `false` | Turns on `/v1/models` and `/v1/chat/completions` |
| `notify.webhook_url` | V7.9 | empty (off) | One outbound URL. Real value only in the real config file, never committed or logged |
| `notify.events` | V7.9 | `[batch_done, schedule_failed, job_failed]` | Which events send a message |
| `notify.timeout` | V7.9 | `10s` | Per message |

Validation to add with each key: integers above 0, `files.chunk_chars` within
500 to 20000, `schedules.grace` a positive duration, `notify.webhook_url` an
`http` or `https` URL when set. A key that needs storage while `data_dir` is
unset is a startup error naming the key.

## 6. Rules for changes

- Adding a setting means documenting it here with a default.
- A setting must have a reason. Do not add knobs for their own sake (P2).
