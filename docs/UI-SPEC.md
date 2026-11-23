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

## 3. V2 features

| Feature | Description |
|---|---|
| Request history | List of past requests with status and time |
| Open a past request | See its prompt, flags, status history, and saved output |
| Reattach | Open a still-running request and watch it continue |
| Retry visibility | Shows the retrying state, attempt count and next attempt time |

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

- Exact flag and pin controls.
- Whether to show raw token speed.
