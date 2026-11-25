# Implementation Plan

Status: v3. V1 to V7 are built and live. V7.1 to V7.9 are specified and approved (section 5c). Written for whoever builds this (including an AI coding
agent). Read the [constitution](../CONSTITUTION.md) and [PRD](PRD.md) first.
This document says what to build, in what order, and how to know each step is
done. It does not contain code.

## 0. V1 on one page

If anything below seems to conflict with a longer document, stop and check.
This is the short version of what V1 must do.

- One small Go program on Mane. It serves the page and the API. Listens on
  localhost only. Tailscale Serve publishes it.
- Config file lists machines: name, Ollama address, roles (`fast` or `heavy`),
  default model, max concurrent requests (1), context size (8192). Mane is
  `fast`, Firmino is `heavy`.
- `POST /api/chat` takes `prompt` or `messages`, plus optional `fast`, `model`,
  `machine`. It streams NDJSON events: `route`, `status`, `ping`, `token`,
  `thinking`, `done` or `error`.
- Routing: pins win (and override `fast`). Else `fast` goes to the `fast`
  machine. Else it goes to the `heavy` machine. Pinned model on several
  machines with no machine named is an error. Never substitute, never reroute.
- Each machine runs one request at a time. Others wait, first come first served.
  Waiting shows as a status. A down machine is an immediate error (no retry in V1).
- Client disconnect cancels the request, including the call to Ollama.
- Every answer reports machine, model and mode.
- The router always sends `num_ctx` (8192) and watches for a prompt that fills it.
- Health checks every 30 s. A machine with a request in flight counts as up, and
  is never marked down mid-request.
- Heartbeat `ping` every 15 s on open streams.
- Order: validate, Host/Origin, route, down check, then write the `200` header and
  `route`, then wait, then run. Before the header: HTTP errors. After: in-band.
- Also in V1: Host/Origin checks, a body size limit, `stream:false`,
  `GET /api/machines`, `/api/models`, `/api/health`, `context_warning` on `done`,
  and `MODELS_UNKNOWN` when a pinned model cannot be found but a machine was never
  reached.
- Timeouts: first chunk 30 min, idle 5 min, total 2 h. No server-wide write
  timeout on streams.
- Never log prompts or outputs. Memory stays under about 50 MB.
- The page is plain HTML and JS embedded in the program, no external assets.
- No queue, no saved history, no retries, no logins. Those are later versions.

## 1. Ground rules

- **Build only what the owner has approved.** V1 to V7 are built. V7.1 to V7.7
  are approved and specified, built one at a time in the order of section 5c.
  V7.8 and V7.9 are approved too and are built after V7.7. Anything else in
  the roadmap is not to be built early.
- **Smallest thing that works.** No frameworks, brokers or abstractions that
  the docs do not call for.
- **Respect the RAM budget** (50 MB working budget for the router on Mane).
- **Docs are the source of truth.** If implementation forces a change, update
  the doc and add an entry to [DECISIONS.md](DECISIONS.md) in the same change.
- **The owner does the machine setup.** Phase 0 (power settings, firewall,
  Tailscale, installing Ollama and models) happens on real hardware that a
  coding agent cannot reach. Do not assume it is done. The checklist is
  [SETUP-GUIDE.md](SETUP-GUIDE.md). Build against a fake Ollama.

## 2. Technology (confirmed by the owner)

Confirmed in D-024, D-025 and D-037. Take a light approach: the least code and
the fewest dependencies that meet the PRD, preferring the standard library.

| Choice | Recommendation | Reason |
|---|---|---|
| Language | **Go** | One small static program. Typically a few tens of MB of RAM or less at idle. Standard library covers HTTP, streaming and JSON. Can embed the web page in the program. Cross-compiles for macOS on Apple silicon and Linux, which helps if the brain moves (P13) |
| Web page | Plain HTML, CSS and a little JavaScript, embedded in the program | No build tooling, no framework, no external assets |
| V2 storage | SQLite in a single file on Mane | Embedded, small, reliable, no server. Plain files are an acceptable alternative |
| Dependencies | As few as possible. Justify each | RAM budget, simplicity |
| Process supervision | `launchd` user launch agent on macOS | Starts at automatic login, restarts on failure |

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
| `deploy/` | Example config with placeholders, launchd launch agent example |
| `testdata/` | Fake Ollama scenarios |
| `docs/` | These documents |

## 4. V1 milestones

Each milestone ends with passing tests and updated docs. Work on a branch per
milestone, small commits, and a pull request.

