# Product Requirements Document

Status: draft v2. Date: 2026-11-23.

Precedence if documents disagree: [constitution](../CONSTITUTION.md), then this
PRD, then the other documents.

## 1. Summary

Two always-on machines on a home LAN, **Mane** (Mac mini M4) and **Firmino**
(Ubuntu laptop), each run local LLMs through Ollama. Pingpong makes them one
service: the owner sends a request to Mane from a MacBook Air, and Mane decides
which machine and model should handle it.

## 2. Goals

1. Reach local models remotely through a single address (Mane over Tailscale).
2. Let Mane choose where a request runs, using simple, predictable rules.
3. Let the owner override that choice by pinning a model or a machine.
4. Use Firmino for long-running and heavier work, and Mane for quick work.
5. Stay small: the brain adds only a few tens of MB of RAM on Mane.
6. Make adding future machines straightforward.
7. (Later) Let tasks keep running when the client disconnects, and let the
   owner come back to the results.

## 3. Non-goals

See also the constitution.

- RAG, document search, vector databases.
- High availability and failover.
- Splitting one model across machines.
- Multiple users, logins, billing.
- Notifications (deferred).
- Automatic prompt classification (deferred).
- Profiles such as "fast / balanced / deep" (shipped in V5, D-068).
- Model aliases (planned for V7, D-070).
- An OpenAI-compatible API (deferred).

## 4. User and context

One user (the owner), mostly on a MacBook Air. Both servers are on Ethernet in
the same place and are assumed always on. Speed is not the top priority.
Slower is acceptable for long-running work.

## 5. Use cases

| ID | Use case |
|---|---|
| UC1 | Ask a quick question and get a fast answer from a small model on Mane. |
| UC2 | Send a long or heavy task to the 14B model on Firmino and wait. |
| UC3 | Pin a specific model or machine for a particular task. |
| UC4 | See which model and machine produced an answer. |
| UC5 | See that a request is waiting, running, failed or done. |
| UC6 | Stop a request that is no longer wanted. In V1 this means closing or stopping the request. In V2 it is an explicit cancel. |
| UC7 | (V2) Start a long task, close the laptop, and read the result later. |
| UC8 | (V4) Add a new machine without changing routing rules. |

## 6. Functional requirements

Version targets: **V1** is the first release. **V2** adds durable sessions.
**V3** adds the queue and **V4** adds more machines. See [ROADMAP.md](ROADMAP.md).

Interfaces are specified in [API.md](API.md), settings in
[CONFIGURATION.md](CONFIGURATION.md).

### Routing

| ID | Requirement | Target |
|---|---|---|
| F1 | One entry point on Mane reachable from the MacBook Air over Tailscale. | V1 |
| F2 | **Auto mode** (default): the request goes to the machine with the `heavy` role (Firmino), using its default model. | V1 |
| F3 | **Fast flag:** the request goes to the machine with the `fast` role (Mane), using its default model. | V1 |
| F4 | **Pin a model:** use exactly that model. If it is installed on one machine, use that machine. If it is on several and no machine is named, fail as ambiguous. | V1 |
| F5 | **Pin a machine:** use exactly that machine, with its default model unless a model is also pinned. | V1 |
| F6 | Pinned requests are never substituted or switched. If the pinned target is unavailable or lacks the model, the request fails with a clear message. | V1 |
| F7 | Pins override the `fast` flag. | V1 |
| F8 | Auto and fast choose once, at the start of a request, and do not switch mid-task. | V1 |
| F9 | Mane keeps a registry of machines and the models installed on each, and reads it when routing. | V1 |
| F10 | Every response states the model and machine used, and how the target was chosen. | V1 |

### Waiting, failure and cancel

| ID | Requirement | Target |
|---|---|---|
| F11 | If the intended machine is busy, the request waits for it, first come first served. It is not rerouted. | V1 |
| F12 | While waiting, the request shows a visible "waiting" state. | V1 |
| F13 | If the intended machine is down, return a clear error. | V1 |
| F14 | If the client disconnects, a waiting request is dropped and a running request is cancelled upstream, so the worker stops. | V1 |
| F15 | After a down or failed machine, retry every 2 minutes for up to 3 hours from the first failure, then stop and mark the request failed. | V2 |
| F16 | If a machine fails partway through a task, the same retry rule applies and the task restarts from the beginning. | V2 |
| F17 | Statuses are saved so they can be inspected later. | V2 |
| F18 | The owner can explicitly cancel a waiting, running or retrying request. | V2 |
| F19 | The owner can delete a saved request. | V2 |

