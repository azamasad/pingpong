# Instructions for AI coding assistants

Read this before touching the repository.

## Read first

1. [CONSTITUTION.md](CONSTITUTION.md). It overrides everything else.
2. [docs/PRD.md](docs/PRD.md) for what is in and out of scope.
3. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/ROUTING.md](docs/ROUTING.md)
   before changing any behaviour.

## Rules

- **Do not over-engineer.** Do the smallest thing that satisfies the PRD. Do
  not add queues, brokers, databases, frameworks or abstractions that the PRD
  does not call for.
- **Respect the memory budget.** The router and web page on Mane stay within
  the budget in the constitution (working budget 50 MB). Avoid heavyweight
  dependencies and runtimes.
- **Do not add features that are not in the PRD.** If something seems useful,
  propose it by adding to the backlog in [docs/ROADMAP.md](docs/ROADMAP.md).
  Do not build it.
- **Keep deferred items deferred.** Profiles, the full queue, notifications,
  high availability and RAG are out of scope until the roadmap says otherwise.
- **Routing is rule-based.** Never route by reading or classifying prompt
  content.
- **Pinned means pinned.** Never substitute a different model or machine for a
  pinned request.
- **No UI on workers.** The only web page lives on Mane.
- **Mane-to-worker traffic stays on the LAN.**
- **No secrets in the repository.** It is public. No keys, tokens, LAN or
  Tailscale addresses, or login links.
- **Update the docs with the change.** If behaviour changes, update the
  matching document and add an entry to [docs/DECISIONS.md](docs/DECISIONS.md)
  when a decision was made.

## When unsure

Ask the owner. Do not guess on anything that affects routing, memory use,
security, or scope.
