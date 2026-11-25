# Hardware and Models

Status: v3. Measured on the real machines (section 8). Section 5 keeps the
earlier community-benchmark expectations next to what was measured, so you can
see how far off they were. Re-measure after any hardware or model change, using
the scripts in `deploy/bench/` ([OPERATIONS.md](OPERATIONS.md) section 9).

## 1. Machines

### Mane

| Item | Value |
|---|---|
| Machine | Mac mini |
| Chip | Apple M4 |
| Memory | 16 GB unified (shared by CPU and GPU) |
| Storage | 256 GB SSD |
| Network | Ethernet. On a different subnet from Firmino, so traffic between them goes over Tailscale (D-072) |
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
| Memory | 40 GB DDR4-3200: 8 GB soldered to the board plus one 32 GB SODIMM |
| Storage | 512 GB NVMe. Ubuntu root about 178 GB (about 28% used at time of writing). Plus leftover Windows partitions (about 290 GB NTFS in total) |
| GPU | Integrated Radeon only. Not used for inference |
| Network | Gigabit Ethernet (`eno1`), on a different subnet from Mane. Reached from Mane over Tailscale. Addresses are kept out of this public repo |
| Role | Heavy worker. No router, no UI |

Notes:

- Inference runs on the CPU. Generation is limited by memory bandwidth, and
  reading long prompts is limited by the four cores.
- **Soldered 8 GB plus a 32 GB module.** The board has 8 GB soldered and one
  SODIMM slot, so memory runs in "flex" mode: roughly the first 16 GB is
  dual-channel and the rest is single-channel and slower. Where a model lands
  in memory can change speed from run to run. **There is no matched-module
  upgrade**: the 8 GB is soldered and cannot be replaced, so do not buy
  anything for this. The practical answer is a model that reads few bytes per
  token, which is why Firmino runs a mixture-of-experts model (D-073).
- It is a laptop. Lid, sleep, power and thermals need attention (see
  [SETUP-GUIDE.md](SETUP-GUIDE.md)).
- It has a pending reboot and package updates at the time of writing.

## 2. Starting models

Chosen for simplicity: **non-thinking models**. Thinking models emit a long
reasoning phase before the answer, which is painfully slow on Firmino's CPU.
They can still be used later by pinning (see D-038). Note that the plain
`qwen3:30b` tag is the thinking variant. The tag used here, `instruct-2507`,
is the non-thinking one.

| Machine | Model tag | Size on disk (approx.) | Purpose | Reached by |
|---|---|---|---|---|
| Mane | `llama3.1:8b` | 4.9 GB | Default for the `fast` flag | `fast` |
| Mane | `llama3.2:3b` | 2.0 GB | Simple, quick tasks | Pinning |
| Firmino | `qwen3:30b-a3b-instruct-2507-q4_K_M` | about 18 GB | Default for auto. Long-running work (D-073) | Auto |
| Firmino | `qwen2.5:14b` | 9 GB | Kept on Firmino by the owner (D-083). A slower dense comparison | Pinning |
| Firmino | `gpt-oss:20b` | about 14 GB | Candidate for a blind comparison, pin only (D-084). See section 2a | Pinning |

Notes:

- Tags are as listed in the Ollama library at the time of writing. Confirm each
  with `ollama pull`. If one is unavailable, pick the nearest equivalent and
  record it in [DECISIONS.md](DECISIONS.md).
- The owner may change any of these. They are configuration, not code
  ([CONFIGURATION.md](CONFIGURATION.md)).
- **Why a mixture-of-experts model on Firmino.** CPU generation speed is about
  memory bandwidth divided by the bytes read per token. A dense 14B reads about
  9 GB per token. The 30B mixture-of-experts model has 30B parameters in total
  but uses only about 3.3B per token, so it reads far less and is about five
  times faster than the dense 14B here, while being a bigger and better model.
  A dense 32B would be about 1 token/s and a 70B does not fit.
- Second candidate: `gpt-oss:20b`. See section 2a for whether it fits and how
  it should behave. Not yet tested here.
- Alternatives for Mane: keep to 8B or smaller.

## 2a. Can `gpt-oss:20b` sit alongside the 30B on Firmino?

The owner asked about adding "GMP 20S", read as `gpt-oss:20b`. This is an
assessment from the model's published size and design, **not a measurement**.
The comparison to settle it is in [OPERATIONS.md](OPERATIONS.md) section 13.

