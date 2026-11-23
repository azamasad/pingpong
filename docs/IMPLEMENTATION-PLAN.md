# Implementation Plan

Status: draft v1. Written for whoever builds this (including an AI coding
agent). Read the [constitution](../CONSTITUTION.md) and [PRD](PRD.md) first.
This document says what to build, in what order, and how to know each step is
done. It does not contain code.

## 1. Ground rules

- **Build V1 only** until the owner says otherwise. V2 and later are specified
  so V1 does not block them, not so they get built early.
- **Smallest thing that works.** No frameworks, brokers or abstractions that
  the docs do not call for.
- **Respect the RAM budget** (50 MB working budget for the router on Mane).
- **Docs are the source of truth.** If implementation forces a change, update
  the doc and add an entry to [DECISIONS.md](DECISIONS.md) in the same change.
- **The owner does the machine setup.** Phase 0 (power settings, firewall,
  Tailscale, installing Ollama and models) happens on real hardware that a
  coding agent cannot reach. Do not assume it is done. Produce checklists
  (see [OPERATIONS.md](OPERATIONS.md)) and build against a fake Ollama.

## 2. Recommended technology (proposed)

Proposed, pending owner confirmation (see D-024). Proceed with this unless the
owner says otherwise.

| Choice | Recommendation | Reason |
|---|---|---|
| Language | **Go** | One small static program. Typically a few tens of MB of RAM or less at idle. Standard library covers HTTP, streaming and JSON. Can embed the web page in the program. Cross-compiles for macOS on Apple silicon and Linux, which helps if the brain moves (P13) |
| Web page | Plain HTML, CSS and a little JavaScript, embedded in the program | No build tooling, no framework, no external assets |
| V2 storage | SQLite in a single file on Mane | Embedded, small, reliable, no server. Plain files are an acceptable alternative |
| Dependencies | As few as possible. Justify each | RAM budget, simplicity |
| Process supervision | `launchd` on macOS | Starts on boot, restarts on failure |

Alternatives considered:

| Option | Why not the default |
|---|---|
| Python with a web framework | Usually uses noticeably more memory and needs a runtime and packages on Mane. Risky against the 50 MB budget |
| Node.js | Same concern, plus a runtime to maintain |
| Rust | Also meets the budget, but slower to iterate for a small project |

If another language is chosen, the RAM budget must still be met and measured.

## 3. Suggested repository layout

Illustrative. Adjust sensibly.

| Path | Contents |
|---|---|
| `cmd/` | The program's entry point |
| `internal/config` | Reads and validates configuration |
| `internal/registry` | Machines, roles, installed models, health |
| `internal/route` | The pure routing decision |
| `internal/gate` | Per-machine concurrency and the wait list |
| `internal/ollama` | Client for Ollama's HTTP API |
| `internal/api` | HTTP handlers and streaming |
| `internal/store` | V2 only. Saved requests |
| `web/` | The tiny page |
| `deploy/` | Example config with placeholders, launchd service example |
| `testdata/` | Fake Ollama scenarios |
| `docs/` | These documents |

## 4. V1 milestones

Each milestone ends with passing tests and updated docs. Work on a branch per
milestone, small commits, and a pull request.

| # | Milestone | What it delivers | Done when |
|---|---|---|---|
| M0 | Scaffold | Repo layout, build, test command, lint, a simple CI check | Build and tests run from a clean checkout |
| M1 | Config | Load and validate configuration per [CONFIGURATION.md](CONFIGURATION.md) | Bad configs produce clear errors. Example config with placeholders is committed. No real addresses |
| M2 | Ollama client and fake Ollama | List models, stream a chat, cancel. A scriptable fake server for tests | Client works against the fake. Cancel closes the upstream call |
| M3 | Registry and health | Machine list, roles, installed models, reachability, refresh rules | Down and up transitions and model refresh covered by tests |
| M4 | Routing decision | Pure function implementing [ROUTING.md](ROUTING.md) | Every case in [TESTING.md](TESTING.md) 2.1 passes |
| M5 | Concurrency gate | Per-machine limit, first-come first-served waiting, removal on disconnect | Cases in TESTING 2.2 pass |
| M6 | HTTP API | Endpoints and streaming per [API.md](API.md), errors, Host and Origin checks, body limit, timeouts | Cases in TESTING 2.3 pass |
| M7 | Tiny web page | Per [UI-SPEC.md](UI-SPEC.md) V1 | Works on laptop and phone widths. No external assets. Small |
| M8 | Packaging | Build for macOS (Apple silicon) and Linux. Service definition example for `launchd`. Deployment notes | Router runs as a service in a clean environment |
| M9 | Resource check | Measure router memory idle and under load | Under the 50 MB working budget. Result recorded in [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) |
| M10 | Acceptance | Owner runs the manual acceptance table in TESTING section 4 on real machines | All V1 criteria in the PRD pass |

Order is a suggestion. M2 to M5 can proceed in parallel after M1.

## 5. V2 outline (do not build yet)

| # | Milestone | Notes |
|---|---|---|
| V2-1 | Store | SQLite file, schema per [JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md), retention sweep |
| V2-2 | Detached execution | Requests run independent of the client. Output saved as produced |
| V2-3 | Job endpoints | Submit, list, get, events with resume, cancel, delete |
| V2-4 | Retry | Every 2 minutes up to 3 hours from first failure, mid-task restart from the beginning |
| V2-5 | Restart recovery | Rules in JOBS-AND-RETRIES |
| V2-6 | Page: history | List, open, reattach, cancel, retry status |

## 6. Things to decide early in implementation

Record each in [DECISIONS.md](DECISIONS.md).

| Question | Guidance |
|---|---|
| Config file format | Any simple, human-editable format. Keep setting names as in CONFIGURATION |
| Which Ollama endpoints | Check the installed version's API. Note it in [API.md](API.md) section 6 |
| Exact default models | The owner chooses at setup. Use placeholders in committed files |
| Flag and pin controls in the page | Keep minimal: a fast toggle and two optional selectors |

## 7. Guardrails (do not do these)

- Do not add RAG, vector search or document features.
- Do not add logins, users, or accounts.
- Do not add a queue, profiles, notifications or automatic task classification.
- Do not put any UI or router on Firmino.
- Do not route by reading prompts.
- Do not substitute for pinned requests.
- Do not log prompts or outputs.
- Do not commit real addresses, keys or login links. The repo is public.
- Do not add heavy dependencies to save a few lines.

## 8. Working agreement for agents

- Use the branch naming your environment requires, make small commits, and open
  one pull request per milestone.
- Tests with every behaviour change.
- Run the build and tests before pushing.
- Update the matching docs in the same change.
- If a doc is unclear or conflicts with another, stop and ask the owner,
  then fix the doc. Precedence: constitution, then PRD, then the other docs.
