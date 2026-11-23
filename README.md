# Pingpong

A small, private control layer that lets one machine (**Mane**) decide where
local LLM requests run across a few always-on machines on a home network.

You send a request to Mane. Mane either handles it itself (fast, small models)
or hands it to **Firmino** (slower, larger models, long-running work).

> Status: **V1 code complete.** Docs in `docs/`. Router binary `cmd/pingpong`.
> Go-live on your machines: [docs/PRODUCTION.md](docs/PRODUCTION.md).

## The machines

| Name | Hardware | Role |
|---|---|---|
| **Mane** | Mac mini M4, 16 GB unified memory, 256 GB SSD | The brain (router and tiny web page). Fast worker, models up to 8B (`llama3.1:8b`, `llama3.2:3b`) |
| **Firmino** | Lenovo ThinkBook 15 G3 (Ryzen 3 5300U, 40 GB RAM, 512 GB NVMe), Ubuntu 24.04 | Heavy worker. Starts with a 14B model (`qwen2.5:14b`). Long-running work |
| **MacBook Air** | Client | Where you send requests from |

More machines can be added later. They are always controlled through Mane.

## How it works, in one minute

1. Your MacBook Air reaches **Mane** over Tailscale.
2. Mane talks to **Firmino** over the home LAN. Nothing goes over the internet.
3. By default (**auto**), a request goes to Firmino.
4. Add the **fast** flag and it goes to Mane instead.
5. **Pin** a model or machine and it goes exactly there, with no switching.
6. Every reply says which model and machine answered.
7. If the target is busy, the request waits for it. If it is down, you get an
   error (V1) or it retries every 2 minutes for up to 3 hours (V2).
8. From V2, tasks keep running if you close the laptop, and you can read the
   results later.

## Versions

| Version | What it adds |
|---|---|
| V1 | Router, tiny web page, auto, fast and pinned modes, waiting, errors |
| V2 | Durable sessions, saved results (90 days), retries, cancel, history |
| V3 | A simple queue for batch work |
| V4 | More machines, by role |

See [docs/ROADMAP.md](docs/ROADMAP.md).

## Build (V1)

Requires Go 1.22 or newer.

```bash
make tidy && make test && make build
```

Run with a config file outside the repo (see `deploy/config.example.yaml`):

```bash
export PINGPONG_CONFIG=/path/to/your.local.yaml
./bin/pingpong
```

## Documents

Read in this order if you are new. If documents disagree, the constitution wins,
then the PRD, then the rest.

| # | Document | What it is |
|---|---|---|
| 1 | [CONSTITUTION.md](CONSTITUTION.md) | The non-negotiable principles and limits |
| 2 | [docs/PRD.md](docs/PRD.md) | What we are building and why, with requirements and acceptance criteria |
| 3 | [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | How the system is put together |
| 4 | [docs/ROUTING.md](docs/ROUTING.md) | Exactly how requests are routed |
| 5 | [docs/API.md](docs/API.md) | The interface between clients and the router |
| 6 | [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | Settings and defaults |
| 7 | [docs/JOBS-AND-RETRIES.md](docs/JOBS-AND-RETRIES.md) | Request lifecycle, durable sessions, retries |
| 8 | [docs/UI-SPEC.md](docs/UI-SPEC.md) | The tiny built-in web page |
| 9 | [docs/HARDWARE-AND-MODELS.md](docs/HARDWARE-AND-MODELS.md) | The machines, memory budgets, starting models |
| 10 | [docs/NETWORK-AND-SECURITY.md](docs/NETWORK-AND-SECURITY.md) | Access paths, firewall, privacy |
| 11 | [docs/SETUP-GUIDE.md](docs/SETUP-GUIDE.md) | Step-by-step machine setup (Phase 0) |
| 12 | [docs/OPERATIONS.md](docs/OPERATIONS.md) | Keeping the machines healthy |
| | [docs/PRODUCTION.md](docs/PRODUCTION.md) | V1 go-live on Mane and Firmino |
| 13 | [docs/IMPLEMENTATION-PLAN.md](docs/IMPLEMENTATION-PLAN.md) | Build order, milestones, guardrails |
| 14 | [docs/TESTING.md](docs/TESTING.md) | How to verify each version |
| 15 | [docs/ROADMAP.md](docs/ROADMAP.md) | Phases, and what is deferred |
| 16 | [docs/DECISIONS.md](docs/DECISIONS.md) | Log of decisions and the reasons behind them |
| 17 | [docs/GLOSSARY.md](docs/GLOSSARY.md) | Terms used in these documents |
| | [AGENTS.md](AGENTS.md) | Rules for AI coding assistants working in this repo |

## Conventions

- **V1, V2, V3, V4** tag when a feature arrives. Do not build later versions early.
- **Proposed** means a suggestion pending the owner's confirmation. Nothing is
  currently pending.
- Figures for speed and size are estimates until measured.
- This repository is public: no real addresses, keys or login links.

## Out of scope

- RAG (retrieval over documents). It is being explored separately. If it ever
  needs a local model, it can call Mane like any other client.
- High availability. Both machines are assumed to be up.
- Notifications. Deferred.

## License

See [LICENSE](LICENSE).
