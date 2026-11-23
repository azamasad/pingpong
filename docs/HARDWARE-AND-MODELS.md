# Hardware and Models

Status: draft v1. Speed and size figures are rough estimates for planning.
Measure after setup and update this document.

## 1. Machines

### Mane

| Item | Value |
|---|---|
| Machine | Mac mini |
| Chip | Apple M4 |
| Memory | 16 GB unified (shared by CPU and GPU) |
| Storage | 256 GB SSD |
| Network | Ethernet, same LAN as Firmino |
| Inference | Apple GPU (Metal), fast |
| Role | Brain, tiny web page, fast worker (models up to 8B) |

Notes:

- 16 GB is shared with macOS, the router and the model. This is why Mane is
  capped at 8B.
- 256 GB fills quickly with models. Keep only a few on Mane.

### Firmino

| Item | Value |
|---|---|
| Machine | Lenovo ThinkBook 15 G3 ACL (model 21A4) |
| OS | Ubuntu 24.04 LTS |
| CPU | AMD Ryzen 3 5300U, 4 cores / 8 threads |
| Memory | 40 GB DDR4-3200 (one 32 GB module plus one 8 GB module) |
| Storage | 512 GB NVMe. Ubuntu root about 178 GB (about 28% used at time of writing). Plus leftover Windows partitions (about 290 GB NTFS in total) |
| GPU | Integrated Radeon only. Not used for inference. |
| Network | Gigabit Ethernet (`eno1`). LAN address is kept out of this public repo |
| Role | Heavy worker. No router, no UI |

Notes:

- Inference runs on the CPU, so it is slow, and long prompts are especially
  slow to start.
- Mixed memory modules likely give lower memory bandwidth than a matched pair.
  Treat speed estimates cautiously.
- It is a laptop. Lid, sleep, power and thermals need attention (see
  [OPERATIONS.md](OPERATIONS.md)).
- It has a pending reboot and package updates at the time of writing.

## 2. Memory budgets

### Mane (16 GB)

| Item | Approx. |
|---|---|
| macOS and system | 4 to 5 GB |
| Router and web page | 50 MB (working budget) |
| 8B model (4-bit) | about 5 GB |
| 3B model (4-bit) | about 2 GB |
| Context and headroom | the rest |

An 8B and a 3B can be loaded together with room to spare. A 14B (about 9 GB)
would fit only tightly, so the rule is: **nothing larger than 8B on Mane**.

### Firmino (40 GB)

| Item | Approx. |
|---|---|
| Ubuntu and services | 1 to 2 GB |
| 14B model (4-bit) | about 9 GB |
| Headroom | plenty for context, a second model, or a larger model later |

## 3. Starting models

Specific model names are chosen at setup. This table records the intent.

| Machine | Size | Purpose | Reached by |
|---|---|---|---|
| Mane | 8B | Default for the `fast` flag | `fast` |
| Mane | 3B | Simple, quick tasks | Pinning |
| Firmino | 14B | Default for auto. Long-running work | Auto |
| Firmino | Larger, later | Only if needed | Pinning, or by changing the default |

## 4. Rough performance expectations

These are guesses to calibrate expectations, not measurements.

| Machine | Model | Output speed (estimate) |
|---|---|---|
| Mane | 8B | Roughly 20 to 30 tokens per second |
| Mane | 3B | Faster than the 8B |
| Firmino | 14B | Roughly 2 to 4 tokens per second, with slow start on long prompts |

The owner accepts slow responses on Firmino. Speed is not the priority.

## 5. Disk plan

| Machine | Plan |
|---|---|
| Mane | Free up space. Keep only the 3B and 8B, plus room for the router's data (V2). |
| Firmino | Free up space. Act as the larger model library. Decide whether to reclaim the unused Windows partitions (confirm nothing is needed first). |

Saved results (V2) are text and small. 90 days of normal use should be a
minor share of disk.

## 6. Context window

The context window is how much text a model considers at once. Larger windows
use more memory and make long prompts slower, especially on Firmino's CPU.

| Item | Guidance |
|---|---|
| Starting point | A modest window (for example 4,096 to 8,192 tokens) on both machines. Raise it after measuring |
| Setting | An optional `num_ctx` per machine in [CONFIGURATION.md](CONFIGURATION.md). Unset means Ollama's default |
| Mane | Keep it modest. Memory is tight |
| Firmino | More room for larger windows, but processing long prompts is slow |
| Conversations | The whole history is resent on each turn, so long chats grow slower |

## 7. Measurements (to fill in after setup)

Replace the estimates in section 4 with real numbers.

| Machine | Model | Cold load time | First-token delay (short prompt) | First-token delay (long prompt) | Tokens per second | Memory in use |
|---|---|---|---|---|---|---|
| Mane | 8B | | | | | |
| Mane | 3B | | | | | |
| Firmino | 14B | | | | | |

Also record the router's memory use on Mane (idle and under load) against the
50 MB working budget.

## 8. Future machines

Possible additions such as a Mac Studio or DGX Spark would be registered by
role (see [ARCHITECTURE.md](ARCHITECTURE.md)) and described here when added.

## 9. Open questions

- Exact models and quantization for each size.
- Measured speeds on both machines.
- Whether to reclaim the Windows partitions on Firmino.
