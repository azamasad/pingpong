# Architecture

Status: v3. Built and live. Language-neutral in the design. The chosen language (Go) is in
[IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md).

Related: [API.md](API.md), [CONFIGURATION.md](CONFIGURATION.md),
[ROUTING.md](ROUTING.md), [JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md).

## 1. Overview

```
        MacBook Air (client)
                |
                |  Tailscale (private network)
                v
   +-----------------------------+
   |  Mane  (Mac mini M4)        |
   |  - Router (the brain)       |
   |  - Tiny web page            |
   |  - Registry and health      |
   |  - Per-machine wait gate    |
   |  - Saved requests (V2)      |
   |  - Ollama (<= 8B models)    |
   +--------------+--------------+
                  |
                  |  Tailscale (private, no public internet)
                  v
   +-----------------------------+
   |  Firmino  (Ubuntu laptop)   |
   |  - Ollama (30B MoE model)   |
   |  - No UI, no router         |
   +-----------------------------+

   Future machines attach the same way, via Mane.
```

## 2. Components

All of these are inside **one small program** on Mane (plus Ollama), to keep
memory use and moving parts low.

### Router

The only place routing decisions are made. It:

1. Accepts requests from the client (web page or terminal).
2. Reads the flags and any pinned model or machine.
3. Consults the registry to pick the target (pure decision, no side effects).
4. Passes the request through the wait gate for that machine.
5. Forwards the request to Ollama on that machine.
6. Streams the answer back, tagged with the model and machine used.
7. Tracks the request status (waiting, running, done, failed).

It follows [ROUTING.md](ROUTING.md). It never inspects the meaning of a prompt.

### Tiny web page

Served by the router itself. Static and minimal, with all assets local and
embedded in the program. No separate UI server. Details in
[UI-SPEC.md](UI-SPEC.md).

### Registry and health

A list of machines from configuration, plus what Mane learns by asking each
machine. For each machine:

| Field | Meaning |
|---|---|
| Name | For example `mane`, `firmino` |
| Address | Private address used for traffic (Tailscale address for Firmino) |
| Roles | For example `fast`, `heavy`. Rules refer to roles |
| Default model | Used when none is pinned |
| Installed models | Read from the machine's Ollama |
| Loaded models | Informational, from the machine's Ollama |
| Reachable | Result of the latest health check |

Health checks and model refresh run on a schedule (see CONFIGURATION). The
registry is refreshed once more when a pinned model is not found.

Routing rules say "auto goes to the `heavy` role", not "auto goes to
Firmino". Today Firmino is the only `heavy` machine. When another is added,
the rules do not change. How to choose between several machines of one role is
a later decision (see ROADMAP).

### Wait gate

One small gate per machine. It enforces the machine's concurrency limit
(default 1) and holds extra requests in a first-come first-served list. It
also reports the busy and waiting counts shown in the page. If a client
disconnects, its entry leaves the list, and a running request is cancelled
upstream.

This gate is deliberately simple. It is the seed of the V3 queue, but it is not
a queue: nothing is saved, and there are no lanes.

### Ollama client

Talks to each machine's Ollama over HTTP: stream a chat, list installed
models, list loaded models, and cancel by closing the connection. Applies the
timeouts in CONFIGURATION.

### Inference servers (Ollama)

One Ollama instance per machine. Mane runs models up to 8B. Firmino runs a
30B mixture-of-experts model. Firmino's Ollama listens on its Tailscale address
and should be restricted to Mane by a Tailscale ACL (D-072). Mane's Ollama is
local only.

### Planned additions (V7.1 to V7.9, not built)

All fit the existing shape: more saved-request tables and jobs, no new
services. Threads (V7.5) group jobs. Split (V7.1), checking (V7.2) and file input
(V7.6) create linked jobs with explicit machine and model pairs. Templates (V7.3)
and schedules (V7.7) add definitions. Export (V7.4) streams rows. Details and
memory notes: [NEXT-FEATURES-PRDS.md](NEXT-FEATURES-PRDS.md). Exact tables,
columns and endpoints: [NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md).

| Addition | Where it lives | New moving part |
|---|---|---|
| `batches`, `templates`, `threads`, `schedules`, `schedule_runs` tables, and extra `jobs` columns (`parent_id`, `stage`, `check_pending`, `thread_id`, `turn`) | The existing SQLite file | None. Additive migrations |
| Stage jobs (check, reduce) | Created by a hook when a job or the last job of a batch reaches a final state, and re-created at start if a restart interrupted it | A small function in the job service, not a new service |
| Scheduler | One timer goroutine in the router | One timer for all schedules |
| Upload and export | HTTP handlers that stream one chunk or one row at a time | None |
| OpenAI endpoint (V7.8, draft) | Another handler over the same engine as `POST /api/chat` | None |
| Webhook (V7.9, draft) | One outbound HTTP call per event, best effort | None. Needs owner approval first |

