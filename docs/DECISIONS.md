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
| D-014 | 2026-11-23 | The retry loop arrives in V2. In V1, a down target returns an error immediately. | Retrying needs requests to outlive the client, which is V2's saved-request store. Recorded as an assumption for the owner to confirm. |
| D-015 | 2026-11-23 | Durable sessions are required but come after V1. | The owner wants to close the laptop and return later. V1 stays smaller. |
| D-016 | 2026-11-23 | Finished results are kept for 90 days. | The owner's choice. Text is small. |
| D-017 | 2026-11-23 | A simple queue is deferred but expected soon (V3). It builds on V2's store. | The owner expects to need it. No heavy infrastructure. |
| D-018 | 2026-11-23 | Profiles are deferred. V1 has auto, `fast`, and pinned. | Do not over-engineer. |
| D-019 | 2026-11-23 | Notifications are deferred. | Not needed yet. |
| D-020 | 2026-11-23 | High availability is out of scope. | Both machines are assumed up. |
| D-021 | 2026-11-23 | Machines are described by roles in a registry. | Easier to add machines such as a Mac Studio or DGX Spark. |
| D-022 | 2026-11-23 | The router is movable to another machine. | A larger machine may become a better home later. |
| D-023 | 2026-11-23 | Docs live in this repository, with documents under `docs/`. | The owner's instruction. |
| D-024 | 2026-11-23 | Proposed: build the router in Go, as one small program with the page embedded. Pending owner confirmation. | Small memory footprint against the 50 MB budget, easy service deployment, cross-compiles for macOS and Linux. Python and Node are likelier to strain the budget. |
| D-025 | 2026-11-23 | Proposed: V2 stores saved requests in a single SQLite file on Mane. Plain files are an acceptable alternative. Pending owner confirmation. | Embedded, small, reliable, no server process. |
| D-026 | 2026-11-23 | Machines have a concurrency limit, default 1. Requests wait first come, first served. | Fits CPU inference on Firmino and keeps behaviour predictable. Mane's own direct use of Ollama is not counted. |
| D-027 | 2026-11-23 | In V1, closing the client connection cancels the request, including upstream, so the worker stops. | Avoids wasted CPU. Surviving disconnects is V2. |
| D-028 | 2026-11-23 | The router keeps no conversation state in V1. The client sends full history. | Keeps V1 small. |
| D-029 | 2026-11-23 | Pins override `fast`. A pinned model on several machines with no machine named is an error. | Pinned means pinned, and no guessing. |
| D-030 | 2026-11-23 | The retry window runs from the first failure and is not reset by later failures. | The owner's rule: retry every 2 minutes until 3 hours, then stop. |
| D-031 | 2026-11-23 | A `cancelled` state and explicit cancel and delete are added for V2. | A 3-hour retry loop needs a way to stop it. |
| D-032 | 2026-11-23 | Prompts and outputs are never written to logs. | Privacy. V2 stores them only in the saved-request store. |
| D-033 | 2026-11-23 | The router checks Host and Origin headers. | It has no login, and this blocks other web pages from using it through the browser. |
| D-034 | 2026-11-23 | Timeouts are generous by default: 20 minutes to first token, 5 minutes between tokens. They are starting values to tune. | CPU inference on long prompts can take minutes. |
| D-035 | 2026-11-23 | An OpenAI-compatible endpoint, model aliases, and conversation threads are deferred. | Not needed for V1. Do not over-engineer. |
| D-036 | 2026-11-23 | Real addresses and config are never committed. Only placeholders. | The repository is public. |
