# V6: Operability

Status: **planned, not built.** Do not code this before the real-machine trial
of V1 to V5 (see [PRODUCTION.md](PRODUCTION.md)) has been run and its findings
are folded in here.

Related: [ROADMAP.md](ROADMAP.md) phase 6, [OPERATIONS.md](OPERATIONS.md),
[TESTING.md](TESTING.md), decision D-069.

## 1. Why

Everything built so far was tested against a fake Ollama. The first run of the
real binary still found four deploy bugs (a `~` path, a config default, a
rewritten error, a boot race). V6 gives the owner a way to find that kind of
problem in seconds on the real machines, and to run Pingpong unattended with
fewer surprises.

V6 adds no new routing behaviour and no new request types.

## 2. Scope

| # | Feature | What the owner gets |
|---|---|---|
| 1 | `pingpong doctor` | One command that checks config, data folder, and both machines, and says what to fix |
| 2 | Graceful shutdown | Restarting the router does not lose or garble running work |
| 3 | `pingpong backup` | A consistent copy of saved requests, safe while the router runs |
| 4 | Status view | One page and one JSON endpoint showing machines, queue and recent failures |

## 3. `pingpong doctor`

```text
pingpong doctor --config PATH [--chat] [--json]
```

Runs locally on the router machine. It reads the same config file as the
router and does not need the router to be running.

### Checks

| Group | Check | Result |
|---|---|---|
| Config | File loads and validates | fail if invalid |
| Config | Router listens on localhost only | fail otherwise (constitution P7, D-040) |
| Config | Each role named in `routing` has at least one machine | fail |
| Config | Every profile (and, from V7, alias) points at a real machine | fail |
| Storage | `data_dir` set, exists or can be created, writable | fail if not; warn if jobs are off |
| Storage | Free disk space under `data_dir` | warn below 5 GB, fail below 500 MB |
| Machine | Reachable (`/api/tags` answers within the health timeout) | fail if not |
| Machine | Ollama version is reported | info |
| Machine | Default model is installed | fail if not (same rule as `MODEL_NOT_INSTALLED`) |
| Machine | Models listed that no role or profile uses | info |
| Machine | With `--chat`: a tiny real chat completes (see below) | fail if the stream shape is wrong |

### The `--chat` check

This closes the long-standing gap in D-052: the router has never been proven
against a real Ollama stream.

- Sends one fixed prompt, "Reply with the single word OK.", with a small
  output limit, to each machine's default model.
- Not the owner's content, so P16 is not affected. Nothing is logged or stored.
- Verifies that the NDJSON stream has the fields the router depends on:
  `message.content` (and `message.thinking` when present), a final chunk with
  `done: true`, and `eval_count` and `prompt_eval_count`.
- Reports time to first token and tokens per second as information. These are
  the owner's own measurements that replace the community figures (D-044).
- It is opt-in because it loads a model and may take minutes on Firmino.

### Output and exit codes

- Human output: one line per check, `ok`, `warn`, `fail` or `info`, with a
  short fix hint on any non-ok line.
- `--json`: the same results as an array, for scripts.
- Exit code `0` if there are no failures, `1` if any check failed, `2` if the
  command itself could not run (for example the config file is missing).
- Warnings do not change the exit code.
- Machine addresses are printed only to the local terminal. Nothing is
  written to a file (P17).

## 4. Graceful shutdown

On `SIGTERM` or `SIGINT` the router:

1. Stops accepting new requests. `POST` endpoints return `503` with a new
   retryable error code `ROUTER_RESTARTING` (add to the error table in
   [API.md](API.md)). `GET` endpoints keep working until the end.
2. For **jobs** (V2 and later): lets each running job flush its saved output,
   then marks it `retrying` with reason `router_restarting`. This is the same
   state restart recovery already uses, so no new lifecycle is needed. The
   retry window is not reset and not consumed by a restart.
3. For **live `POST /api/chat` streams**: sends an in-band `error` event with
   `ROUTER_RESTARTING` and closes. These are connection-tied by design (D-064).
4. Closes the SQLite store cleanly (checkpoint the write-ahead log).
5. Exits, after at most `shutdown_grace` (default `10s`). Anything still
   running at that point is cancelled and handled as in step 2 or 3.

New config key: `router.shutdown_grace`.

Launchd already sends `SIGTERM` on unload, so the plist needs no change.

