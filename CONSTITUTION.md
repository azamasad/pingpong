# Constitution

These are the rules every other document and every future change is checked
against. If a proposal breaks one of them, either change the proposal or amend
this document deliberately and record why in [docs/DECISIONS.md](docs/DECISIONS.md).

## Principles

**P1. Keep it simple.**
Prefer the smallest thing that works. No message brokers, no clusters, no
orchestration platforms. If a feature needs heavy infrastructure, it does not
belong here.

**P2. Do not over-engineer.**
Build for today's two machines and one user. Leave room for growth by keeping
designs tidy, not by building features nobody has asked for yet.

**P3. Mane is the brain.**
Routing decisions are made on Mane and nowhere else. Other machines do not
route, and clients never talk to workers directly.

**P4. Light on the brain.**
Mane's 16 GB is the scarcest resource. The router, the web page and any
bookkeeping together stay within a few tens of MB of RAM (working budget:
50 MB). Model memory is separate and governed by P5.

**P5. Mane runs small models only.**
Models on Mane are 8B parameters or smaller. Larger models run on Firmino
or on future machines with more memory.

**P6. Workers are lean.**
No web UI and no extra services on Firmino or any other worker beyond what is
needed to serve models, and what keeps the machine healthy and reachable.

**P7. Local traffic stays local.**
Mane talks to workers over the home LAN. Nothing between Mane and a worker
goes via the internet.

**P8. Private by default.**
Reachable only through Tailscale. Never exposed to the public internet.

**P9. Explicit beats clever.**
Routing follows simple, documented rules driven by clear signals: flags,
pinned names, and what is installed where. The router does not read prompts
to guess difficulty.

**P10. Pinned means pinned.**
If the user names a model or a machine, that is exactly what is used. No
silent substitution, no switching mid-task. If it is unavailable, say so.

**P11. Be transparent.**
Every response reports which model and which machine produced it. Waiting,
retrying and failing are visible states, never silent.

**P12. Design for more machines.**
Machines are described by roles and capabilities in a registry, not
hard-coded names in the rules. Adding a machine must not require rewriting
the routing rules.

**P13. The brain can move.**
The router is a small, self-contained service driven by configuration, so it
could be moved to another machine later (for example a Mac Studio or DGX
Spark) with little effort.

**P14. Work survives the client.**
Long-running tasks must eventually be able to finish with no client
attached, so closing the laptop never loses work. This is a firm
requirement, scheduled after the first version (see the roadmap).

**P15. Decisions are written down.**
Significant choices and their reasons go in
[docs/DECISIONS.md](docs/DECISIONS.md) so they are not re-debated.

**P16. Never log content.**
Prompts and model outputs are never written to logs. In V2 they are stored
only in the saved-request store, for the retention period.

**P17. Public repository, private system.**
The repository is public. It never contains real addresses, keys, tokens,
login links, or real configuration. Only placeholders.

**P18. Documents win over assumptions.**
If the docs and an implementation disagree, resolve it deliberately: fix
whichever is wrong and record why. Precedence is this constitution, then the
PRD, then the other documents.

## Hard limits

| Limit | Value |
|---|---|
| Router and web page RAM on Mane | Working budget 50 MB |
| Largest model on Mane | 8B parameters |
| Users | One (the owner) |
| Logins | None. Tailscale is the access control. |
| Public exposure | None |
| Retry window for an unavailable machine | Every 2 minutes, up to 3 hours, then stop |
| Retention of finished results (when stored) | 90 days |
| Logging of prompts or outputs | Never |
| Concurrency per machine (default) | 1 request in flight |

## Non-goals

- RAG, document search or vector databases.
- High availability or automatic failover.
- Distributed inference that splits one model across machines.
- Multi-user accounts, billing or quotas.
- Notifications (deferred).
- Automatic prompt classification (deferred).

## Amending this document

Change a principle only on purpose. Add an entry to
[docs/DECISIONS.md](docs/DECISIONS.md) explaining what changed and why.