| # | Milestone | What it delivers | Done when |
|---|---|---|---|
| M0 | Scaffold | Repo layout, build, test command, lint, a simple CI check, and a `.gitignore` that excludes real config files and data directories (use a clear naming rule such as `*.local.*`) | Build and tests run from a clean checkout. Committing a real config by accident is prevented |
| M1 | Config | Load and validate configuration per [CONFIGURATION.md](CONFIGURATION.md) | Bad configs produce clear errors. Example config with placeholders is committed. No real addresses |
| M2 | Ollama client and fake Ollama | List models, stream a chat (content and `thinking` text), cancel. A scriptable fake server for tests, following [API.md](API.md) section 6. **Check the fake against a real Ollama** using a tiny model (for example a 0.5B model) wherever the build environment allows it, and record any difference from the docs | Client works against the fake and, where possible, a real Ollama. Cancel closes the upstream call. Differences are written into API.md section 6 |
| M3 | Registry and health | Machine list, roles, installed models, reachability, refresh rules | Down and up transitions and model refresh covered by tests |
| M4 | Routing decision | Pure function implementing [ROUTING.md](ROUTING.md) | Every case in [TESTING.md](TESTING.md) 2.1 passes |
| M5 | Concurrency gate | Per-machine limit, first-come first-served waiting, removal on disconnect | Cases in TESTING 2.2 pass |
| M6 | HTTP API | Endpoints and streaming per [API.md](API.md), errors, Host and Origin checks, body limit, timeouts | Cases in TESTING 2.3 pass |
| M7 | Tiny web page | Per [UI-SPEC.md](UI-SPEC.md) V1 | Works on laptop and phone widths. No external assets. Small |
| M8a | Packaging (agent) | Build for macOS (Apple silicon) and Linux. A `launchd` user launch agent example using absolute paths (it starts at automatic login, as in [SETUP-GUIDE.md](SETUP-GUIDE.md)). Deployment notes, including the Tailscale Serve step | Binaries build. Example plist is valid (`plutil -lint`) |
| M8b | Install (owner) | Install on Mane, reboot test, Tailscale Serve, macOS Local Network check, ACL edits | Router comes back after a reboot and reaches Firmino from the launch agent |
| M9a | Resource check (agent) | On Linux, measure router resident memory (a proxy for macOS) idle and under the defined load (PRD N17) | Under 50 MB. Result recorded in [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) |
| M9b | Resource check (owner) | Repeat on Mane (`ps` or Activity Monitor) | Under 50 MB |
| M10 | Acceptance | Owner runs the manual acceptance table in TESTING section 4 on real machines | All V1 criteria in the PRD pass |

Order is a suggestion. M2 to M5 can proceed in parallel after M1.

Light approach: milestones may be combined (for example M2 with M3, or M6 with
M7). Use one pull request per milestone or per sensible group, as long as every
"done when" is still met. Do not skip tests or the memory check (M9a, M9b).

### Decide by default

If something is not specified, use these. Record anything new in DECISIONS.

| Question | Default |
|---|---|
| Config file location | `--config` flag, else `PINGPONG_CONFIG`. No hidden default |
| Router port | 8787, configurable |
| Which options pass to Ollama | `temperature`, `top_p`, `top_k`, `min_p`, `seed`, `num_predict`, `repeat_penalty`, `stop`, `num_thread`. Client `num_ctx` is ignored. `keep_alive` is never sent |
| Model name in `route` and `done` | The registry's installed name |
| Two machines with the same role | First in file order, with a warning |
| One machine with two roles | Allowed |
| Timestamps | RFC 3339, UTC |
| Log format | One JSON line per event. Fields only: time, level, event, machine, model, mode, state, duration_ms, error_code, message |
| Go layout | `internal/` is a suggestion, not a requirement |
| Memory budget measure | Resident memory of the process |
| Waiting request when its machine goes down (V1) | Fails with an in-band `MACHINE_DOWN` |

## 5. V2 (done on `cursor/v2-jobs-aa2f`, PR #3)

| # | Milestone | Status |
|---|---|---|
| V2-1 | Store | Done |
| V2-2 | Detached execution | Done |
| V2-3 | Job endpoints | Done |
| V2-4 | Retry | Done |
| V2-5 | Restart recovery | Done |
| V2-6 | Page: history | Done (filter, open, reattach, cancel, delete) |

Owner hardware acceptance: [TESTING.md](TESTING.md) §4 after merge.

## 5a. V3 to V5 (done, merged to `main`)

| Version | Work | Status |
|---|---|---|
| V3 | Lanes, `POST /api/batches`, `pingpong batch` (D-066) | Done |
| V4 | Role choice among several machines (D-067) | Done |
| V5 | Named profiles, `GET /api/profiles`, page selector (D-068) | Done |

Hardware acceptance for all of them is the owner's: [PRODUCTION.md](PRODUCTION.md)
section 4 and [TESTING.md](TESTING.md) section 4.

## 5b. V6 and V7 (done, merged to `main`)

| Version | Spec | Milestones |
|---|---|---|
| V6 operability | [V6-OPERABILITY.md](V6-OPERABILITY.md) (D-069) | M11a, M11b, M12, M13, M14, M15 |
| V7 model aliases | [V7-ALIASES.md](V7-ALIASES.md) (D-070) | M16 to M20 |

