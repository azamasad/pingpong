# Pingpong

A small, private control layer that lets one machine (**Mane**) decide where
local LLM requests run across a few always-on machines on a home network.

You send a request to Mane. Mane either handles it itself (fast, small models)
or hands it to **Firmino** (slower, larger models, long-running work).

> Status: planning. This repository currently contains documentation only.
> No code has been written yet.

## The machines

| Name | Hardware | Role |
|---|---|---|
| **Mane** | Mac mini M4, 16 GB unified memory, 256 GB SSD | The brain (router and tiny web page). Fast worker, models up to 8B. |
| **Firmino** | Lenovo ThinkBook 15 G3 (Ryzen 3 5300U, 40 GB RAM, 512 GB NVMe), Ubuntu 24.04 | Heavy worker. Starts with a 14B model. Long-running work. |
| **MacBook Air** | Client | Where you send requests from. |

More machines can be added later. They are always controlled through Mane.

## How it works, in one minute

1. Your MacBook Air reaches **Mane** over Tailscale.
2. Mane talks to **Firmino** over the home LAN. Nothing goes over the internet.
3. By default (**auto**), a request goes to Firmino.
4. Add the **fast** flag and it goes to Mane instead.
5. **Pin** a model or machine and it goes exactly there, with no switching.
6. Every reply says which model and machine answered.

## Documents

Start with the constitution, then the PRD.

| Document | What it is |
|---|---|
| [CONSTITUTION.md](CONSTITUTION.md) | The non-negotiable principles and limits |
| [AGENTS.md](AGENTS.md) | Rules for AI coding assistants working in this repo |
| [docs/PRD.md](docs/PRD.md) | What we are building and why |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | How the system is put together |
| [docs/ROUTING.md](docs/ROUTING.md) | Exactly how requests are routed |
| [docs/JOBS-AND-RETRIES.md](docs/JOBS-AND-RETRIES.md) | Request lifecycle, durable sessions, retries |
| [docs/HARDWARE-AND-MODELS.md](docs/HARDWARE-AND-MODELS.md) | The machines, memory budgets, starting models |
| [docs/NETWORK-AND-SECURITY.md](docs/NETWORK-AND-SECURITY.md) | Access paths, firewall, privacy |
| [docs/UI-SPEC.md](docs/UI-SPEC.md) | The tiny built-in web page |
| [docs/OPERATIONS.md](docs/OPERATIONS.md) | Keeping the machines healthy |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Phases, and what is deferred |
| [docs/DECISIONS.md](docs/DECISIONS.md) | Log of decisions and the reasons behind them |

## Out of scope

- RAG (retrieval over documents). It is being explored separately. If it ever
  needs a local model, it can call Mane like any other client.
- High availability. Both machines are assumed to be up.
- Notifications. Deferred.

## License

See [LICENSE](LICENSE).