| Question | Answer |
|---|---|
| What is it | An open-weight reasoning model with about 21B parameters in total and about 3.6B active per token (mixture-of-experts, like the 30B). Weights in a 4-bit format, about 14 GB on disk |
| Disk | Fits easily. Models on Firmino would be about 18 GB (30B) + 9 GB (14B) + 14 GB (gpt-oss) = about 41 GB, against roughly 130 GB free on the 178 GB root partition |
| Memory, one at a time | Fits easily. 14 GB of weights plus a few GB of context is well under 40 GB |
| Memory, both loaded together | Would fit on paper (18 + 14 = 32 GB of 40 GB) but leaves only about 8 GB for Ubuntu, the context caches and the flex-mode slow region. **Do not do this.** Firmino stays at `OLLAMA_MAX_LOADED_MODELS=1`, so Ollama unloads one model when the other is requested |
| Cost of switching | Each switch pays the cold load time (25.9 s measured for the 30B, probably 15 to 25 s for gpt-oss, not measured). Fine for pinned occasional use, annoying if requests alternate constantly |
| Expected generation speed | Similar to the 30B, because both read about the same number of bytes per token (about 3.3B and 3.6B active at 4 bits). A reasonable guess is 8 to 14 tokens/s. Community reports say it is somewhat slower than Qwen3 on CPU. Not measured |
| The catch: it thinks first | It is a reasoning model. It writes a reasoning phase before the answer, counted in the same token budget. At 10 tokens/s, 300 to 1,000 thinking tokens is 30 to 100 seconds before the first visible word. So the time to a usable answer is much longer than the tokens/s figure suggests. Compare "median reply" time, not tokens/s |
| Reasoning effort | It has levels (low, medium, high). The router's `think` option is a true/false switch only (tech debt TD-2), so it runs at its default (medium) |
| Fit with the non-thinking rule (D-038) | It must be pinned. It would not become the default unless it clearly wins the comparison |
| Verdict | **Install it to compare. It fits.** Keep the 30B as the default, keep one model loaded, and decide after the blind comparison |

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
| 30B mixture-of-experts model (4-bit) | about 18 GB |
| Context cache at 8K | a few GB at most |
| Headroom | room for one more model on disk, but Firmino loads one model at a time (`OLLAMA_MAX_LOADED_MODELS=1`) |

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

## 5. Performance: expected and measured

Expectations came from community benchmarks published in 2026. They follow
the usual rule that generation speed is about memory bandwidth divided by the
bytes read per token. Measured numbers are from the real machines
(section 8).

| Machine | Model | Expected generation | Measured generation | Measured prompt reading |
|---|---|---|---|---|
| Mane (M4, 16 GB) | `llama3.1:8b` | about 21 tokens/s | **21.3 tokens/s** | 488 tokens/s warm |
| Firmino (Ryzen 3 5300U) | `qwen2.5:14b` (dense) | 2 to 4 tokens/s | **2.1 tokens/s** | 11 to 12 tokens/s |
| Firmino | `qwen3:30b-a3b-instruct-2507-q4_K_M` (mixture-of-experts) | not expected | **11.9 tokens/s** | 29 tokens/s short, 35 tokens/s on a long prompt |

What this means:

- The community figures for Mane and for the dense 14B were accurate. The
  dense 14B sat at the low end of the range, as expected for 4 cores and mixed
  memory.
- **The mixture-of-experts model is about 5.6 times faster than the dense 14B
  on the same machine**, and about 2.5 times faster at reading prompts. A
  500-token answer takes under a minute instead of four.
- Reading a long prompt is still the bigger wait on Firmino. A 1,700-token
  prompt took about 48 seconds in total on the 30B and about 133 seconds on the
  14B.
- With an 8K context limit, the worst case is about 8,000 tokens. At the
  measured 29 to 35 tokens/s reading speed that is about 4 to 5 minutes, far
  inside the 30-minute first-token timeout. The timeout could be lowered later.

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

## 8. Measurements

Measured through the router on the real machines, with `num_predict` 128 and
context 8192. Each figure is **one run, not an average**. Treat differences
under about 10 percent as noise. A dash means not measured.

| Machine | Model | Cold load | Prompt tokens/s | Generation tokens/s | Long prompt (about 1,700 tokens) total |
|---|---|---|---|---|---|
| Mane | `llama3.1:8b` | 4.4 s | 126 first run, 488 warm | 20.5 first run, 21.3 later | not measured |
| Mane | `llama3.2:3b` | not measured | not measured | not measured | not measured |
| Firmino | `qwen2.5:14b` | 12.6 s | 11.2 short, 12.4 long | 2.11 | 133 s |
| Firmino | `qwen3:30b-a3b-instruct-2507-q4_K_M` | 25.9 s | 28.7 short, 34.8 long | 11.88 | 48.5 s |

Other facts measured:

- Mane ran the 8B on 100 percent GPU with context 8192, and kept the model
  loaded for the 30-minute keep-alive.
- The router used about 24 MB of resident memory after 24 jobs, against the
  50 MB working budget.
- Output speeds for the long-prompt runs come from short outputs, so they are
  rougher than the short-prompt figures.
- Not measured yet: Firmino's temperature during a long generation, memory in
  use per model, and whether limiting Firmino to 4 threads helps.

To repeat or extend these, use `deploy/bench/quick.sh` for speed and
`deploy/bench/quality.py` for a blind quality and speed comparison, or the
Stress tab for a long soak. See [OPERATIONS.md](OPERATIONS.md) section 9.
Note that `pingpong doctor --chat` reports speeds from a 2-token reply and is
not a benchmark (D-077).

| Version | Load | VmRSS delta (CI test binary) | Owner on Mane |
|---|---|---|---|
| V7.1 | 500-item split batch submit (default `max_items`) | ~35 MB in CI test binary (`TestBatchMaxItemsRSSGate`; not the production router) | **Pending** (10 MB gate on real router) |
| V7.3 | 200-row template batch submit | ~5 MB in CI test binary (`TestTemplateBatchRSSGate`) | **Pending** (5 MB gate on real router) |
| V7.9 | none (notifications off by default) | no measurable delta in CI | n/a |

Each later V7.x pull request adds a row. The owner repeats on Mane.

## 9. Future machines

Possible additions such as a Mac Studio or DGX Spark would be registered by
role (see [ARCHITECTURE.md](ARCHITECTURE.md)) and described here when added.

## 10. Open questions

- Whether to reclaim the Windows partitions on Firmino.
- Whether `gpt-oss:20b` beats the current Firmino model on quality enough to
  justify its longer thinking time (comparison ready, OPERATIONS section 13).
