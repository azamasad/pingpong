# Testing

Status: v2. How we know each version works. Acceptance criteria live in
[PRD.md](PRD.md). This document says how to check them. V1 to V7 are covered by
automated tests and have been run on the real machines (section 4a). Tests for
the planned V7.1 to V7.9 are listed in section 2.10 and the owner's checks for
them in section 4b.

## 1. Approach

- The routing logic is a pure decision and should be tested heavily with
  tables of inputs and expected outputs.
- Everything that talks to Ollama is tested against a **fake Ollama server**
  that can be scripted to be slow, fail, or disappear. Real models are not
  needed for automated tests.
- Real machines are used only for the final manual acceptance run.
- Time-dependent behaviour (retries, retention) must be testable without
  waiting real time, so the code should take an injectable clock.

## 2. Automated tests

### 2.1 Routing decision (unit)

A table of cases derived from [ROUTING.md](ROUTING.md) section 8, including:

| Case | Expected |
|---|---|
| No flags | Heavy role machine, its default model, mode `auto` |
| `fast` | Fast role machine, its default model, mode `fast` |
| Pin model installed on one machine | That machine, mode `pinned_model` |
| Pin machine | That machine, default model, mode `pinned_machine` |
| Pin model and machine, installed there | That pair, mode `pinned_model_and_machine` |
| Pin model and machine, not installed there | `MODEL_NOT_INSTALLED` |
| Pin model installed on two machines, no machine named | `MODEL_AMBIGUOUS` |
| Pin unknown machine | `MACHINE_UNKNOWN` |
| Pin model installed nowhere, every machine's list known | `MODEL_NOT_INSTALLED`, after one registry refresh |
| Pin model not found, but a machine was never reached | `MODELS_UNKNOWN` (retryable) |
| Pin model last seen on a machine that is now down | That machine, then `MACHINE_DOWN` |
| Default model not installed, machine up | `MODEL_NOT_INSTALLED` naming the model and machine |
| Default model not in a stale list, machine down | `MACHINE_DOWN` (never `MODEL_NOT_INSTALLED`) |
| Model name without a tag | Matches `:latest` |
| `fast` plus a pin | The pin wins |
| Target machine marked down | `MACHINE_DOWN` |
| No machine with the needed role | `NO_TARGET` |
| Model omitted its tag | Treated as the default tag, as Ollama does |

### 2.2 Concurrency gate (unit)

- With `max_concurrent = 1`, a second request waits, then runs when the first
  ends.
- Waiting is first-come, first-served.
- A waiting request whose client disconnects is removed without running.
- A running request whose client disconnects cancels the upstream call.
- Machines do not block each other.

### 2.3 API (integration with fake Ollama)

- Streaming order: `route`, `status`, `token` events, `done`.
- Errors before streaming return the right HTTP status and code.
- Errors after streaming starts arrive as an in-band `error` event.
- `stream:false` returns one object.
- `Host` and `Origin` checks reject unexpected values.
- Oversized bodies are rejected.
- Fake Ollama scenarios: slow first token, long gaps between tokens (timeouts),
  connection dropped mid-stream, an `error` line mid-stream, machine unreachable,
  HTTP 503 when overloaded, model list changes, and a reasoning model whose
  stream includes `thinking` text (it must arrive as `thinking` events, never
  inside `token` text).
- Statistics: nanosecond durations from Ollama are converted correctly into
  milliseconds and tokens per second.

- Heartbeat: while a stream is quiet (waiting, loading, reading a prompt), `ping`
  events arrive at the configured interval, and a failed write drops the request
  and frees the machine.
- Context warning: a final statistic showing the prompt within about 5 percent of
  `num_ctx` sets `context_warning`. A normal prompt does not.
- The router always sends the configured `num_ctx` and ignores a client-supplied one.
- `stream:false` returns the single object described in API.md, and errors use
  HTTP status codes.
- Error mapping: each row of the mapping table in API.md section 3.2 is exercised
  with the fake Ollama, and each code in section 3.1 returns its listed HTTP
  status (including 403 for Host and Origin, 413, 404, and 502 or 504 for
  `stream:false` upstream failures).
- Response commitment: failures in validation, routing or the down check return
  an HTTP error with nothing streamed. After the `route` event, every failure is
  an in-band `error`. `route` arrives before any waiting.
- Status events: no `waiting` when the machine is free; `waiting` with a 1-based
  `position`, re-sent when it changes; exactly one `running` before the first chunk.
