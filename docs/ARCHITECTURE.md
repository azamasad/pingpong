# Architecture

Status: draft v1. Technology-neutral on purpose: the language and runtime for
the router are still open (see [PRD.md](PRD.md) section 12).

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
   |  - Machine/model registry   |
   |  - Saved requests (V2)      |
   |  - Ollama (<= 8B models)    |
   +--------------+--------------+
                  |
                  |  Home LAN (Ethernet, no internet)
                  v
   +-----------------------------+
   |  Firmino  (Ubuntu laptop)   |
   |  - Ollama (14B and larger)  |
   |  - No UI, no router         |
   +-----------------------------+

   Future machines attach the same way, via Mane.
```

## 2. Components

### Router (on Mane)

The only place routing decisions are made. It:

1. Accepts requests from the client (web page or terminal).
2. Reads the flags and any pinned model or machine.
3. Consults the registry to pick the target.
4. Forwards the request to Ollama on that machine.
5. Streams the answer back, tagged with the model and machine used.
6. Tracks the request status (waiting, running, done, failed).

It follows the rules in [ROUTING.md](ROUTING.md). It never inspects the
meaning of a prompt.

### Tiny web page (on Mane)

Served by the router itself. Static and minimal, so it costs almost no memory.
No separate UI server. Details in [UI-SPEC.md](UI-SPEC.md).

### Registry

A list of machines, kept as configuration plus what Mane learns by asking each
machine. For each machine:

| Field | Meaning |
|---|---|
| Name | For example `mane`, `firmino`. |
| Address | LAN address used for traffic. |
| Roles | For example `fast`, `heavy`. Rules refer to roles. |
| Installed models | Read from the machine's Ollama. |
| Reachable | Result of the latest health check. |

Routing rules say things like "auto goes to the `heavy` role", not "auto goes
to Firmino". Today, Firmino is the only `heavy` machine. When another is
added, the rules do not change. How to choose between several machines of the
same role is a later decision (see ROADMAP).

### Inference servers (Ollama)

One Ollama instance per machine. Mane runs models up to 8B. Firmino starts
with a 14B model. Firmino exposes its Ollama port to the LAN, restricted by
firewall to Mane's address only.

### Saved-request store (V2)

A small record per request, stored on Mane's disk: the prompt, flags, chosen
machine and model, status history, output so far, timestamps. Results are kept
90 days. It must stay small enough to meet the RAM budget, so it should be
plain files or an embedded database, not a database server.

Not built in V1, but nothing in V1 should prevent it. See
[JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md).

## 3. Request flow

### V1 (request tied to the connection)

1. Client sends a request to Mane with optional flags.
2. Router selects the target (see ROUTING).
3. If the target is busy, the request waits and the page shows "waiting".
4. If the target is down, return an error.
5. Router forwards to the target's Ollama and streams the answer back with the
   model and machine used.

If the client disconnects mid-request, the work may be lost. This is a known
V1 limitation.

### V2 (durable)

1. Client submits the request. Router saves it and returns an ID.
2. Router runs the request in the background, independent of the client.
3. Output and status updates are saved as they happen.
4. The client can attach, detach and come back later using the ID.
5. Retry rules apply (see JOBS-AND-RETRIES).

## 4. Network paths

| Path | Transport | Purpose |
|---|---|---|
| MacBook Air to Mane | Tailscale | The only client entry point |
| Mane to Firmino | Home LAN | Inference traffic |
| Admin to Firmino | Tailscale SSH | Maintenance only, not inference |

Details in [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md).

## 5. Memory

| Item | Budget |
|---|---|
| Router, page, registry, store (Mane) | Working budget 50 MB |
| Models on Mane | 8B or smaller, with room for the OS and context |
| Firmino | Not constrained by the brain, since it does not run it |

## 6. Moving the brain

The router is self-contained and configuration-driven, so it can be moved to a
different machine by moving the service and its saved-request store and
updating addresses. The client entry point (Tailscale name) is then repointed.
This matters if a larger machine is added later and becomes a better home for
the brain.

## 7. Failure behaviour

| Situation | Behaviour |
|---|---|
| Target busy | Wait for it, show "waiting". |
| Target down | Error (V1). Retry every 2 minutes for up to 3 hours (V2). |
| Pinned target unavailable | Fail with a clear message. No substitution. |
| Failure partway through | Retry and restart from the beginning (V2). |
| Mane down | Whole service unavailable. Accepted for now. |

## 8. Out of scope

RAG, high availability, distributed inference, multi-user accounts,
notifications. See [PRD.md](PRD.md).
