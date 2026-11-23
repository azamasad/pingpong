# Roadmap

Status: draft v1. Dates are intentionally absent. Phases are ordered by
dependency, not by calendar.

## Overview

| Phase | Name | Outcome |
|---|---|---|
| 0 | Foundation | Both machines ready and reachable |
| 1 | V1: front door and routing | Send a request to Mane and have it routed |
| 2 | V2: durable sessions | Tasks survive the client closing. Retries. History |
| 3 | V3: queue | A simple batch lane and terminal submission |
| 4 | V4: expansion | More machines, by role |

## Who does what

| Work | Who |
|---|---|
| Phase 0 (hardware, OS, network, Tailscale, installing Ollama and models) | The owner, on the real machines. A coding agent cannot reach them |
| Writing the router, page and tests | A coding agent or the owner, following [IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md) |
| Acceptance runs on real hardware | The owner, using [TESTING.md](TESTING.md) section 4 |

Phase 0 and V1 development can proceed in parallel. Development uses a fake
Ollama until the real machines are ready.

## Phase 0: Foundation

The step-by-step checklist is [SETUP-GUIDE.md](SETUP-GUIDE.md).

- Free up disk on Mane and Firmino.
- Mane: no sleep, auto-start after power failure, services start on boot.
- Firmino: lid and sleep settings, apply updates and reboot, normal user instead
  of root, BIOS power setting, thermals check.
- Fixed LAN addresses for both machines.
- Ollama running on both. Firmino's firewalled to Mane only.
- Tailscale access from the MacBook Air to Mane, with ACLs.
- Install starting models (`llama3.2:3b` and `llama3.1:8b` on Mane, `qwen2.5:14b` on
  Firmino) and measure real speeds. Update [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md).

Done when: from Mane, each machine answers a simple request, and the MacBook
Air can reach Mane over Tailscale.

## Phase 1: V1, front door and routing

- Router on Mane with the registry.
- Auto, `fast`, pinned model and pinned machine, per [ROUTING.md](ROUTING.md).
- Wait for the intended machine when busy. Error when it is down.
- Tiny web page and a scriptable endpoint.
- Every response shows the model, machine, and how it was chosen.
- Verify the RAM budget and LAN-only traffic.

Build steps are in [IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md) (M0 to M10).

Done when: the V1 acceptance criteria in [PRD.md](PRD.md) pass.

## Phase 2: V2, durable sessions

- Requests keep running with no client attached.
- Saved requests with status history, stored on Mane.
- Retry every 2 minutes for up to 3 hours, including mid-task failures
  (restart from the beginning).
- Request history in the page, and reattaching to a running request.
- 90-day retention.
- Resume waiting and retrying requests after Mane restarts.
- Explicit cancel and delete of requests.

Done when: the V2 acceptance criteria in [PRD.md](PRD.md) pass.

## Phase 3: V3, queue

Implemented on `cursor/v3-queue-aa2f` (D-066).

- Interactive and batch lanes, interactive admitted first, FIFO within a lane.
- Per-machine concurrency stays `max_concurrent`.
- `POST /api/batches`, `GET /api/batches/{id}`, and `pingpong batch`.
- No brokers or job frameworks.

## Phase 4: V4, expansion

- Register new machines by role (for example Mac Studio, DGX Spark).
- A rule for choosing between several machines of one role.
- Reconsider where the brain should live.

## Deferred and backlog

| Item | Notes |
|---|---|
| Profiles (fast / balanced / deep) | Named presets. Possibly during or after V3 |
| Notifications when tasks finish | Not needed yet |
| Automatic task classification | Only if rules prove insufficient |
| High availability | Out of scope while both machines are assumed up |
| Task-specific routing refinements | After real usage shows the need |
| Model aliases | Short names for models |
| Conversation threads on Mane | Group saved requests into conversations |
| OpenAI-compatible endpoint | So other tools can use Pingpong |

## Explicitly excluded

- RAG. Done as a separate learning project. If it needs a local model later, it
  can call Mane as an ordinary client.

## Open decisions

- Which models to start with (owner, at setup).