- Request validation: empty `messages`, a bad role, a last message that is not
  from the user, both `prompt` and `messages`, and unknown `options` keys (ignored).
- Host and Origin: the full tailnet name, `localhost` and `127.0.0.1` pass, ignoring
  case and port; `Origin: null` and other hosts are rejected; a missing `Origin` passes.
- Timeouts: no first chunk (a `thinking` chunk counts), an idle gap, and the
  total run-time cap each produce `TIMEOUT` and cancel upstream. Also with `stream:false`.
- Context warning: also set when `prompt + output` reaches `num_ctx` or the done
  reason is `length`. Missing or zero `prompt_tokens` does not crash or set it by itself.
- A machine's default model that is not installed returns `MODEL_NOT_INSTALLED`
  naming the default model, before any call to Ollama.

### 2.4 Health and registry

- A machine is marked down after the configured number of failed checks, and
  back up when it recovers.
- A request-time connection failure marks it down immediately.
- Installed models update on refresh, and on a miss for a pinned model.
- A machine with a request in flight is not marked down by failed health checks,
  and marking it down never aborts the in-flight request.
- In V1, requests waiting for a machine that goes down fail with an in-band
  `MACHINE_DOWN`.
- The last known model list is kept while a machine is down. A machine never
  reached since start gives `MODELS_UNKNOWN` for a pinned model that cannot be found.
- Real Ollama contract check (where the build environment allows): run the same
  stream and model-list tests against a real Ollama with a tiny model, so the
  fake cannot drift from reality.

### 2.5 V2 tests

- A request keeps running after the client disconnects.
- Output is saved, and `events?from=` replays correctly.
- Retry timing, using a fake clock: every 2 minutes, stop after 3 hours from
  the first failure, window not reset by later failures.
- Mid-task failure restarts from the beginning and the earlier partial output
  is kept as history.
- Router restart recovery: `waiting` and `retrying` resume, `running` becomes
  `retrying`.
- Cancel works from each active state.
- Retention removes results older than 90 days and leaves newer ones.

## 3. Resource tests (Mane)

| Check | How | Pass |
|---|---|---|
| Idle memory | Measure the router process after start | Well under the 50 MB working budget |
| Memory under load | Run several streaming requests | Stays under the budget |
| Page weight | Measure the served page | Small, with no external assets |
| V2 store growth | Save many requests and check disk use | Roughly proportional to text stored |

## 4. Manual acceptance run (real machines)

Done by the owner after setup. Check off against the PRD acceptance criteria.