All built (M11a to M14, M16 to M19). M15 and M20 were the owner's checks on the
real machines, and are done in effect: `doctor --chat` passes on both machines
(see [PRODUCTION.md](PRODUCTION.md)). The Stress tab, bench scripts and the
per-request log line were added afterwards (D-075 to D-077) and are not
numbered milestones.

## 5c. Ready for build: V7.1 to V7.9

Verdict of the readiness review (D-106): **V7.1 to V7.9 are ready to build.
The owner answered every open question (D-116) and the constitution was amended
for notifications (D-117). Nothing waits on the owner.**
The contract for all of them is [NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md).
Intent and scope: [NEXT-FEATURES-PRDS.md](NEXT-FEATURES-PRDS.md). Tests: [TESTING.md](TESTING.md) section 2.10.

**Start here.**

1. Read, in order: [CONSTITUTION.md](../CONSTITUTION.md), this section, the build spec sections 0 and 1, then the section for the version you are building.
2. Branch `cursor/<name>-<suffix>` from the current `main`. One version per pull request. Do not start the next version before the previous one is merged and its done list is ticked.
3. First pull request (V7.1) begins with the test tooling in build spec 1.9 (fake Ollama records calls and replies per call), then the `batches` table, atomic submit, the paged batch view, the Batches tab, and split.
4. Run `make lint && make test && make build` before every push.

| Milestone | Version | Needs | Done when |
|---|---|---|---|
| M21 | V7.1 split, `batches`, Batches tab | none | Build spec section 2 acceptance and the common done list |
| M22 | V7.2 answer checking | M21 | Section 3 |
| M23 | V7.3 templates | none | Section 4 |
| M24 | V7.4 export | M21, M22 | Section 5 |
| M25 | V7.5 threads | none | Section 6 |
| M26 | V7.6 file input and reduce | M21, M22, M23 | Section 7 |
| M27 | V7.7 schedules | M21, M23 | Section 8 |
| M28 | V7.8 OpenAI endpoint | M27 merged | Section 9 |
| M29 | V7.9 notifications | M21, M27 | Section 10 |

**Per-milestone done (the common list, build spec section 0).** Lint, tests and
build pass. Smoke run of the real binary against two stand-in Ollama servers.
Router RSS measured and recorded, under 50 MB, growth within the section's limit.
No content in logs. No substitution, no rerouting. Docs updated in the same
pull request with a DECISIONS entry for anything new. Config example updated
with placeholders only. Owner's real-machine checks listed in TESTING 4b.

**Guardrails that apply to every one of them.** Never log prompts or outputs
(new log fields only through the `logx` whitelist). A pinned or stored pair is
used exactly, a down machine means wait and retry on it (3 hours, every 2
minutes), never reroute. Routing is rule-based and never reads content, and
the router never parses a model's answer to decide anything. No new service,
broker, framework or dependency without a written reason (P1, P2), and none is
expected for V7.1 to V7.9. Budget 50 MB (P4). No real addresses, keys or
webhook URLs in the repository (P17). Machine setup and real-hardware checks
stay with the owner.

**If something is unclear**, ask the owner. If you cannot, choose the simplest
option that obeys the constitution, record it in DECISIONS (D-053).

## 6. Things to decide early in implementation

Record each in [DECISIONS.md](DECISIONS.md).

| Question | Guidance |
|---|---|
| Config file format | Any simple, human-editable format. Keep setting names as in CONFIGURATION |
| Which Ollama endpoints | Check the installed version's API. Note it in [API.md](API.md) section 6 |
| Default models | Chosen: `llama3.1:8b` and `llama3.2:3b` on Mane (D-038), and on Firmino `qwen3:30b-a3b-instruct-2507-q4_K_M` (D-073, replacing `qwen2.5:14b`). Put them in the example config. Do not hard-code them |
| Flag and pin controls in the page | Keep minimal: a fast toggle and two optional selectors |

## 7. Guardrails (do not do these)

- Do not add RAG, embeddings, vector search or document retrieval. Chunking a text file to run as a batch (V7.6) is allowed: the constitution non-goal was reworded for it (D-101).
- Do not add logins, users, or accounts.
- Do not add notifications beyond the V7.9 webhook (one optional message, no content), an OpenAI endpoint beyond V7.8, or automatic task classification. (The queue and profiles are built, V3 and V5.)
- Do not put any UI or router on Firmino.
- Do not route by reading prompts.
- Do not substitute for pinned requests.
- Do not log prompts or outputs.
- Do not commit real addresses, keys or login links. The repo is public.
- Do not add heavy dependencies to save a few lines.

## 8. Working agreement for agents

- Use the branch naming your environment requires, make small commits, and open
  one pull request per milestone or sensible group of milestones.
- Tests with every behaviour change.
- Run the build and tests before pushing.
- Update the matching docs in the same change.
- If a doc is unclear or conflicts with another, ask the owner, then fix the doc.
  If you cannot ask (for example you are running unattended), choose the
  simplest option that obeys the constitution, write the choice and your
  reasoning in the pull request and in [DECISIONS.md](DECISIONS.md), and move on.
  Precedence: constitution, then PRD, then the other docs.
