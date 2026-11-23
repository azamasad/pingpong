# Testing

Status: draft v1. How we know each version works. Acceptance criteria live in
[PRD.md](PRD.md). This document says how to check them.

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
| Pin model installed nowhere | `MODEL_NOT_INSTALLED`, after one registry refresh |
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
  connection dropped mid-stream, machine unreachable, model list changes.

### 2.4 Health and registry

- A machine is marked down after the configured number of failed checks, and
  back up when it recovers.
- A request-time connection failure marks it down immediately.
- Installed models update on refresh, and on a miss for a pinned model.

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
| 1 | From the MacBook Air, over Tailscale, send a request with no flags | Answered by Firmino's 14B model. The response says so |
| 2 | Send with `fast` | Answered by Mane's 8B model |
| 3 | Pin the 3B model | Answered by the 3B on Mane |
| 4 | Pin a model that is not installed | Clear error. No substitution |
| 5 | Send two requests to Firmino back to back | The second shows "waiting" and then completes |
| 6 | Switch Firmino off and send an auto request | Clear error |
| 7 | Close the browser during a long request | V1: work is stopped. Firmino stops generating |
| 8 | Watch traffic between Mane and Firmino | LAN addresses only |
| 9 | Check the router's memory on Mane | Within budget |
| 10 | Check that Firmino's inference port is unreachable from the MacBook Air directly | Blocked |

V2 additions: start a long task, close the laptop, return later and read the
result; switch Firmino off, confirm retries every 2 minutes and a failure
after 3 hours (or success on return); confirm results older than 90 days are
removed.

## 5. What not to test here

- Model quality. Choosing models is separate.
- RAG. Out of scope.
- Public exposure. It must not exist, so the check is that it does not.