| # | Step | Expected |
|---|---|---|
| 1 | From the MacBook Air, over Tailscale, send a request with no flags | Answered by Firmino's default model (the 30B mixture-of-experts model). The response says so |
| 2 | Send with `fast` | Answered by Mane's 8B model |
| 3 | Pin the 3B model | Answered by the 3B on Mane |
| 4 | Pin a model that is not installed | `MODEL_NOT_INSTALLED`. No substitution |
| 5 | Send two requests to Firmino back to back | The second shows "waiting" and then completes |
| 6 | Switch Firmino off and send an auto request | Clear error |
| 7 | Close the browser during a long request | V1: work is stopped. Firmino stops generating |
| 8 | While a request runs, check connections: `lsof -i -nP` on Mane (filter on Firmino's Tailscale address) and `ss -tnp` on Firmino | Only the Tailscale path between Mane and Firmino, no public addresses (D-072) |
| 9 | Check the router's memory on Mane | Within budget |
| 10 | After the ACL is applied, from the MacBook Air try port 11434 on Firmino's tailnet address | Refused or times out |
| 11 | Send a prompt of about 6,000 tokens | `done` reports `prompt_tokens` above 4096 and no context warning (the window is set explicitly). Then send one near 8,192 and confirm the warning appears |
| 12 | Reboot each machine, and cut power to Mane briefly | Both come back and answer without help |
| 13 | Open the router address from a device not on your tailnet | It does not load |
| 14 | Send a request to Firmino with a long prompt and watch the page from the MacBook Air | Tokens appear gradually, not all at once (Tailscale Serve is not buffering). Elapsed time keeps ticking while it waits, and the connection survives several quiet minutes |
| 15 | Close the browser tab during a long prompt on Firmino, then check Firmino's CPU | CPU use falls once the request is cancelled. If it does not fall quickly, Tailscale Serve is hiding the disconnect: note it, rely on the run-time cap, and report it |
| 17 | Press the page's Stop button during a long request | Firmino's CPU falls at once (this path does not depend on disconnect detection) |
| 18 | After a reboot, with the router started by its launch agent, send a request to Firmino | It works (macOS Local Network permission is not blocking it, and Ollama.app is not holding port 11434) |
| 16 | After a reboot of Mane, check that Tailscale Serve is still publishing the router | The page loads without any manual step |

V2 additions: start a long task, close the laptop, return later and read the
result; switch Firmino off, confirm retries every 2 minutes and a failure
after 3 hours (or success on return); confirm results older than 90 days are
removed.

### 2.6 V3 to V5 tests (built)

- **Lanes (V3):** an interactive job is admitted before a waiting batch job on
  the same machine, and FIFO holds within a lane (`TestWaitTurnPrefersInteractive`).
  A running job is never preempted.
- **Batches (V3):** `POST /api/batches` then `GET /api/batches/{id}` returns the
  jobs and a `pending` count. The `pingpong batch` commands are covered by the
  API tests.
- **Role choice (V4):** least in-flight machine wins among those that are up,
  then shortest wait, then file order. A down machine is skipped. All down
  gives `MACHINE_DOWN` (`TestRolePicksLeastLoadedUpMachine` and the cases next to it).
- **Profiles (V5):** `fast` routes to the fast role, an explicit flag, model or
  machine beats the preset, an unknown profile is `BAD_REQUEST`, and
  `GET /api/profiles` lists them.
- **Config:** default profiles exist, `refresh_on_model_miss` defaults to on
  when omitted, and a leading `~/` in `data_dir` is expanded.
- **Registry:** a machine that was down at start loads its model list at the
  first health check after it comes up, not at the next 5-minute refresh.
- **Error shape:** a pinned model that is not installed returns a JSON
  `MODEL_NOT_INSTALLED` with status 404 (not an empty error, not a generic
  `NOT_FOUND`).

The real binary is also smoke-run against two stand-in Ollama servers (health,
auto, fast, profile, pin, jobs, batch CLI, host and origin guards, streaming
timing, memory). That run is manual today. V6's `doctor` replaces most of it.

### 2.7 V6 tests (built)

- `internal/doctor`: unknown machine, unreachable machine, missing default
  model, storage warn when jobs off, writable `data_dir`, disk warn and fail
  thresholds, exit codes, and the `--chat` check (including its 2-token sample).
- `internal/ollama`: `Version` against a fake server.
- Graceful shutdown: running jobs are stopped on purpose, marked retrying with
  `ROUTER_RESTARTING`, and counted toward the retry window.
- `internal/store`: `pingpong backup` produces a consistent copy.
- `GET /api/status` returns counts, machines and memory, and never content.

See [V6-OPERABILITY.md](V6-OPERABILITY.md) section 10.

### 2.8 V7 tests (built)

Aliases: config validation, resolution order after profiles and before routing,
explicit `machine` wins over the alias's machine, `GET /api/aliases`, and the
doctor check. Listed in [V7-ALIASES.md](V7-ALIASES.md) section 8.

### 2.9 Test tooling tests

- Stress run: a 2-second run against a fake Ollama finishes at least one job on
  Mane, then `DELETE /api/stress` stops it and cancels its jobs.
- Per-request log: the log whitelist keeps speed and outcome fields and drops
  anything else, so prompts and outputs cannot leak (`internal/logx`).
- A finished job's `done` event carries speed `stats`.

### 2.10 Tests for V7.1 to V7.9 (planned, per version)

Each version's full acceptance list is in
[NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md) (the section named
below). Write the tests from that list. Layers, for every version:

| Layer | What it covers | How |
|---|---|---|
| Unit | Pure logic: split assignment (2), template substitution (4), chunking (7), CSV and Markdown writers (5), next-due maths (8), context estimates (3, 6), history building (6) | Tables of inputs and outputs, injectable clock |
| Fake Ollama | Which pair ran each job, what prompt or `messages` the model received, down machine while the other finishes, failure and retry windows | Upgraded fake with recorded calls and per-call replies (spec 1.9). Needed from V7.1 on |
| API integration | Every endpoint, every validation error and HTTP status, atomic submit (nothing saved on any error), Host and Origin on new endpoints, `/api/chat` rejects `check`, `template`, `split` | `httptest` with real store in a temp dir |
| Store | Migration from a database made by `main` (old jobs and batches still load), new tables in `pingpong backup` output, retention sweep rules (child with parent, thread jobs with thread, templates and schedules kept) | Fake clock |
| Restart | Stop and start with work in each in-between state: split batch half done, check stage 1 done with no child, thread turn running, reduce pending, schedule due while stopped | Reopen the store and service |
| Log | No prompt, output, template, var, file text or webhook URL in any log line during the smoke scenario. New fields only through the whitelist | Capture log output and assert |
| Memory | The load named in each spec section, read from `VmRSS` on Linux (as `memory_linux_test.go`) | Recorded in HARDWARE-AND-MODELS section 8 |
| Smoke | Real binary, two stand-in Ollama servers, `curl` commands from the spec section | Manual, results in the pull request |

Cross-version cases to cover once all are built: a checked, split batch from a
template exports to CSV with every stage and pair; a scheduled template batch
with `split` runs and notifies (V7.9); a thread survives a router restart; a
2 MB file with reduce stays under the memory gate.

### Fake Ollama scenario format

The fake Ollama is a small test server. Each test gives it a scenario: a list of
steps played back in order for one call. Steps are: wait for a duration, send a
chunk (with fields as real Ollama would: content or `thinking`, then a final
chunk with the statistics), send an `error` line, close the connection, or return
an HTTP status. The scenario can also set the model list returned by the tags
call. Keep it a plain data file, one per test. Exact error lines Ollama sends
mid-stream are still not captured from a real failure. The normal stream shape
has been verified by `doctor --chat`.

## 4a. Real-machine results so far

| Check | Result |
|---|---|
| `pingpong doctor --chat` | Passes for both machines (real Ollama stream verified, D-052 closed) |
| Router memory | About 24 MB resident after 24 jobs |
| Jobs | 24 jobs: 23 done, 1 cancelled, 0 failures |
| Mane speed | 21.3 tokens/s generation, 488 tokens/s prompt reading (warm) |
| Firmino speed | 11.9 tokens/s generation on the 30B mixture-of-experts model, 35 tokens/s prompt reading |
| Page reachable only on the tailnet | `tailscale serve status` says tailnet only, and the owner confirmed from a device off the tailnet that it does not load (D-080) |
| ACL blocking Firmino's port 11434 | **Not yet applied.** Ready to apply: [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md) section 4 (D-081) |
| Soak test with the Stress tab | **Not yet done.** Runbook: [OPERATIONS.md](OPERATIONS.md) section 11 |
| Reboot and power-cut checks (steps 12 and 16) | **Not yet done.** Runbook: [OPERATIONS.md](OPERATIONS.md) section 12 |
| `gpt-oss:20b` blind comparison | **Not yet done.** Runbook: [OPERATIONS.md](OPERATIONS.md) section 13 |

Speed figures are single runs. See [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md)
section 8 and [OPERATIONS.md](OPERATIONS.md) section 9 to repeat them.

## 4b. Owner checks for V7.1 to V7.9 (when each is built)

Run after the version is merged and installed. The agent cannot do these.
Details per version: the "Owner on real machines" line in
[NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md).

| Version | Check |
|---|---|
| V7.1 | A 10-item split with the 8B on Mane and the 30B on Firmino: both bars move, each result shows its own machine and model. Router RSS on Mane with the batch queued |
| V7.2 | One review and one revise with real models. Is the fixed review wording useful? Time for the model swap on Firmino (one model loaded at a time) |
| V7.3 | A real template run as a 5-row batch |
| V7.4 | Download the CSV of a real batch and open it in your spreadsheet. Accented text, and a cell starting with `=` |
| V7.5 | A 10-turn conversation with the 30B, close the laptop mid-answer, reopen on the phone. Time to start turn 10 |
| V7.6 | Upload a real report of about 100 KB, with and without reduce. Say what chunk size suits. Router RSS during the upload |
| V7.7 | One nightly schedule for a week: one due-time run, one after a Mane restart, one run now. Check Mane's time zone. Daylight-saving change if one falls in the week |
| V7.8 | Point one OpenAI-compatible client at the router and chat |
| V7.9 | Finish one small batch and check the phone message has no content |
| All | `pingpong backup` after each, `pingpong doctor --chat` still passes, router RSS under 50 MB, log lines contain no content (`logsummary.py`) |

## 5. What not to test here

- Model quality in automated tests. Compare models by hand with `deploy/bench/quality.py` (it is a tool, not a test).
- RAG. Out of scope.
- Public exposure. It must not exist, so the check is that it does not.
