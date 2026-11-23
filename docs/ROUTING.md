# Routing

Status: draft v1.

Routing is done by Mane's router, using simple rules and clear signals only.
It never reads a prompt to guess difficulty (see constitution P9).

## 1. Inputs to a routing decision

| Input | Source |
|---|---|
| Flags on the request | Client |
| Pinned model, pinned machine | Client |
| Machines and roles | Registry |
| Models installed on each machine | Registry (read from each Ollama) |
| Whether each machine is reachable | Health check |
| Whether each machine is busy | Router's own bookkeeping |

## 2. Modes

| Mode | How to select | Where it goes |
|---|---|---|
| **Auto** | No flags | The `heavy` role machine (Firmino), default model (14B) |
| **Fast** | `fast` flag | Mane, default model (8B) |
| **Pinned model** | Name a model | The machine that has that model |
| **Pinned machine** | Name a machine | That machine, default model unless a model is also named |

Profiles (named presets such as "fast / balanced / deep") are deferred.

## 3. Decision order

1. If a model and/or a machine is pinned, use exactly that.
   - If the model is installed on more than one machine and no machine is
     named, the request is ambiguous. Return an error asking the user to name a
     machine. Do not guess.
   - If the model is not installed anywhere, or the machine is unknown, return
     an error. No substitution.
2. Otherwise, if the `fast` flag is set, target Mane's default model.
3. Otherwise (auto), target the `heavy` role's default model.

The choice is made once, at the start. It is not changed mid-task.

## 4. Busy, down and unknown

| State of the target | Behaviour |
|---|---|
| Busy | Wait for it. Do not reroute. Show "waiting". |
| Down or unreachable | Error. In V2: retry every 2 minutes for up to 3 hours. |
| Model not loaded but installed | Allowed. It loads on demand, so the first answer is slower. |

"Wait, don't reroute" holds for auto, fast and pinned requests alike: a request
meant for Firmino waits for Firmino.

## 5. Why Firmino is the default

Speed is not the priority. Most of the owner's work is expected to be long or
heavy. Defaulting to Firmino keeps Mane free for quick, interactive work
requested with `fast`, and avoids loading large models on Mane.

## 6. Transparency

Each response includes:

- the model used
- the machine used
- how the target was chosen (auto, fast, or pinned)

## 7. Examples

| Request | Result |
|---|---|
| No flags | Firmino, 14B, "auto" |
| `fast` | Mane, 8B, "fast" |
| Pin model `3B` | Mane, 3B, "pinned model" |
| Pin machine `firmino` | Firmino, default model, "pinned machine" |
| Pin model `70B` (not installed) | Error: model not installed |
| Auto while Firmino is busy | Waits for Firmino |
| Auto while Firmino is down | Error (V1). Retries (V2). |
| `fast` while Mane is busy | Waits for Mane |

## 8. Future extensions (deferred)

| Item | Notes |
|---|---|
| Profiles | Named presets replacing or complementing flags. |
| Several machines per role | Needs a rule to choose between them (for example least busy). |
| Task-aware routing | Only if flags and rules prove insufficient. |

## 9. Open questions

- Exact flag and pin syntax in the page and in the terminal endpoint.
- How "busy" is measured per machine (concurrency of one request, or more).
- Default model per machine when several are installed.
