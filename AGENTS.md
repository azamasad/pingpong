# Instructions for AI coding assistants

Read this before touching the repository.

## Read first, in this order

1. [CONSTITUTION.md](CONSTITUTION.md). It overrides everything else.
2. [docs/PRD.md](docs/PRD.md) for what is in and out of scope, and which
   version each requirement belongs to.
3. [docs/IMPLEMENTATION-PLAN.md](docs/IMPLEMENTATION-PLAN.md) for build order,
   milestones and guardrails.
4. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), [docs/ROUTING.md](docs/ROUTING.md),
   [docs/API.md](docs/API.md) and [docs/CONFIGURATION.md](docs/CONFIGURATION.md)
   before changing any behaviour.
5. [docs/TESTING.md](docs/TESTING.md) for what to test and how.
6. [docs/SETUP-GUIDE.md](docs/SETUP-GUIDE.md) only for context. It is the owner's
   checklist for the real machines, and it lists the settings your code should
   assume (localhost listening, 8192 context, one request per machine).

If documents disagree: constitution, then PRD, then the rest. If something is
unclear or conflicting, stop and ask the owner, then fix the docs.

## Current state

**V1 ready for production** on branch `cursor/v1-implementation-aa2f` ([PR #2](https://github.com/azamasad/pingpong/pull/2)).
CI green. Owner steps: [docs/PRODUCTION.md](docs/PRODUCTION.md), [SETUP-GUIDE.md](docs/SETUP-GUIDE.md) M8b–M10.

| Milestone | Status |
|---|---|
| M0 Scaffold | Done |
| M1 Config | Done |
| M2 Ollama client + fake | Done (`internal/ollama`, `internal/fakeollama`) |
| M3 Registry + health | Done (`internal/registry`) |
| M4 Routing | Done (`internal/route`) |
| M5 Gate | Done (`internal/gate`) |
| M6 HTTP API | Done (`internal/api`) |
| M7 Web UI | Done (embedded in `internal/api/web/`) |
| M8a Packaging | Done (`deploy/`, `deploy/BUILD.md`, launchd example) |
| M9a Memory check | Linux proxy test (`memory_linux_test.go`); **M9b on Mane still owner** |
| M8b, M10 | **Owner** on real machines (SETUP-GUIDE, TESTING §4) |

Run: `make test && make build`. Config: `deploy/config.example.yaml` (placeholders only).

Do not start V2 (SQLite jobs, retries, history UI).

## Rules

- **Do not over-engineer.** Do the smallest thing that satisfies the PRD. Do
  not add queues, brokers, databases, frameworks or abstractions that the PRD
  does not call for.
- **Respect the memory budget.** The router and web page on Mane stay within
  the working budget of 50 MB. Avoid heavyweight dependencies and runtimes.
  Justify every dependency.
- **Do not add features that are not in the PRD.** If something seems useful,
  propose it by adding to the backlog in [docs/ROADMAP.md](docs/ROADMAP.md).
  Do not build it.
- **Keep deferred items deferred.** Profiles, the full queue, notifications,
  high availability, aliases, an OpenAI-compatible API and RAG are out of scope
  until the roadmap says otherwise.
- **Routing is rule-based.** Never route by reading or classifying prompt
  content.
- **Pinned means pinned.** Never substitute a different model or machine for a
  pinned request.
- **Wait, do not reroute.** A request waits for its intended machine.
- **No UI or router on workers.** The only web page lives on Mane.
- **Mane-to-worker traffic stays on the LAN.**
- **Never log prompts or outputs.**
- **No secrets or real addresses in the repository.** It is public. No keys,
  tokens, LAN or Tailscale addresses, login links, or real config files. Commit
  placeholders only.
- **You cannot reach the real machines.** Phase 0 setup is done by the owner.
  Build and test against a fake Ollama, and give the owner checklists for
  anything that must happen on real hardware.

## Working agreement

- Small commits, one pull request per milestone, using the branch naming your
  environment requires.
- Add tests with every behaviour change. Make time-dependent code testable
  with an injectable clock.
- Run the build and tests before pushing.
- Update the matching docs in the same change, and add an entry to
  [docs/DECISIONS.md](docs/DECISIONS.md) when a decision is made.
- Measure the router's memory use before calling a milestone done (M9).

## Definition of done for a milestone

1. The behaviour matches the docs.
2. Tests for it pass.
3. Docs are updated, with any decision recorded.
4. No guardrail in the implementation plan is broken.
5. No real addresses or secrets are committed.

## When unsure

Ask the owner. Do not guess on anything that affects routing, memory use,
security, or scope.

## V1 on one page

See [docs/IMPLEMENTATION-PLAN.md](docs/IMPLEMENTATION-PLAN.md) section 0. If code
and docs disagree, fix whichever is wrong before shipping.

## Unattended-decision rule

If you cannot ask the owner, pick the simplest option that obeys the
constitution, record it in DECISIONS, and continue (D-053).