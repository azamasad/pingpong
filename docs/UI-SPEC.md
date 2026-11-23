# UI Specification

Status: draft v1.

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
| Stop button | Shown while a request is waiting or running. Pressing it cancels the request (the worker stops) |
| Waiting | Shows "Waiting for Firmino" (or the machine name) and the position if more than one |
| Mode label | Each answer shows a small label such as "Firmino, qwen2.5:14b, auto" |
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

## 4. Not included

- Logins and user management.
- Settings pages and themes.
- Model installation or deletion from the page.
- Charts and dashboards.
- Notifications (deferred).
- RAG or document features.

## 5. Terminal access

A scriptable endpoint on the same router supports terminal use (curl or a
tiny wrapper) with the same flags. It is not a separate UI. Batch submission
from the terminal arrives with the queue (see [ROADMAP.md](ROADMAP.md)).

## 6. Open questions

- Whether to show raw token speed (nice to have, not required).
- Whether the page should remember the last settings across reloads (for
  example in the browser's local storage).