Nothing of ours runs on Firmino for any of them.

### Saved-request store (V2)

A small record per request, stored on Mane's disk, as described in
[JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md). Results are kept 90 days. An
embedded database file (SQLite recommended) or plain files, never a database
server, to meet the RAM budget.

Not built in V1, but nothing in V1 should prevent it. In particular, keep the
"run a request" logic independent of the HTTP connection, so V2 can run it in
the background.

## 3. Request flow

### V1 (request tied to the connection)

1. Client sends a request to Mane with optional flags.
2. Router validates it and selects the target (see ROUTING).
3. If the target is marked down, return an HTTP error. Nothing has been written yet.
4. Router writes the `200` header and the `route` event, so the client can show
   the target at once.
5. The request enters the target's wait gate. If the machine is at its limit, it
   waits and the client sees "waiting". Heartbeats keep the connection alive.
6. Router forwards to the target's Ollama and streams the answer back with the
   model, machine and mode. Any failure from here on is an in-band `error`.
7. If the client disconnects at any point, the request is dropped or cancelled.

Go implementation note: do not use a server write timeout or a client total
timeout on streaming calls, because they would cut off 30-minute streams. Use the
router's own timers (first chunk, idle, total run time). Any response-header
timeout on the call to Ollama must be at least `first_token_timeout`.

### V2 (durable)

1. Client submits the request. Router validates it, saves it, and returns an ID.
2. Router runs the request in the background, independent of the client.
3. Output and status updates are saved as they happen.
4. The client can attach, detach and come back later using the ID.
5. Retry and cancel rules apply (see JOBS-AND-RETRIES).

## 4. Network paths

| Path | Transport | Purpose |
|---|---|---|
| MacBook Air to Mane | Tailscale | The only client entry point |
| Mane to Firmino | Tailscale (LAN if on the same subnet) | Inference traffic |
| Admin to Firmino | Tailscale SSH | Maintenance only, not inference |

Details in [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md).

## 5. Memory

| Item | Budget |
|---|---|
| Router, page, registry, wait gate, store (Mane) | Working budget 50 MB |
| Models on Mane | 8B or smaller, with room for the OS and context |
| Firmino | Not constrained by the brain, since it does not run it |

## 6. Deployment

| Item | Notes |
|---|---|
| Where | One program on Mane |
| How it runs | As a user launch agent (`launchd` on macOS) that starts at automatic login and restarts on failure |
| Exposure | Listens on localhost only. Tailscale Serve publishes it to the tailnet |
| Configuration | A config file on Mane (real values are not committed) |
| Data (V2) | A data directory on Mane's disk |
| Updates | Replace the program and restart. Active requests in V1 are lost on restart. In V2 waiting and retrying requests resume, and an interrupted attempt restarts from the beginning |
| Workers | Ollama only, with firewall rules. Nothing of ours runs on Firmino |

## 7. Logging

- Logs go to the service's log output, with rotation handled by the host.
  Besides errors, one metadata line is written per finished request or job
  (speeds, outcome, never content), for tuning (D-076).
- Log request metadata only, as one line per event with these fields and no
  others: time, level, event name, request ID (V2), mode, machine, model, state,
  duration, error code, and error message text from the router itself.
  Never log a request body, prompt, output, or Ollama's raw error body (it can
  echo prompt text).
- **Never log prompts or outputs.**
- Keep logs modest. This is a single-user system.

## 8. Startup and shutdown

| Event | Behaviour |
|---|---|
| Start | Load configuration, validate it, run the first health check and model refresh, then accept requests. A worker being down at start is not fatal |
| Shutdown | Stop accepting new requests, cancel running upstream calls (V1), and close cleanly. In V2, mark running requests so they are recovered on next start |
| Config error | Refuse to start with a clear message |

## 9. Moving the brain

The router is self-contained and configuration-driven, so it can be moved to a
different machine by moving the service and its saved-request store and
updating addresses. The client entry point (the Tailscale name) is then
repointed. This matters if a larger machine is added later and becomes a
better home for the brain.

## 10. Failure behaviour

| Situation | Behaviour |
|---|---|
| Target busy | Wait for it, show "waiting" |
| Target down | Error (V1). Retry every 2 minutes for up to 3 hours (V2) |
| Pinned target unavailable | Fail with a clear message. No substitution |
| Failure partway through | Error (V1). Retry and restart from the beginning (V2) |
| Client disconnects | V1: cancel. V2: keep running |
| Mane restarts | V1: active requests lost. V2: waiting and retrying requests resume, and a request that was running restarts from the beginning |
| Mane down | Whole service unavailable. Accepted for now |

## 11. Out of scope

RAG, high availability, distributed inference, multi-user accounts,
notifications. See [PRD.md](PRD.md).
