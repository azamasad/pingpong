# Hardware and Models

Status: draft v2. Speed figures come from third-party community benchmarks on
similar hardware (sources in section 5). They are expectations, not
measurements of your machines. Measure after setup (section 8) and update
this document.

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

- Ollama only lets the GPU use part of unified memory. Reports for a 16 GB M4
  put the limit at roughly 11.3 to 11.8 GiB. If loaded models plus their
  context cache exceed it, layers move to the CPU and speed drops sharply.
  This is the practical reason for the 8B cap.
- 256 GB fills quickly with models. Keep only a few on Mane.

### Firmino

| Item | Value |
|---|---|
| Machine | Lenovo ThinkBook 15 G3 ACL (model 21A4) |
| OS | Ubuntu 24.04 LTS |
| CPU | AMD Ryzen 3 5300U, 4 cores / 8 threads |
| Memory | 40 GB DDR4-3200 (one 32 GB module plus one 8 GB module) |
| Storage | 512 GB NVMe. Ubuntu root about 178 GB (about 28% used at time of writing). Plus leftover Windows partitions (about 290 GB NTFS in total) |
| GPU | Integrated Radeon only. Not used for inference |
| Network | Gigabit Ethernet (`eno1`). LAN address is kept out of this public repo |
| Role | Heavy worker. No router, no UI |

Notes:

- Inference runs on the CPU. Generation is limited by memory bandwidth, and
  reading long prompts is limited by the four cores.
- **Mismatched memory modules.** With a 32 GB and an 8 GB module, only the
  first part of memory can run in dual-channel mode. The rest is likely
  single-channel and slower. Where the model lands in memory can change speed
  from run to run. Measure first. If speed is disappointing, matched modules
  are a possible hardware upgrade, and community figures suggest roughly a
  1.6 to 1.9 times difference between single and dual channel.
- It is a laptop. Lid, sleep, power and thermals need attention (see
  [SETUP-GUIDE.md](SETUP-GUIDE.md)).
- It has a pending reboot and package updates at the time of writing.

## 2. Starting models

Chosen for simplicity: **non-thinking models**, all with widely used Ollama
tags and published benchmarks. Thinking models (such as the Qwen3 family) emit
a long reasoning phase before the answer, which is painfully slow on
Firmino's CPU. They can still be used later by pinning (see D-038).

| Machine | Model tag | Size on disk (approx.) | Purpose | Reached by |
|---|---|---|---|---|
| Mane | `llama3.1:8b` | 4.9 GB | Default for the `fast` flag | `fast` |
| Mane | `llama3.2:3b` | 2.0 GB | Simple, quick tasks | Pinning |
| Firmino | `qwen2.5:14b` | 9 GB | Default for auto. Long-running work | Auto |

Notes:

- Tags are as listed in the Ollama library at the time of writing. Confirm each
  with `ollama pull`. If one is unavailable, pick the nearest equivalent and
  record it in [DECISIONS.md](DECISIONS.md).
- The owner may change any of these. They are configuration, not code
  ([CONFIGURATION.md](CONFIGURATION.md)).
- Alternatives worth trying later: `phi4` (14B, non-thinking) on Firmino,
  `qwen3:8b` or `qwen3:14b` (thinking, pin only), and the newer Gemma 4 models.
  Mind the 8B cap on Mane.

## 3. Memory budgets

### Mane (16 GB)

| Item | Approx. |
|---|---|
| macOS and system | 4 to 5 GB |
| Router and web page | 50 MB (working budget) |
| 8B model loaded (as `ollama ps` reports) | about 5.6 GB |
| 3B model loaded | about 2.5 GB |
| Both loaded together | about 8.1 GB, under the GPU limit |
| Context cache | Grows with context length. About 1 GiB at 8K tokens for an 8B model |

Rule: **nothing larger than 8B on Mane**, and the models loaded together plus
context must stay under the GPU limit (about 11.3 GiB), so they stay on the GPU.

### Firmino (40 GB)

| Item | Approx. |
|---|---|
| Ubuntu and services | 1 to 2 GB |
| 14B model (4-bit) | about 9 GB |
| Headroom | plenty for context, a second model, or a larger model later |

