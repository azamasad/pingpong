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

**V1 to V5 are built and merged to `main`.** V3 lanes are D-066, V4 role choice is D-067, V5 presets are D-068.

**V6 and V7 are specified but not built.** See [docs/V6-OPERABILITY.md](docs/V6-OPERABILITY.md) (D-069) and [docs/V7-ALIASES.md](docs/V7-ALIASES.md) (D-070). Do not code them until the owner says so.

| Milestone | Status |
|---|---|
| M0–M8a, M9a | Done (`main`) |
| V2 (store, runner, API, retries, recovery, job UI) | Done |
| V3 lanes and batches, V4 role choice, V5 profiles | Done |
| Automated tests | `docs/TESTING.md` §2 in CI, plus a smoke run of the real binary against stand-in Ollama servers |
| M8b, M9b, M10 | **Owner** on real machines. Not yet done |
| V6 (M11a to M15), V7 (M16 to M20) | Specified. Not started |

The real Ollama stream format has never been checked against the router
(D-052). Until the owner has run the real-machine checks in
[docs/PRODUCTION.md](docs/PRODUCTION.md), treat any assumption about real
Ollama as unverified.

Run: `make test && make build`. Enable jobs with `storage.data_dir` in config (`deploy/config.example.yaml`).

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
- **Keep deferred items deferred.** Notifications, conversation threads, an
  OpenAI-compatible API, high availability and RAG are out of scope until the
  roadmap says otherwise. V6 and V7 are specified but are built only when the
  owner says so.
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