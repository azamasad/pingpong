# Glossary

| Term | Meaning |
|---|---|
| **Pingpong** | This project: the control layer that routes requests between machines |
| **Mane** | The Mac mini M4 (16 GB). The brain and the fast worker |
| **Firmino** | The Ubuntu laptop (40 GB RAM, CPU only). The heavy worker |
| **MacBook Air** | The owner's client machine |
| **Brain** | The router and its page, running on Mane. The only place routing is decided |
| **Router** | The small service on Mane that accepts requests, chooses a machine and model, and forwards them |
| **Worker** | A machine that runs models. Mane (also the brain) and Firmino today |
| **Registry** | The router's list of machines, roles and installed models |
| **Role** | A label on a machine, such as `fast` or `heavy`. Rules refer to roles, not machine names |
| **Auto** | Default routing: the request goes to the `heavy` role (Firmino) |
| **Fast** | A flag that sends the request to the `fast` role (Mane) |
| **Pinned** | The user names a model and/or machine. It is used exactly, with no substitution |
| **Mode** | How a target was chosen: auto, fast, or a pinned variant |
| **Request** | One message (or conversation turn) sent to a model |
| **Waiting** | A request held until its intended machine is free |
| **Busy** | A machine with a request in flight (up to its concurrency limit) |
| **Retrying** | (V2) A request waiting for its next attempt after the machine was down or failed |
| **Durable session** | (V2) A request that keeps running with no client attached, with results saved |
| **Job** | (V2) The saved record of a request, with status history and output |
| **Queue** | (V3) A simple lane system for batch work, built on the saved-request store |
| **Lane** | (V3) `interactive` (default) or `batch`. Interactive work on a machine is admitted before batch work |
| **Batch** | (V3) A group of jobs submitted together on the batch lane, from `POST /api/batches` or `pingpong batch` |
| **Profile** | (V5) A named preset such as `fast`, `balanced` or `deep` that fills `fast`, `model` and `machine` when a request leaves them unset |
| **Alias** | (V7) A short name for a real model, defined in config. It acts as a pin |
| **Doctor** | (V6) `pingpong doctor`, a command that checks config, storage and machines. `--chat` also sends a tiny real request. Its speed is a 2-token sample, not a benchmark |
| **Split (planned, V7.1)** | Running one batch on several machines at once, each with its own pinned model |
| **Check (planned, V7.2)** | An explicit second step where a user-chosen model reviews or revises an answer |
| **Template (planned, V7.3)** | A saved prompt with `{{blanks}}` |
| **Stress run** | A server-side soak, started from the Stress tab, that keeps short batch jobs flowing to Mane, Firmino or both for up to 2 hours |
| **Mixture-of-experts (MoE)** | A model with many parameters in total but only a few used per token (the Firmino model has 30B in total and about 3.3B active). Much faster on CPU than a dense model of the same size |
| **Dense model** | A model that uses all its parameters for every token (for example `qwen2.5:14b`). Slower on CPU |
| **Tailnet** | The private Tailscale network the owner's devices are on |
| **LAN** | The home local network. Mane and Firmino are on different subnets of it and talk over Tailscale instead (D-072) |
| **Ollama** | The program on each machine that runs models and serves them over HTTP |
| **Model name** | The exact Ollama name, typically `name:tag` |
| **Quantization** | Compressing a model (for example 4-bit) so it uses less memory |
| **tok/s** | Tokens per second. A measure of generation speed |
| **First-token delay** | Time before the first word appears. Long on CPU for big prompts |
| **Thinking model** | A model that writes out reasoning before its answer (for example the Qwen3 family). Slow on CPU, so not used by default. Its reasoning arrives as separate `thinking` events |
| **Tailscale Serve** | A Tailscale feature that publishes a local service to the tailnet only, with HTTPS. Used so the router can stay on localhost |
| **Context window** | How much text a model can consider at once. Larger uses more memory |
| **RAG** | Retrieval over documents. Out of scope here |
| **NDJSON** | Newline-delimited JSON. One JSON object per line. Used for streaming |
| **V1 to V7** | Release stages. All are built and running. See [ROADMAP.md](ROADMAP.md) |
