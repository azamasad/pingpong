# UI Specification

Status: v3. V1 to V7 UI implemented, plus the Status and Stress tabs.

A tiny, built-in web page served by the router on Mane. One page, no
logins, used by one person.

## 1. Constraints

| Constraint | Value |
|---|---|
| Where it runs | Served by the router on Mane. Nothing runs on Firmino |
| Memory | Part of the router's 50 MB working budget. No separate UI server |
| Weight | Static and minimal. No heavy UI platforms |
| Assets | All local. No external fonts, scripts or trackers |
| Access | Over Tailscale, from the MacBook Air (and a phone if wanted) |
| Auth | None (Tailscale is the control) |
| Devices | Works on a laptop and on a phone-sized screen |

## 2. V1 features

| Feature | Description |
|---|---|
| Prompt box | Type a request and send it |
| Fast toggle | Sends the request to Mane (the `fast` flag) |
| Pin model | Optional selector, filled from the registry |
| Pin machine | Optional selector, filled from the registry |
| Response area | Shows the answer as it streams |
| Provenance | Shows the model, the machine, and how it was chosen (auto, fast, pinned) |
| Request state | Shows waiting, running, done or failed |
| Machine status | Shows whether Mane and Firmino are reachable |
| Clear errors | Plain messages, for example "Model not installed" or "Firmino is down" |

### V1 behaviour details

| Topic | Behaviour |
|---|---|
| Conversation | The page keeps the message history in the browser and sends it all with each request. Reloading the page loses it in V1 |
| Settings | Fast toggle and pins stay as set between messages, so a conversation keeps its target until changed |
| Elapsed time | While waiting or running, show elapsed time. The clock ticks in the browser, and `ping` events confirm the connection is alive and correct the clock. If pings stop arriving for more than about twice the heartbeat interval, show a "connection lost" state |
| Context warning | If `context_warning` is set on `done`, show a clear notice: the prompt filled the context window and part of it may have been cut |
| Machine status refresh | Refresh machine status every few seconds while the page is open |
| Stop button | Shown while a request is waiting or running. Pressing it cancels the request (the worker stops) |
| Waiting | Shows "Waiting for Firmino" (or the machine name) and the position if more than one |
| Mode label | Each answer shows a small label such as "Firmino, qwen3:30b-a3b-instruct-2507-q4_K_M, auto". When the request finishes, speeds are appended: output tokens/s, prompt tokens/s and model load time, when Ollama reports them |
| Thinking text | If a reasoning model streams `thinking` events, show them collapsed and separate from the answer, or hide them. Never mix them into the answer |
| Errors | Plain language, built from the error codes in [API.md](API.md), for example "That model is not installed" or "Firmino is down" |
| Machine status | A small indicator per machine: reachable, busy, down |
| Pin controls | Dropdowns filled from `GET /api/machines` and `GET /api/models`. Default is "none" |
| Pin plus fast | Choosing a pin disables or ignores the fast toggle, matching the routing rules |
| Not a chat history | Past conversations are not saved in V1 |
| Accessibility | Keyboard friendly, readable text, works at phone widths |

## 3. V2 features

| Feature | Description |
|---|---|
| Request history | List of past requests with status and time |
| Open a past request | See its prompt, flags, status history, and saved output |
| Reattach | Open a still-running request and watch it continue |
| Retry visibility | Shows the retrying state, attempt count and next attempt time |
| Cancel and delete | Cancel an active request. Delete a saved one |
| Status filter | Filter the history by status |

## 3a. Tabs (V6 and test tooling)

The page has three tabs: **Chat** (everything above), **Status** and **Stress**.

| Tab | Description |
|---|---|
| Status | Machines (reachable, models, in flight), job counts by status and lane, oldest waiting job age, router memory and uptime. Reads `GET /api/status`. Never shows prompts or outputs |
| Stress | Pick a target (mane, firmino, both) and a duration (1 to 7200 seconds), then Start or Stop. Shows remaining time and, per machine, done and failed counts and median, minimum and maximum speeds. The run is on the server and survives closing the page. Reads `/api/stress` |

## 3b. Batches and Templates tabs (V7.1 to V7.3, built)

Hidden when jobs are off. **Batches** (V7.1, extended by V7.2): batch list, new batch form with split and check, batch view with per-machine progress and cancel. **Templates** (V7.3): list with new, edit, delete, and a Use panel (one field per blank, routing selectors, run one job or run as batch with one JSON object per line). The Chat tab has a "Check with" selector (V7.2).

## 3c. Planned tabs (V7.5 to V7.7, not built)

| Tab | Version | Holds |
|---|---|---|
| Threads | V7.5 | Thread list and conversation view |
| Schedules | V7.7 | Schedule table, form, runs |

Batches will gain export (V7.4), file input (V7.6), and related controls when those versions ship. Contract in [NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md).

## 4. Not included

- Logins and user management.
- Settings pages and themes.
- Model installation or deletion from the page.
- Charts and dashboards (the Status and Stress tabs show numbers only).
- Notifications in the page (V7.9 sends one optional webhook message, nothing is shown in the page).
- RAG. (Text file chunking arrives with V7.6 in the Batches tab.)

## 5. Terminal access

A scriptable endpoint on the same router supports terminal use (curl or a
tiny wrapper) with the same flags. It is not a separate UI. Batch submission
from the terminal arrives with the queue (see [ROADMAP.md](ROADMAP.md)).

## 6. Open questions

- Whether the page should remember the last settings across reloads (for
  example in the browser's local storage).
