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
| `listen` | all interfaces, configurable port | Where the router accepts clients. Reachability is restricted by the firewall and Tailscale |
| `allowed_hosts` | Mane's Tailscale name and `localhost` | Accepted `Host` headers (see API security notes) |
| `max_body_bytes` | modest limit, for example a few MB | Largest accepted request body |
| `log_level` | `info` | Logging detail |

### Machines (one entry per machine)

| Setting | Meaning |
|---|---|
| `name` | Registry name, for example `mane`, `firmino`. Used in pins |
| `address` | Base URL of that machine's Ollama on the LAN |
| `roles` | List of roles, for example `fast`, `heavy` |
| `default_model` | Model used when none is pinned |
| `max_concurrent` | Requests in flight at once on this machine. Default `1` |
| `num_ctx` | Optional context window size passed to Ollama. Unset means Ollama's default |

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
measuring.

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

These are settings on each machine's Ollama, not the router. See
[OPERATIONS.md](OPERATIONS.md) for how to apply them.

| Setting | Suggested value | Why |
|---|---|---|
| Listen address (Firmino) | LAN interface | So Mane can reach it. Firewall to Mane only |
| Listen address (Mane) | Local only | Only the router uses it |
| Parallel requests | 1 | Matches `max_concurrent = 1` and keeps CPU inference responsive |
| Loaded models limit | 1 on Firmino. 2 on Mane (the 3B and 8B) | Memory control |
| Keep-alive | Long enough to avoid reload between requests, short enough to free memory | Tune after measuring |

## 4. Illustrative example

Placeholders only. Do not commit real addresses.

```yaml
router:
  listen: ":PORT"
  allowed_hosts: ["mane", "localhost"]

machines:
  - name: mane
    address: "http://MANE_LAN_ADDRESS:11434"
    roles: [fast]
    default_model: "<8b-model-name>"
    max_concurrent: 1

  - name: firmino
    address: "http://FIRMINO_LAN_ADDRESS:11434"
    roles: [heavy]
    default_model: "<14b-model-name>"
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
