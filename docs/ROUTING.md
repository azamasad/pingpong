# Routing

Status: draft v3.

Routing is done by Mane's router, using simple rules and clear signals only.
It never reads a prompt to guess difficulty (see constitution P9).

Related: [API.md](API.md) for the request format and errors,
[CONFIGURATION.md](CONFIGURATION.md) for defaults, [TESTING.md](TESTING.md)
for the test cases.

## 1. Inputs to a routing decision

| Input | Source |
|---|---|
| `fast` flag | Client |
| Pinned model, pinned machine | Client |
| Machines and roles | Registry (from configuration) |
| Models installed on each machine | Registry (read from each Ollama, last known list kept) |
| Whether each machine is marked down | Health checks and request outcomes |
| Whether each machine is at its concurrency limit | Router's own count of requests in flight |

## 2. Modes

| Mode | How to select | Where it goes |
|---|---|---|
| **Auto** | No flags | The machine with the `heavy` role (Firmino), its default model (`qwen2.5:14b`) |
| **Fast** | `fast` flag | The machine with the `fast` role (Mane), its default model (`llama3.1:8b`) |
| **Pinned model** | Name a model | The machine that has that model |
| **Pinned machine** | Name a machine | That machine, with its default model |
| **Pinned model and machine** | Name both | Exactly that pair |

Profiles (named presets such as "fast / balanced / deep") are deferred.

If two machines share a role (not expected until V4), V1 uses the first one in
the configuration file and logs a warning. Choosing between them is V4 work.

## 3. Decision order (exact)

This happens before any response header is written (see
[API.md](API.md) section 3). The first rule that fails ends routing with that
error.

1. **Pick the target machine and model.**
   - If a machine is named and it is not in the registry: `MACHINE_UNKNOWN`.
   - Machine named, model not named: that machine, with its default model.
   - Model named, machine not named: look at every machine's **last known**
     model list, including machines currently down.
     - Exactly one machine has it: that machine.
     - Several machines have it: `MODEL_AMBIGUOUS`. Do not guess.
     - None has it, and at least one machine has **never been reached** since
       the router started (so its models are unknown): `MODELS_UNKNOWN`
       (retryable).
     - None has it, and every machine's list is known: refresh the registry
       once from the reachable machines and look again. Still none:
       `MODEL_NOT_INSTALLED`.
   - Both named: the pair, as given.
   - Neither named, `fast` set: the `fast` role machine and its default model.
   - Neither named, no `fast`: the `heavy` role machine and its default model.
   - If the needed role has no machine: `NO_TARGET`.
2. **Down check.** If the chosen machine is marked down: `MACHINE_DOWN`. This
   comes before any model check, so a machine that is off always reports as
   down, never as "model not installed". (In V2 the request is accepted and
   retried instead, see [JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md).)
3. **Model check.** The machine is up, so its model list is known. If the chosen
   model is not in it (for a pinned model, after one registry refresh;
   for a default model, immediately): `MODEL_NOT_INSTALLED`. For a default
   model the message names the model and the machine.

Pins override `fast`: if a model or machine is named, `fast` is ignored.
The choice is made once, at the start. It is not changed mid-task.

## 4. Model names

- Models are matched by their exact Ollama name, usually `name:tag`.
- If a name is given without a tag, treat it as `:latest`, the way Ollama
  does, so it matches the installed model.
- Matching is case-sensitive.
- Responses report the **installed name** (for example `llama3.1:latest` if that
  is how the registry lists it), not what the client typed.
- Short aliases are deferred.

## 5. Busy, waiting, down and unknown

| State of the target | Behaviour |
|---|---|
| Below its concurrency limit | Run now |
| At its concurrency limit | Wait for it, first come first served. Do not reroute |
| Marked down at routing time | `MACHINE_DOWN` (V1: immediate error) |
| Goes down while requests are waiting for it | V1: fail each waiting request with an in-band `MACHINE_DOWN` error. V2: they stay waiting or retrying |
| Model installed but not loaded | Allowed. It loads on demand, so the first answer is slower |