### Durable sessions

| ID | Requirement | Target |
|---|---|---|
| F20 | A request keeps running on Mane when the client disconnects. | V2 |
| F21 | Output is saved on Mane as it is produced. | V2 |
| F22 | The owner can return later, find the request, and read its result and status, or reattach to a still-running one. | V2 |
| F23 | Finished results are kept for 90 days, then removed. | V2 |

### Conversations

| ID | Requirement | Target |
|---|---|---|
| F24 | In V1 the router keeps no conversation state. The client sends the full message history each time, and the page holds it. | V1 |
| F25 | The page keeps flag and pin settings between messages, so a conversation does not change target unless the owner changes it. | V1 |

### Interface

| ID | Requirement | Target |
|---|---|---|
| F26 | A tiny web page served by Mane: send a request, set flags, see the answer and the model and machine used. | V1 |
| F27 | The page shows whether each machine is reachable and busy. | V1 |
| F28 | The page lists past requests and their statuses. | V2 |
| F29 | A scriptable endpoint for terminal use, with the same flags. | V1 |
| F31 | The request can pass a small allow-list of generation options and a `think` setting, and can ask for a single non-streamed reply (`stream:false`). | V1 |
| F32 | Read-only endpoints list machines (with reachable and busy), installed models, and overall health. | V1 |
| F33 | The router rejects requests with an unexpected `Host` or `Origin`, and bodies over a size limit. | V1 |

### Machines

| ID | Requirement | Target |
|---|---|---|
| F30 | New machines are added by registering them with a role and capabilities. Routing rules refer to roles, not names. | V4 |

## 7. Non-functional requirements

| ID | Requirement |
|---|---|
| N1 | The router and web page use at most about 50 MB of RAM on Mane (working budget). |
| N2 | Mane runs models of 8B parameters or smaller. |
| N3 | Mane-to-Firmino traffic stays on the LAN. |
| N4 | The service is reachable only via Tailscale. No public exposure. |
| N5 | No logins. Tailscale is the access control. |
| N6 | No UI or extra services on Firmino beyond model serving and basic health. |
| N7 | The router is movable to another machine with configuration changes. |
| N8 | Slower responses on Firmino are acceptable. |
| N9 | Prompts and outputs are never written to logs. |
| N10 | Timeouts are generous enough for CPU inference on long prompts (see CONFIGURATION). |
| N11 | The router starts on boot and restarts after failure. |
| N12 | The context window is set explicitly on both machines (8192 tokens to start), never left at Ollama's silent 4096 default. |
| N13 | Reasoning (thinking) output, if a model produces it, is kept separate from the answer. |
| N14 | Open streams send periodic heartbeats, so a long silent wait is not mistaken for a dead connection, and dead clients are detected. |
| N15 | If a prompt fills the context window, the response carries a visible warning instead of failing silently. |
| N16 | While a request is running or waiting, the owner can see how long it has run. Total running time is capped (default 2 hours) so a stuck request cannot block a machine indefinitely. |
| N17 | The 50 MB budget (N1) means the router's resident memory: well under it when idle, and at most 50 MB during a defined load of 4 concurrent streams plus one 8,192-token prompt. |

## 8. Starting models

| Machine | Models |
|---|---|
| Mane | `llama3.1:8b` (default for the fast flag) and `llama3.2:3b` (reached by pinning). |
| Firmino | `qwen2.5:14b` (default for auto). Larger models added later if needed. |

These are non-thinking models, chosen for simplicity and published benchmarks. They are configuration and can be changed. See [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) and D-038.

## 9. Acceptance criteria

The manual check list is in [TESTING.md](TESTING.md) section 4.

**V1**

