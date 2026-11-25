# Roadmap

Status: v3. V1 to V7 are built, merged and **running on the real machines**. V7.1 to V7.9 are specified, approved and ready to build.
Dates are intentionally absent. Phases are ordered by dependency, not by
calendar. Current focus: measure, tune and use it (see "Now").

## Now

| Item | State |
|---|---|
| Router V1 to V7, Stress tab, bench scripts, per-request log | Built |
| Live on Mane and Firmino, `doctor --chat` passing | Done |
| Firmino model: mixture-of-experts 30B (D-073) | Done, measured |
| Page reachable only on the tailnet | Done, checked by the owner (D-080) |
| Tailscale ACL for Firmino's port 11434 | Owner. Rules and steps are ready (D-081) |
| Soak with the Stress tab, then read the log for improvements | Owner, runbook in [OPERATIONS.md](OPERATIONS.md) section 11 |
| Reboot and power-cut tests | Owner, runbook in section 12 |
| `gpt-oss:20b` blind comparison against the 30B | Owner, runbook in section 13. Assessment in [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 2a |
| `qwen2.5:14b` on Firmino | Decided: stays (D-083) |

## Overview

**Versioning scheme:** a feature is a minor step after the last built version
(V7.1, V7.2, ...). A new major version (V8) is used only for a large change.
Built so far: V1 to V7. Everything numbered V7.x below is planned and not built, and is built in the order V7.1 to V7.9.

| Phase | Name | Outcome |
|---|---|---|
| 0 | Foundation | Both machines ready and reachable |
| 1 | V1: front door and routing | Send a request to Mane and have it routed |
| 2 | V2: durable sessions | Tasks survive the client closing. Retries. History |
| 3 | V3: queue | A simple batch lane and terminal submission |
| 4 | V4: expansion | More machines, by role |
| 5 | V5: profiles | Named presets for fast, balanced and deep |
| 6 | V6: operability | `doctor`, graceful shutdown, backup, status view |
| 7 | V7: model aliases | Short names for models, defined once |
| 7.1 | V7.1: split a batch across machines (planned) | Both machines at once, each with its own pinned model, fixed assignment |
| 7.2 | V7.2: answer checking (planned) | Mane drafts, Firmino reviews or revises, user can override |
| 7.3 | V7.3: prompt templates (**built** on V7 branch) | Saved prompts with blanks |
| 7.4 | V7.4: results export (planned) | Download batches as CSV or Markdown |
| 7.5 | V7.5: conversation threads (planned) | Saved conversations with history |
| 7.6 | V7.6: file input (planned) | Text and Markdown, chunked, run as a batch. Not RAG. PDF later |
| 7.7 | V7.7: scheduled jobs (planned) | Nightly or weekly runs, set up from the page |
| 7.8 | V7.8: OpenAI-compatible endpoint (planned) | Opt-in `/v1` |
| 7.9 | V7.9: notifications (planned) | One optional webhook, no content |
| 7+ | Test tooling (not a numbered version) | Stress tab, bench scripts, per-request log line (D-075, D-076) |

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
- Reachable addresses for both machines. They are on different subnets, so
  Mane reaches Firmino over Tailscale (D-072).
- Ollama running on both. Firmino's restricted to Mane (ACL recommended).
- Tailscale access from the MacBook Air to Mane.
- Install starting models (`llama3.1:8b` on Mane, the 30B mixture-of-experts
  model on Firmino) and measure real speeds. Done: see
  [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 8.

Done when: from Mane, each machine answers a simple request, and the MacBook
Air can reach Mane over Tailscale. **Done.**

## Phase 1: V1, front door and routing

- Router on Mane with the registry.
- Auto, `fast`, pinned model and pinned machine, per [ROUTING.md](ROUTING.md).
- Wait for the intended machine when busy. Error when it is down.
- Tiny web page and a scriptable endpoint.
- Every response shows the model, machine, and how it was chosen.
- Verify the RAM budget (about 24 MB measured) and that traffic is private (Tailscale or LAN).

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

Built and merged (D-066).

- Interactive and batch lanes, interactive admitted first, FIFO within a lane.
- Per-machine concurrency stays `max_concurrent`.
- `POST /api/batches`, `GET /api/batches/{id}`, and `pingpong batch`.
- No brokers or job frameworks.

## Phase 4: V4, expansion

Built and merged (D-067).

- New machines are config entries with a role. Rules still name roles.
- Several machines with one role: least loaded machine that is up, then file order.
- The brain stays on Mane. Moving it is a config change, not a code change.

## Phase 5: V5, profiles

Built and merged (D-068).

- Named presets in config. Defaults: `fast`, `balanced`, `deep`.
- A request may set `profile`. Explicit `fast`, `model`, or `machine` overrides the preset.
- `GET /api/profiles` lists them. The page has a profile selector.

## Phase 6: V6, operability

Spec: [V6-OPERABILITY.md](V6-OPERABILITY.md) (D-069). Built and merged. Run on
the real machines.

- `pingpong doctor`, with an opt-in `--chat` check against the real Ollama.
- Graceful shutdown with a retryable `ROUTER_RESTARTING` error.
- `pingpong backup` using a consistent SQLite copy.
- `GET /api/status` and a Status tab. No content is ever shown.

## Phase 7: V7, model aliases

Spec: [V7-ALIASES.md](V7-ALIASES.md) (D-070). Built and merged.

- `aliases:` in config map a short name to a real model, and optionally a machine.
- An alias is a pin. Routing rules and errors are unchanged.
- `GET /api/aliases`, an `alias` field on the route event, and a picker in the page.

## Test tooling (added during go-live)

Requested by the owner to test both machines without typing prompts, and to
gather data for tuning. Kept small (D-075, D-076, D-077).

- **Stress tab** and `POST/GET/DELETE /api/stress`: a server-side soak of up to
  2 hours on Mane, Firmino or both. It runs on the batch lane with short
  prompts, so it does not depend on the browser staying open.
- **Per-request log line** with speeds and outcome, never content.
- **Speed shown on the page** when a request finishes.
- `deploy/bench/quick.sh` and `deploy/bench/quality.py`.

## Backlog from the real-machine results

| Item | Why |
|---|---|
| Read a day of the log and tune from it | The log now carries speed, queue wait, loads and retries |
| Lower `first_token_timeout` | The measured prompt reading speed on Firmino is about 30 tokens/s, so 8K tokens take 4 to 5 minutes |
| Compare `gpt-oss:20b` against the current Firmino model | Second mixture-of-experts candidate. Reported slower on CPU. Setup ready (OPERATIONS section 13) |
| Apply the Tailscale ACL for Firmino's Ollama port | The only gap in the Firmino path. Rules ready (D-081) |
| Login layer or custom domain | The page has no login and is open to the tailnet. Only if the owner wants it |
| Restart-on-boot checks | Runbook ready (OPERATIONS section 12) |

## Planned next versions (owner-approved, not built)

PRDs for V7.1 to V7.7 are in [NEXT-FEATURES-PRDS.md](NEXT-FEATURES-PRDS.md), and
the build contract for V7.1 to V7.9 (payloads, storage, config, errors, UI,
acceptance) is in [NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md).
Decisions D-086 to D-117 (renumbering in D-095). The build checklist is
[IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md) section 5c.

**Owner priority and build order: V7.1 and V7.2 first, then V7.3, V7.4, V7.5, V7.6,
V7.7, then V7.8 and V7.9.** The version numbers follow the
build order. Dependencies: V7.3 (templates) feeds V7.6 and V7.7, V7.2 (checking)
gives V7.6 its optional final step, V7.4 (export) and V7.1 (split) are
independent of each other and useful to all.

| Version | Feature | Owner priority | Main risk |
|---|---|---|---|
| V7.1 | Split a batch across both machines | High, owner likes it | Fixed assignment only. A down machine delays its own items |
| V7.2 | Answer checking by a second model | High, owner needs it | Doubles run time, context size of the review prompt |
| V7.3 | Prompt templates | Medium | Keep it plain text substitution only |
| V7.4 | Results export (CSV, Markdown) | Medium | Spreadsheet formula injection, streaming |
| V7.5 | Conversation threads | Medium | Context growth, retention rule |
| V7.6 | File input (text and Markdown) | Lower | Non-goal wording. PDF is later, needs a measured memory check |
| V7.7 | Scheduled jobs, set up from the page | Lower | Missed runs, daylight-saving time, anyone on the tailnet can create schedules |
| V7.8 | OpenAI-compatible endpoint | Built (M28, V7 branch) | Connection-tied, no tools |
| V7.9 | Notifications | Owner approved (D-116, D-117) | Outbound call from Mane. Public webhook host accepted, no content sent |

The owner approved four constitution wording changes (D-101). They are applied
in CONSTITUTION.md and listed in the PRD file under "Constitution changes
applied".

## Tech debt and future work

| ID | Item | Why it matters | Possible fix |
|---|---|---|---|
| TD-1 | The Ollama desktop app is still installed on Mane (D-082) | If it starts at login it holds port 11434, so the launch agent's settings (context 8192, keep-alive, no-cloud) silently do not apply. This already happened once | Delete the app, or remove it from Login Items. Cheap safety net, only if the owner wants it: `pingpong doctor` could warn when the listener on the Ollama port is not the launch agent. Until then, check after each reboot with `lsof -nP -iTCP:11434 -sTCP:LISTEN` |
| TD-2 | The router's `think` option is a true/false switch only | `gpt-oss` models take levels (low, medium, high), so their reasoning effort cannot be set through the router | Accept a string as well. Only if `gpt-oss` wins the comparison |
| TD-3 | No automatic tests of the bench scripts | They are exercised by hand against stand-in servers only | Add a small script test if they become important |

## Deferred and backlog

| Item | Notes |
|---|---|
| Profiles (fast / balanced / deep) | Shipped as V5 (D-068) |
| Notifications when tasks finish | Planned as V7.9 |
| Automatic task classification | Only if rules prove insufficient |
| High availability | Out of scope while both machines are assumed up |
| Task-specific routing refinements | After real usage shows the need |
| Model aliases | Shipped as V7 (D-070) |
| Conversation threads on Mane | Planned as V7.5, PRD written ([NEXT-FEATURES-PRDS.md](NEXT-FEATURES-PRDS.md)) |
| OpenAI-compatible endpoint | Shipped as V7.8 (M28, opt-in `/v1`) |
| Batch split, answer checking, templates, export, file input, schedules | Planned as V7.1 to V7.7, PRDs written |

## Explicitly excluded

- RAG (retrieval, embeddings, search). Chunking a file to process as a batch is V7.6 and is not RAG. Done as a separate learning project. If it needs a local model later, it
  can call Mane as an ordinary client.

## Open decisions

None. The owner answered Q1 to Q5 (D-116). Owner choices under "Now" are unchanged.