"Wait, don't reroute" holds for auto, fast and pinned requests alike: a request
meant for Firmino waits for Firmino.

### What "busy" means

- Each machine has a concurrency limit (default **1**, configurable).
- A machine is **busy** only when the router has that many requests in flight on it.
  `GET /api/machines` reports this same definition.
- Requests beyond the limit wait in a first-come first-served list per machine.
- The router can only count requests that pass through it. Anything used on
  Mane directly (for example the owner using Ollama at the Mac itself) is not
  counted, which is accepted.
- Machines do not block each other.

### Health

- Each machine is checked on a schedule (default 30 seconds) with a generous
  timeout (default 10 seconds).
- A machine is marked down after a few consecutive failed checks (default 2),
  or immediately if a request cannot connect.
- It is marked up again as soon as a check succeeds.
- **A machine with a request in flight is treated as up.** A CPU-bound Firmino
  can answer health checks slowly or not at all while it reads a long prompt,
  with no output for many minutes. Failed health checks never mark a machine
  down while it has a request in flight, and marking a machine down never aborts
  a request in flight. In-flight requests end only through a connection error,
  a timeout, or the client leaving.
- The registry keeps the **last known list of installed models** for each
  machine, even while it is down, so pinned requests can still be validated.
  A machine never reached since the router started has an unknown list, which
  produces `MODELS_UNKNOWN` in the case described in section 3.

### Timeouts

CPU inference can take many minutes before the first output, so limits are
generous (defaults in [CONFIGURATION.md](CONFIGURATION.md)):

- Connecting to a machine: short.
- First chunk from Ollama (any chunk, whether answer text or reasoning text):
  long, covering model loading and reading a long prompt.
- Gap between chunks after that: moderate.
- Total running time of one request: capped.
- Waiting for a busy machine: no limit in V1. The client can cancel.

If a limit is hit, the request ends with a `TIMEOUT` error and the upstream call
is cancelled.

## 6. Conversations

- In V1 the router keeps no conversation. The client sends the whole message
  history with each request, and the page holds it.
- The page keeps the flag and pin settings between messages, so a conversation
  stays on the same target unless the owner changes it.
- If the owner changes the target mid-conversation, the new machine gets the
  whole history and processes it from scratch. That is normal, and slower on
  long conversations, especially on Firmino.

## 7. Transparency

Each response includes:

- the model used
- the machine used
- how the target was chosen (`auto`, `fast`, or a pinned mode)

## 8. Examples

| Request | Result |
|---|---|
| No flags | Firmino, `qwen2.5:14b`, `auto` |
| `fast` | Mane, `llama3.1:8b`, `fast` |
| Pin model `llama3.2:3b` | Mane, that model, `pinned_model` |
| Pin machine `firmino` | Firmino, default model, `pinned_machine` |
| Pin machine `firmino` and model `qwen2.5:14b` | Firmino, that model, `pinned_model_and_machine` |
| `fast` and pin machine `firmino` | Firmino. The pin wins |
| Pin model `70b-example` (installed nowhere, all lists known) | Error `MODEL_NOT_INSTALLED` |
| Pin a model that only Firmino could have, but Firmino has never been reached | Error `MODELS_UNKNOWN` (retryable) |
| Pin a model installed on two machines, no machine named | Error `MODEL_AMBIGUOUS` |
| Auto with Firmino switched off | Error `MACHINE_DOWN` (never `MODEL_NOT_INSTALLED`) |
| Auto while Firmino is busy | Waits for Firmino |
| Auto while Firmino is down | Error (V1). Retries (V2) |
| `fast` while Mane is busy | Waits for Mane |
| Auto when Firmino's default model was uninstalled | Error `MODEL_NOT_INSTALLED`, naming the model and machine |

## 9. Future extensions (deferred)

| Item | Notes |
|---|---|
| Profiles | Named presets replacing or complementing flags |
| Several machines per role | Needs a rule to choose between them, for example least busy |
| Model aliases | Short names for models |
| Task-aware routing | Only if flags and rules prove insufficient |