- From the MacBook Air over Tailscale, a request with no flags is answered by
  Firmino's 14B model, and the response states so.
- With the fast flag, the request is answered by Mane's 8B model.
- A request pinned to the 3B model on Mane is answered by that model.
- A pinned request for a model that is not installed anywhere fails with a clear
  message and is not substituted.
- A pinned model installed on several machines, with no machine named, fails as
  ambiguous.
- Sending a second request to Firmino while it is busy shows "waiting" and then
  completes after the first finishes.
- With Firmino switched off, an auto request returns a clear error.
- Closing the browser during a request stops the work on the worker.
- Mane's router and page stay within the RAM budget under normal use.
- Mane-to-Firmino traffic is confirmed to use the LAN.
- Firmino's inference port is not reachable from the MacBook Air directly.
- Logs contain no prompts or outputs.
- A long quiet wait on Firmino (many minutes) does not drop the connection, and the page shows elapsed time.
- A prompt that fills the context window shows a warning.

**V2**

- Start a long task on Firmino, close the laptop, reopen later: the result and
  its status are visible.
- With Firmino down at submission, the request retries every 2 minutes and
  succeeds when Firmino returns, or fails after 3 hours.
- A task interrupted partway restarts from the beginning under the same rule.
- Cancel stops a waiting, running or retrying request.
- Waiting and retrying requests survive a restart of the router.
- Results older than 90 days are removed.

## 10. Deferred items

Recorded here so they are not forgotten. See [ROADMAP.md](ROADMAP.md).

| Item | Notes |
|---|---|
| Full queue (priority lanes, batch submit from the terminal) | Shipped as V3 (D-066). Builds on V2's saved-request store. Keep it simple, with no heavy infrastructure. |
| Profiles ("fast / balanced / deep") | Shipped as V5 (D-068). |
| Notifications when a task finishes | Not needed yet. |
| Automatic classification of tasks | Only if rules prove insufficient. |
| More machines (for example Mac Studio, DGX Spark) | Via the roles registry (V4). |
| Several machines with the same role | Shipped as V4 (D-067). |
| Model aliases (short names such as "small") | Planned as V7 (D-070). See [V7-ALIASES.md](V7-ALIASES.md). |
| Conversation threads stored on Mane | V2 stores individual requests. Threads are optional later. |
| OpenAI-compatible endpoint | Would let other tools use Pingpong. Later, if wanted. |
| High availability | Out of scope while both machines are assumed up. |

## 11. Success measures

A simple view of "is this working":

- Every request reports its model and machine, and they match the rules.
- Mane's quick tasks stay quick, because heavy work is not loaded onto it.
- Long tasks on Firmino finish without babysitting (from V2).
- Mane's router stays within its memory budget.
- The owner reaches both machines through one address and does not need to
  think about which machine to use.

## 12. Assumptions and risks

| Item | Notes |
|---|---|
| Both machines are always on | Stated by the owner. No failover is designed. |
| Firmino is a laptop | Needs lid, sleep, power and thermal settings checked. See [OPERATIONS.md](OPERATIONS.md). |
| 14B on CPU is slow, especially on long prompts | Accepted. Speed is not the priority. |
| Mane is a single point of failure | Accepted for now. |
| Speed figures in these docs are rough estimates | Measure after setup. |
| V1 retry behaviour | The retry loop needs requests to outlive the client, so it arrives with V2. In V1, an unavailable machine returns an error immediately. See D-014. |
| No authentication on the router | Mitigated by Tailscale and Host and Origin checks. |
| Public repository | No real addresses or secrets may be committed. |

## 13. Related documents

| Document | Role |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | How it is put together |
| [ROUTING.md](ROUTING.md) | Routing rules |
| [API.md](API.md) | Interfaces |
| [CONFIGURATION.md](CONFIGURATION.md) | Settings |
| [JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md) | Durable sessions and retries |
| [IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md) | Build order |
| [TESTING.md](TESTING.md) | How to verify |
| [GLOSSARY.md](GLOSSARY.md) | Terms |

## 14. Open questions

- Whether `POST /api/chat` stays a separate path in V2 or becomes a wrapper over
  jobs (see API.md section 4).
