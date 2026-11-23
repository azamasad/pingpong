# Decision Log

A running record of decisions and the reasons for them. Add new entries at the
bottom. Do not rewrite old ones: if a decision changes, add a new entry that
supersedes it.

Format: ID, date, decision, reason.

| ID | Date | Decision | Reason |
|---|---|---|---|
| D-001 | 2026-11-23 | Project name is Pingpong, matching the repository. | Already the repo name. |
| D-002 | 2026-11-23 | Mane is the brain. Routing happens on Mane. | Matches the owner's design. Tailscale entry is on Mane. Its GPU inference leaves the CPU free for the router. With RAG excluded and models capped at 8B, there is no strong reason to host the brain elsewhere. |
| D-003 | 2026-11-23 | Routing is rule-based on clear signals. The router does not read prompts to guess difficulty. | Predictable and easy to debug. Avoids a fragile classifier. |
| D-004 | 2026-11-23 | Auto goes to Firmino (14B). The `fast` flag goes to Mane (8B). | Speed is not the priority and most work is expected to be long or heavy. |
| D-005 | 2026-11-23 | Pinned means pinned. No substitution or mid-task switching. | Predictability and trust. |
| D-006 | 2026-11-23 | Mane runs models up to 8B only. A 3B is reached by pinning. | 16 GB is shared with the OS and services. 14B is too tight. |
| D-007 | 2026-11-23 | Firmino starts with a 14B model. Larger models only if needed. | 14B is slow but workable on its CPU. |
| D-008 | 2026-11-23 | RAG is excluded from this project. | The owner is learning it separately. It can call Mane later as an ordinary client. |
| D-009 | 2026-11-23 | Client to Mane uses Tailscale. Mane to workers uses the home LAN directly. | Local traffic should not use the internet or depend on Tailscale. |
| D-010 | 2026-11-23 | No UI on workers. A tiny built-in page lives on Mane, within the RAM budget. | Save RAM. One person, one page. |
| D-011 | 2026-11-23 | No logins. Tailscale is the access control. | Single user. Keep it simple. |
| D-012 | 2026-11-23 | A request waits for its intended machine when busy. No rerouting. | The owner's preference. Work meant for Firmino stays on Firmino. |
| D-013 | 2026-11-23 | If a target is down or fails partway, retry every 2 minutes for up to 3 hours, restart from the beginning, then stop. Statuses are saved. | The owner's decision. |
| D-014 | 2026-11-23 | The retry loop arrives in V2. In V1, a down target returns an error immediately. | Retrying needs requests to outlive the client, which is V2's saved-request store. Confirmed by the owner. |
| D-015 | 2026-11-23 | Durable sessions are required but come after V1. | The owner wants to close the laptop and return later. V1 stays smaller. |
| D-016 | 2026-11-23 | Finished results are kept for 90 days. | The owner's choice. Text is small. |
| D-017 | 2026-11-23 | A simple queue is deferred but expected soon (V3). It builds on V2's store. | The owner expects to need it. No heavy infrastructure. |
| D-018 | 2026-11-23 | Profiles are deferred. V1 has auto, `fast`, and pinned. | Do not over-engineer. |
| D-019 | 2026-11-23 | Notifications are deferred. | Not needed yet. |
| D-020 | 2026-11-23 | High availability is out of scope. | Both machines are assumed up. |
| D-021 | 2026-11-23 | Machines are described by roles in a registry. | Easier to add machines such as a Mac Studio or DGX Spark. |
| D-022 | 2026-11-23 | The router is movable to another machine. | A larger machine may become a better home later. |
| D-023 | 2026-11-23 | Docs live in this repository, with documents under `docs/`. | The owner's instruction. |
| D-024 | 2026-11-23 | Confirmed by the owner: build the router in Go, as one small program with the page embedded. | Small memory footprint against the 50 MB budget, easy service deployment, cross-compiles for macOS and Linux. Python and Node are likelier to strain the budget. |
| D-025 | 2026-11-23 | Confirmed by the owner: V2 stores saved requests in a single SQLite file on Mane. | Embedded, small, reliable, no server process. |
| D-026 | 2026-11-23 | Machines have a concurrency limit, default 1. Requests wait first come, first served. | Fits CPU inference on Firmino and keeps behaviour predictable. Mane's own direct use of Ollama is not counted. |
| D-027 | 2026-11-23 | In V1, closing the client connection cancels the request, including upstream, so the worker stops. | Avoids wasted CPU. Surviving disconnects is V2. |
| D-028 | 2026-11-23 | The router keeps no conversation state in V1. The client sends full history. | Keeps V1 small. |
| D-029 | 2026-11-23 | Pins override `fast`. A pinned model on several machines with no machine named is an error. | Pinned means pinned, and no guessing. |
| D-030 | 2026-11-23 | The retry window runs from the first failure and is not reset by later failures. | The owner's rule: retry every 2 minutes until 3 hours, then stop. |
| D-031 | 2026-11-23 | A `cancelled` state and explicit cancel and delete are added for V2. | A 3-hour retry loop needs a way to stop it. |
| D-032 | 2026-11-23 | Prompts and outputs are never written to logs. | Privacy. V2 stores them only in the saved-request store. |
| D-033 | 2026-11-23 | The router checks Host and Origin headers. | It has no login, and this blocks other web pages from using it through the browser. |
| D-034 | 2026-11-23 | Timeouts are generous by default: 20 minutes to first token, 5 minutes between tokens. They are starting values to tune. | CPU inference on long prompts can take minutes. Superseded by D-055. |
| D-035 | 2026-11-23 | An OpenAI-compatible endpoint, model aliases, and conversation threads are deferred. | Not needed for V1. Do not over-engineer. |
| D-036 | 2026-11-23 | Real addresses and config are never committed. Only placeholders. | The repository is public. |
| D-037 | 2026-11-23 | Take a light approach: build V1 minimally, with the fewest dependencies and the least code that meets the PRD. Prefer the standard library. | The owner asked for a light approach, consistent with constitution P1 and P2. |
| D-038 | 2026-11-23 | Starting models are non-thinking: `llama3.1:8b` (Mane, fast), `llama3.2:3b` (Mane, pinned), `qwen2.5:14b` (Firmino, auto). Thinking models can be pinned later. | Simple, widely used, and each has published benchmarks. Thinking models produce long reasoning output that is very slow on Firmino's CPU. The router still forwards `thinking` text separately so they can be used later. |
| D-039 | 2026-11-23 | FileVault is off and automatic login is on for Mane. | FileVault asks for a password at startup, which blocks unattended recovery after a power failure. The trade-off is that physical access to Mane exposes its disk. The owner can reverse this and accept manual unlocking. |
| D-040 | 2026-11-23 | The router listens on localhost only, and Tailscale Serve publishes it to the tailnet. Firmino's Ollama accepts connections from Mane only, by firewall. | Nothing is open to the LAN, no extra proxy to run, and HTTPS comes for free. Firewall part superseded by D-054. |
| D-041 | 2026-11-23 | Ollama and the router run as user launch agents on Mane, with settings in the service definition. | `launchctl setenv` does not survive a reboot. Running the Ollama desktop app and a separate service would conflict. |
| D-042 | 2026-11-23 | The context window is set explicitly to 8192 tokens on both machines. | Ollama's default of 4096 silently truncates long prompts. 8192 stays within Mane's memory and Firmino's first-token timeout. |
| D-043 | 2026-11-23 | Ollama's cloud features are switched off on both machines (`OLLAMA_NO_CLOUD`). | Keeps everything local and private. |
| D-044 | 2026-11-23 | Speed expectations are based on third-party community benchmarks and must be replaced by the owner's measurements. | No first-party data exists yet. See HARDWARE-AND-MODELS section 8. |
| D-045 | 2026-11-23 | Open streams send a `ping` heartbeat every 15 seconds when otherwise quiet. | Long waits on Firmino can be silent for many minutes. Proxies and browsers close quiet connections, and in V1 a closed connection cancels the request. Failed heartbeat writes also reveal dead clients. |
| D-046 | 2026-11-23 | The router always sends `num_ctx` (default 8192) and ignores a client-supplied value. `done` carries `context_warning` when the prompt nearly fills the window. | Prevents silent truncation and model reloads from changing window sizes. |
| D-047 | 2026-11-23 | A machine that is actively streaming is treated as up. Health checks have a generous timeout. | A CPU-bound Firmino may answer health checks slowly, and marking it down would wrongly break a running request. Refined by D-058. |
| D-048 | 2026-11-23 | The registry keeps the last known model list for a machine that goes down. | Lets pinned requests be validated, and is needed for V2 retries. |
| D-049 | 2026-11-23 | A machine's default model that is not installed returns `MODEL_NOT_INSTALLED` before calling Ollama. | Clear errors, not a late failure. |
| D-050 | 2026-11-23 | Terminal output uses `jq` on the NDJSON stream, with no extra router feature. | Keeps the router small. |
| D-051 | 2026-11-23 | Tailscale Serve HTTPS is used. The tailnet host name appears in public certificate logs. Plain HTTP on the tailnet is the accepted fallback. | The traffic is already encrypted by Tailscale. HTTPS gives a proper address, and the exposure is only a name. |
| D-052 | 2026-11-23 | The fake Ollama used in tests is checked against a real Ollama with a tiny model wherever possible. | Stops the fake from drifting from reality. This is the cheapest protection against unverified API assumptions. |
| D-053 | 2026-11-23 | If an implementer cannot ask the owner, they choose the simplest option that obeys the constitution, record it, and continue. | Allows unattended work without widening scope. |
| D-054 | 2026-11-23 | Firmino's Ollama binds to its LAN address only, and the Tailscale ACL blocks tailnet access to port 11434. The `ufw` rule for `tailscale0` is removed. Supersedes the firewall-only part of D-040. | On Linux, Tailscale accepts all traffic on its interface before `ufw`, so `ufw` cannot keep the MacBook Air off Firmino's Ollama. |
| D-055 | 2026-11-23 | The first-token timeout default is 30 minutes (was 20), with a 2-hour cap on total run time. Supersedes D-034. "First token" means the first chunk of any kind from Ollama, including `thinking`. | 8,192 tokens at 5 tokens per second takes about 27 minutes. The cap stops one stuck request from blocking a machine. |
| D-056 | 2026-11-23 | Routing checks happen in a fixed order: choose target, then machine down, then model installed. If a pinned model is unknown because a machine was never reached, the error is `MODELS_UNKNOWN` (retryable). | A machine that is off must report as down, not as a missing model. |
| D-057 | 2026-11-23 | Errors found before the `200` header is written are HTTP errors. Afterwards they are in-band `error` events. The `route` event is sent right after the header, before any waiting. | Gives clients one clear rule. |
| D-058 | 2026-11-23 | A machine with a request in flight is treated as up, and marking a machine down never aborts an in-flight request. | A CPU-bound Firmino may not answer health checks while reading a long prompt. |
| D-059 | 2026-11-23 | Defaults for unstated details: `waiting` is sent only if the request must wait (1-based position, re-sent on change); `running` is always sent; waiting V1 requests fail with `MACHINE_DOWN` if their machine goes down; options allow-list as in API.md; config path from `--config` or `PINGPONG_CONFIG`; default port 8787. | Removes ambiguity so an implementer does not guess. |
| D-060 | 2026-11-23 | Owner confirmed FileVault stays off on Mane. The owner states both machines are physically secure. Confirms D-039. | Unattended recovery after a power failure. |
| D-061 | 2026-11-23 | Tailscale access uses a tag-based ACL: `tag:router` (Mane) and `tag:worker` (Firmino and future machines). Members reach the router on 443 and workers on SSH only. A `tests` block guards the policy. A new machine only needs a tag. | Simple, secure by default (deny everything else), and scales without rule changes. |
| D-062 | 2026-11-23 | V1 config file format is YAML (`gopkg.in/yaml.v3`). Durations are strings like `30m`. | Matches the illustrative example in CONFIGURATION.md with minimal dependency. |
| D-063 | 2026-11-23 | V1 router is a single Go binary; web UI is `go:embed` under `internal/api/web/`. Only non-stdlib dependency is `gopkg.in/yaml.v3`. | Keeps RAM and deployment simple. |
| D-064 | 2026-11-23 | V2 keeps `POST /api/chat` connection-tied (V1 semantics). Durable runs use `POST /api/jobs`. | Avoids surprising V1 clients; jobs are opt-in via `storage.data_dir`. |
| D-065 | 2026-11-23 | V2 persistence uses embedded SQLite (`modernc.org/sqlite`, no cgo). | Single binary, works on Mane without a separate DB service. |
| D-066 | 2026-11-23 | V3 queue is two lanes on the V2 job store: `interactive` (default) and `batch`. Interactive is admitted before batch on the same machine. FIFO within a lane. A job already admitted is not preempted. `max_concurrent` stays the per-machine cap. `POST /api/batches` plus `pingpong batch` submit and fetch batches. Live `POST /api/chat` still shares the machine gate. | Matches ROADMAP phase 3 without a broker. |
| D-067 | 2026-11-23 | V4 role choice: among up machines with the role, fewest in-flight, then shortest queue, then file order. All down yields `MACHINE_DOWN` on the first machine. The choice is fixed for that request. The brain stays on Mane; the router is still moved by changing config (P13). | Adding a Mac Studio or DGX Spark is a config entry, not a rule change. |
| D-068 | 2026-11-23 | V5 profiles are named presets (`fast`, `balanced`, `deep` by default) that fill `fast`, `model`, and `machine` when the client leaves them unset. An explicit flag or pin wins. Profiles do not read the prompt. | First deferred item after the queue and extra machines. |
| D-069 | 2026-11-23 | V6 (planned) is operability only: `pingpong doctor` (with an opt-in `--chat` check on the real Ollama), graceful shutdown with a retryable `ROUTER_RESTARTING` error, `pingpong backup` via a consistent SQLite copy, and `GET /api/status` with a Status tab. No metrics stack, no alerting, no hot reload. The status view never shows prompts or outputs. See [V6-OPERABILITY.md](V6-OPERABILITY.md). | The first run of the real binary found four deploy bugs that fake-Ollama tests missed. The owner needs a fast way to find that class of problem. Keeps to P1, P2 and P16. |
| D-070 | 2026-11-23 | V7 (planned) adds model aliases: a name without `:` or `/` that maps to a real model and optionally a machine. An alias is a pin, expanded after profiles and before routing. No chains, no fallback lists, no per-alias options. An explicit request `machine` beats the alias's. An alias that equals an installed untagged model name wins and raises a warning. See [V7-ALIASES.md](V7-ALIASES.md). | Gives profiles and scripts one place to change a model, without changing routing (P9, P10). Supersedes the deferral in D-035 for aliases only. |
