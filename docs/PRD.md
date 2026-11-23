# Product Requirements Document

Status: draft v1. Date: 2026-11-23.

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
- Profiles such as "fast / balanced / deep" (deferred).

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
| UC6 | (Later) Start a long task, close the laptop, and read the result later. |
| UC7 | (Later) Add a new machine without changing routing rules. |

## 6. Functional requirements

Version targets: **V1** is the first release. **V2** adds durable sessions.
**V3** adds the queue and **V4** adds more machines. See [ROADMAP.md](ROADMAP.md).

### Routing

| ID | Requirement | Target |
|---|---|---|
| F1 | One entry point on Mane reachable from the MacBook Air over Tailscale. | V1 |
| F2 | **Auto mode** (default): the request goes to Firmino. | V1 |
| F3 | **Fast flag:** the request goes to Mane, using Mane's default model. | V1 |
| F4 | **Pin a model:** use exactly that model, on the machine that has it. | V1 |
| F5 | **Pin a machine:** use exactly that machine, with its default model unless a model is also pinned. | V1 |
| F6 | Pinned requests are never substituted or switched. If the pinned target is unavailable, the request fails with a clear message. | V1 |
| F7 | Auto and fast choose once, at the start of the request, and do not switch mid-task. | V1 |
| F8 | Mane keeps a registry of machines and the models installed on each, and reads it when routing. | V1 |
| F9 | Every response states the model and machine used. | V1 |

### Waiting and failure

| ID | Requirement | Target |
|---|---|---|
| F10 | If the intended machine is busy, the request waits for it. It is not rerouted. | V1 |
| F11 | While waiting, the request shows a visible "waiting" state. | V1 |
| F12 | If the intended machine is down, return a clear error. | V1 |
| F13 | After such an error, retry every 2 minutes for up to 3 hours, then stop and mark the request failed. | V2 |
| F14 | If a machine fails partway through a task, the same retry rule applies and the task restarts from the beginning. | V2 |
| F15 | Statuses are saved so they can be inspected later. | V2 |

### Durable sessions

| ID | Requirement | Target |
|---|---|---|
| F16 | A request keeps running on Mane when the client disconnects. | V2 |
| F17 | Output is saved on Mane as it is produced. | V2 |
| F18 | The owner can return later, find the request, and read its result and status. | V2 |
| F19 | Finished results are kept for 90 days, then removed. | V2 |

### Interface

| ID | Requirement | Target |
|---|---|---|
| F20 | A tiny web page served by Mane: send a request, set flags, see the answer and the model and machine used. | V1 |
| F21 | The page shows whether each machine is reachable. | V1 |
| F22 | The page lists past requests and their statuses. | V2 |
| F23 | A scriptable endpoint for terminal use. | V1 |

### Machines

| ID | Requirement | Target |
|---|---|---|
| F24 | New machines are added by registering them with a role and capabilities. Routing rules refer to roles, not names. | V4 |

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

## 8. Starting models

| Machine | Models |
|---|---|
| Mane | An 8B model (default for the fast flag) and a 3B model (reached by pinning). |
| Firmino | A 14B model (default for auto). Larger models added later if needed. |

Exact model choices are made at setup. See [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md).

## 9. Acceptance criteria

**V1**

- From the MacBook Air over Tailscale, a request with no flags is answered by
  Firmino's 14B model, and the response states so.
- With the fast flag, the request is answered by Mane's 8B model.
- A request pinned to the 3B model on Mane is answered by that model.
- A pinned request for a model that is not installed anywhere fails with a clear
  message and is not substituted.
- Sending a second request to Firmino while it is busy shows "waiting" and then
  completes after the first finishes.
- With Firmino switched off, an auto request returns a clear error.
- Mane's router and page stay within the RAM budget under normal use.
- Mane-to-Firmino traffic is confirmed to use the LAN.

**V2**

- Start a long task on Firmino, close the laptop, reopen later: the result and
  its status are visible.
- With Firmino down at submission, the request retries every 2 minutes and
  succeeds when Firmino returns, or fails after 3 hours.
- A task interrupted partway restarts from the beginning under the same rule.
- Results older than 90 days are removed.

## 10. Deferred items

Recorded here so they are not forgotten. See [ROADMAP.md](ROADMAP.md).

| Item | Notes |
|---|---|
| Full queue (priority lanes, batch submit from the terminal) | Needed soon. Builds on V2's saved-request store. Keep it simple, with no heavy infrastructure. |
| Profiles ("fast / balanced / deep") | Replaces flags with named presets. |
| Notifications when a task finishes | Not needed yet. |
| Automatic classification of tasks | Only if rules prove insufficient. |
| More machines (for example Mac Studio, DGX Spark) | Via the roles registry. |
| High availability | Out of scope while both machines are assumed up. |

## 11. Assumptions and risks

| Item | Notes |
|---|---|
| Both machines are always on | Stated by the owner. No failover is designed. |
| Firmino is a laptop | Needs lid, sleep, power and thermal settings checked. See [OPERATIONS.md](OPERATIONS.md). |
| 14B on CPU is slow, especially on long prompts | Accepted. Speed is not the priority. |
| Mane is a single point of failure | Accepted for now. |
| Speed figures in these docs are rough estimates | Measure after setup. |
| V1 retry behaviour | The retry loop needs requests to outlive the client, so it arrives with V2. In V1, an unavailable machine returns an error immediately. See DECISIONS D-014. |

## 12. Open questions

- Which exact 3B, 8B and 14B models to start with.
- What the router is built with (language and runtime), given the RAM budget.
- Where saved requests are stored in V2 (a small file or database on Mane's disk).