## 5. `pingpong backup`

```text
pingpong backup --config PATH --to DIR
```

- Reads `storage.data_dir` from the config.
- Writes `DIR/pingpong-YYYYMMDD-HHMMSS.db`, a single consistent file made with
  SQLite's `VACUUM INTO`. This is safe while the router is running and does not
  pause it.
- Prints the path and size. Refuses to overwrite an existing file.
- No scheduling and no pruning. The owner runs it from `cron` or `launchd` if
  wanted, and prunes old copies by hand (P1).
- Restore is documented, not automated: stop the router, copy the file to
  `data_dir/pingpong.db`, delete any `-wal` and `-shm` files, start the router.
- Backups hold prompts and outputs, so they are private data. The command
  creates the file with mode `0600`. Never commit them (P17).

## 6. Status view

### `GET /api/status`

Read-only JSON, subject to the same Host and Origin checks as every endpoint.

| Field | Content |
|---|---|
| `version`, `uptime_seconds` | Router build and how long it has run |
| `memory_bytes` | Go runtime memory in use (`runtime.MemStats.Sys`). Not the same as RSS, and labelled as such |
| `machines[]` | For each machine: name, roles, reachable, in-flight, waiting, loaded models, last checked |
| `jobs` | Counts by status and by lane. Age in seconds of the oldest `waiting` job |
| `recent_failures[]` | Last 20 failed or retrying jobs: job id, time, error code, machine. **No prompt or output text** (P16) |
| `storage` | `data_dir` free bytes, database size in bytes |

When jobs are disabled, `jobs`, `recent_failures` and `storage` are `null`.

### Page

A new "Status" tab on the existing page, with no new assets or framework. It
polls `/api/status` every 5 seconds only while the tab is visible, so an idle
page costs nothing.

## 7. Configuration added

| Key | Default | Meaning |
|---|---|---|
| `router.shutdown_grace` | `10s` | Maximum time to finish flushing on shutdown |

No other new keys. `doctor`, `backup` and the status view need none.

## 8. Not in scope

- Metrics systems (Prometheus, OpenTelemetry) and dashboards (P2).
- Alerting and notifications. These are V10.
- Hot reload of config. A restart is a few seconds and now graceful.
- Authentication. Access control stays in Tailscale (D-061).
- Automatic backup schedules or off-machine copies.

## 9. Milestones

| ID | Who | Work | Done when |
|---|---|---|---|
| M11a | Agent | `pingpong doctor` without `--chat`, against the fake Ollama | Every check has a passing and a failing test. Exit codes verified |
| M11b | Agent | `--chat` check, including a stream with a wrong shape | Fake with a malformed stream is reported as a failure |
| M12 | Agent | Graceful shutdown with `ROUTER_RESTARTING` | Test: a running job is `retrying` after shutdown and finishes after restart |
| M13 | Agent | `pingpong backup` | Test: backup taken during writes opens and has consistent rows. Mode is `0600` |
| M14 | Agent | `/api/status` and the Status tab | Test: shape, no content fields, `null` when jobs are off. RSS still under 50 MB |
| M15 | Owner | Run `doctor --chat` on Mane and Firmino. Restart the router during a job. Take a backup | All three behave as described. Results copied into [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) |

## 10. Automated tests

Added to [TESTING.md](TESTING.md) when built.

- Doctor: one test per check, each in an ok and a fail variant. No test may
  need a real machine.
- Doctor `--json` output parses and matches the human result.
- Shutdown: a job running at `SIGTERM` is `retrying`, keeps its partial output,
  and does not lose retry time already used. A live chat gets `ROUTER_RESTARTING`.
  New `POST` requests during shutdown get `503`.
- Backup: concurrent writer during backup, restored copy passes
  `PRAGMA integrity_check`, file mode is `0600`, existing file is not overwritten.
- Status: no prompt or output text appears anywhere in the response (a test
  submits a recognisable string and asserts it is absent). Works with jobs off.
- Log scan: starting, running and stopping the router with a recognisable
  prompt produces no log line containing it (P16, N9 stay green).

## 11. Open questions

- Doctor thresholds for free disk (5 GB warn, 500 MB fail) are guesses. Adjust
  after the owner sees real database growth.
- Whether the status tab should show Ollama's loaded-model memory use. Only if
  Ollama's `/api/ps` already returns it in a stable form on the real machines.
