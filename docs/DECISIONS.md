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
