# Routing

Status: draft v2.

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
| Models installed on each machine | Registry (read from each Ollama) |
| Whether each machine is reachable | Health check |
| Whether each machine is busy | Router's own count of requests in flight |

## 2. Modes

| Mode | How to select | Where it goes |
|---|---|---|
| **Auto** | No flags | The machine with the `heavy` role (Firmino), its default model (14B) |
| **Fast** | `fast` flag | The machine with the `fast` role (Mane), its default model (8B) |
| **Pinned model** | Name a model | The machine that has that model |
| **Pinned machine** | Name a machine | That machine, with its default model |
| **Pinned model and machine** | Name both | Exactly that pair |

Profiles (named presets such as "fast / balanced / deep") are deferred.

## 3. Decision order

1. **Pins first.** If a model and/or a machine is pinned, `fast` is ignored and:
   - Machine named, model not named: that machine, default model.
   - Model named, machine not named: find machines that have the model.
     - None: `MODEL_NOT_INSTALLED`.
     - One: use it.
     - Several: `MODEL_AMBIGUOUS`. Do not guess. Ask the user to name a machine.
   - Both named: the machine must exist (`MACHINE_UNKNOWN` otherwise) and have
     the model (`MODEL_NOT_INSTALLED` otherwise).
2. **Fast.** Otherwise, if `fast` is set, target the `fast` role machine's
   default model.
3. **Auto.** Otherwise, target the `heavy` role machine's default model.
4. If the needed role has no machine: `NO_TARGET`.
5. If the chosen machine is marked down: `MACHINE_DOWN` (V1). In V2 the
   request is accepted and retried (see
   [JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md)).

The choice is made once, at the start of a request. It is not changed
mid-task.

## 4. Model names

- Models are matched by their exact Ollama name, usually `name:tag`.
- If a name is given without a tag, treat it as the default tag the way Ollama
  does, so it matches the installed model.
- Matching is case-sensitive.
- If a pinned model is not found, refresh the registry once and look again
  before failing, in case it was just installed.
- Short aliases are deferred.

## 5. Busy, down and unknown

| State of the target | Behaviour |
|---|---|
| Free | Run now |
| Busy | Wait for it, first come first served. Do not reroute. Show "waiting" |
| Down or unreachable | Error. In V2: retry every 2 minutes for up to 3 hours |
| Model installed but not loaded | Allowed. It loads on demand, so the first answer is slower |

"Wait, don't reroute" holds for auto, fast and pinned requests alike: a request
meant for Firmino waits for Firmino.

### What "busy" means

- Each machine has a concurrency limit (default **1**, configurable).
- A machine is busy when the router has that many requests in flight on it.
- Requests beyond the limit wait in a first-come first-served list per machine.
- The router can only count requests that pass through it. Anything used on
  Mane directly (for example the owner using Ollama at the Mac itself) is not
  counted, which is accepted.
- Machines do not block each other.

### Health

- Each machine is checked on a schedule (default 30 seconds).
- A machine is marked down after a few consecutive failed checks (default 2),
  or immediately if a request cannot connect.
- It is marked up again as soon as a check succeeds.

### Timeouts

CPU inference can take minutes before the first token, so limits are generous
(see [CONFIGURATION.md](CONFIGURATION.md) for defaults):

- Connecting to a machine: short.
- First token: long (covers model loading and processing a long prompt).
- Gaps between tokens: moderate.
- Waiting for a busy machine: no limit in V1. The client can cancel.

If a limit is hit, the request ends with a `TIMEOUT` error.

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
| No flags | Firmino, 14B, `auto` |
| `fast` | Mane, 8B, `fast` |
| Pin model `3B` | Mane, 3B, `pinned_model` |
| Pin machine `firmino` | Firmino, default model, `pinned_machine` |
| Pin machine `firmino` and the 14B model | Firmino, that model, `pinned_model_and_machine` |
| `fast` and pin machine `firmino` | Firmino. The pin wins |
| Pin model `70B` (not installed) | Error `MODEL_NOT_INSTALLED` |
| Pin a model installed on two machines | Error `MODEL_AMBIGUOUS` |
| Auto while Firmino is busy | Waits for Firmino |
| Auto while Firmino is down | Error (V1). Retries (V2) |
| `fast` while Mane is busy | Waits for Mane |

## 9. Future extensions (deferred)

| Item | Notes |
|---|---|
| Profiles | Named presets replacing or complementing flags |
| Several machines per role | Needs a rule to choose between them, for example least busy |
| Model aliases | Short names for models |
| Task-aware routing | Only if flags and rules prove insufficient |

## 10. Open questions

- Default model per machine when several are installed: it is set in
  configuration, so none needed in code. Which model is chosen is the owner's
  call at setup.