## 4. Context window

Ollama's default context window is **4096 tokens**. Longer prompts are cut
silently, which looks like the model forgetting your text. So the context
window is set explicitly on both machines, to **8192 tokens** to start
(D-042).

| Item | Guidance |
|---|---|
| Setting | `OLLAMA_CONTEXT_LENGTH` on each machine, and optionally `num_ctx` per machine in [CONFIGURATION.md](CONFIGURATION.md) |
| Mane | Keep modest. Context cache uses unified memory |
| Firmino | Has memory to go higher, but long prompts are slow to process |
| Conversations | The whole history is resent each turn, so long chats grow slower and eventually hit the limit |
| Timeouts | The first-token timeout must exceed the time to read the largest allowed prompt (see section 5) |

## 5. Performance expectations

Sources are community benchmarks published in 2026, not first-party tests. They
are consistent with the usual rule that generation speed is about memory
bandwidth divided by model size.

| Machine | Model | Generation | Prompt processing | Basis |
|---|---|---|---|---|
| Mane (M4, 16 GB) | 8B, 4-bit | about 21 tokens/s | hundreds to about 1,000 tokens/s | Reported for Llama 3.1 8B with Ollama and llama.cpp |
| Mane | 3B, 4-bit | about 45 tokens/s | fast | Reported for Llama 3.2 3B |
| Firmino (Ryzen 3 5300U) | 14B, 4-bit | about 2 to 4 tokens/s | slow, perhaps 5 to 15 tokens/s | Reported 3 to 5 tokens/s for a 14B on a faster 8-core desktop CPU with dual-channel memory. Lower expected here: 4 cores, a low-power laptop chip, mismatched memory |

What this means:

- **Generation on Firmino is slow but workable**, as you expected. A 500-token
  answer takes a few minutes.
- **Reading a long prompt on Firmino is the bigger wait.** A prompt of several
  thousand tokens can take many minutes before the first word appears. This
  is why the first-token timeout is generous and why interactive long-context
  work is better on Mane.
- With an 8K context limit, the worst case is about 8,000 tokens. At 8 tokens/s
  that is about 17 minutes, and at 5 tokens/s about 27 minutes, inside the
  30-minute first-token timeout (the default). If you measure slower than
  5 tokens/s, raise the timeout or lower the context limit.

## 6. Disk plan

| Machine | Plan |
|---|---|
| Mane | Free up space. Keep only the 8B and the 3B, plus room for the router's data (V2) |
| Firmino | Free up space. Act as the larger model library. Decide whether to reclaim the unused Windows partitions (confirm nothing is needed first) |

Saved results (V2) are text and small. 90 days of normal use should be a
minor share of disk.

## 7. Model storage locations

| Machine | Default location |
|---|---|
| macOS | `~/.ollama/models` |
| Linux | `/usr/share/ollama/.ollama/models` (on the root partition) |

Another location can be set with `OLLAMA_MODELS` if needed.

## 8. Measurements (to fill in after setup)

Follow [SETUP-GUIDE.md](SETUP-GUIDE.md) section 5, then replace the
expectations in section 5 with real numbers.

| Machine | Model | Cold load time | First-token delay (short prompt) | First-token delay (8K prompt) | Prompt tokens/s | Generation tokens/s | Memory in use |
|---|---|---|---|---|---|---|---|
| Mane | `llama3.1:8b` | | | | | | |
| Mane | `llama3.2:3b` | | | | | | |
| Firmino | `qwen2.5:14b` | | | | | | |

Also record:

- The router's memory use on Mane (idle and under load) against the 50 MB
  working budget.
- Firmino's temperature during a long generation.
- Whether limiting Firmino to 4 threads helps.

## 9. Future machines

Possible additions such as a Mac Studio or DGX Spark would be registered by
role (see [ARCHITECTURE.md](ARCHITECTURE.md)) and described here when added.

## 10. Open questions

- Real measured speeds (section 8).
- Whether to reclaim the Windows partitions on Firmino.
- Whether matched memory modules in Firmino are worth buying, after measuring.
