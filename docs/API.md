# API

Status: draft v1. This is the contract between clients (the tiny web page, the
terminal) and the router on Mane. It is a specification, not an
implementation. Field names are stable. Small details may be refined during
implementation, but changes must be recorded here.

Related: [ROUTING.md](ROUTING.md), [JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md),
[CONFIGURATION.md](CONFIGURATION.md).

## 1. Principles

- Small and plain. JSON in, JSON or newline-delimited JSON (NDJSON) out.
- Easy to use with `curl` from a terminal.
- No authentication. Access control is Tailscale (see
  [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md)).
- Streaming by default for generation.
- The router is stateless about conversations in V1: the client sends the
  full message history every time.
- Not an OpenAI-compatible API. That is deferred (see PRD section 10).

Base URL: `https://<mane-tailnet-name>`, served by Tailscale Serve, which
forwards to the router on localhost (see [SETUP-GUIDE.md](SETUP-GUIDE.md)
section 4). The router's own port is configurable (see CONFIGURATION).

## 2. V1 endpoints

| Method and path | Purpose |
|---|---|
| `GET /` | The tiny web page |
| `POST /api/chat` | Send a request. Streams the answer |
| `GET /api/machines` | Machines, roles, reachability, busy state, installed models |
| `GET /api/models` | Models and which machines have them |
| `GET /api/health` | Router liveness and version |

### 2.1 `POST /api/chat`

Request body (JSON):

| Field | Type | Required | Meaning |
|---|---|---|---|
| `messages` | array of `{role, content}` | one of `messages` or `prompt` | Full conversation. Roles: `system`, `user`, `assistant` |
| `prompt` | string | one of `messages` or `prompt` | Shorthand for a single user message |
| `fast` | boolean | no | Send to the `fast` role machine (Mane) with its default model |
| `model` | string | no | Pin a model. Exact Ollama name, for example `name:tag` |
| `machine` | string | no | Pin a machine by registry name |
| `think` | boolean | no | Pass through to Ollama for models that support reasoning. Leave unset for the default non-thinking models. If the model does not support it, Ollama returns an error, which is reported as `UPSTREAM_ERROR` |
| `stream` | boolean | no, default `true` | `false` returns one JSON object at the end |
| `options` | object | no | Optional generation settings passed through to Ollama (for example temperature). Ignored if unknown |

Rules:

- Provide `messages` or `prompt`, not both.
- Pins override `fast`. If `model` or `machine` is given, `fast` is ignored
  and the response mode is a pinned mode.
- Routing follows [ROUTING.md](ROUTING.md).

Response (streaming, `Content-Type: application/x-ndjson`), one JSON object
per line, in this order:

| Event | When | Fields |
|---|---|---|
| `route` | Once, first | `type`, `machine`, `model`, `mode` |
| `status` | On state changes | `type`, `state` (`waiting` or `running`), and for `waiting` an optional `position` |
| `thinking` | Many, only for reasoning models | `type`, `text`. The model's reasoning text, kept separate from the answer. The page may hide or collapse it. Never mixed into `token` text |
| `token` | Many | `type`, `text` |
| `done` | Once, last on success | `type`, `machine`, `model`, `mode`, `stats` |
| `error` | Once, last on failure after streaming began | `type`, `code`, `message`, `retryable` |

`mode` values: `auto`, `fast`, `pinned_model`, `pinned_machine`,
`pinned_model_and_machine`.

`stats` fields (best effort), taken from Ollama's final object. Ollama reports
durations in **nanoseconds**, so convert:

| Field | Source |
|---|---|
| `queue_wait_ms` | Measured by the router (time spent waiting for the machine) |
| `total_ms` | `total_duration` from Ollama, converted |
| `load_ms` | `load_duration` from Ollama, converted |
| `prompt_tokens` | `prompt_eval_count` |
| `prompt_tokens_per_s` | `prompt_eval_count` divided by `prompt_eval_duration` |
| `output_tokens` | `eval_count` |
| `output_tokens_per_s` | `eval_count` divided by `eval_duration` |

These make the speed measurements in
[HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 8 easy to collect.

Example (terminal):

```
curl -N https://MANE_TAILNET_NAME/api/chat -d '{"prompt":"Summarise this in one line: ..."}'
curl -N https://MANE_TAILNET_NAME/api/chat -d '{"prompt":"Hello","fast":true}'
curl -N https://MANE_TAILNET_NAME/api/chat -d '{"prompt":"Hello","model":"llama3.2:3b"}'
```

Example stream:

```
{"type":"route","machine":"firmino","model":"qwen2.5:14b","mode":"auto"}
{"type":"status","state":"waiting","position":1}
{"type":"status","state":"running"}
{"type":"token","text":"Hel"}
{"type":"token","text":"lo"}
{"type":"done","machine":"firmino","model":"qwen2.5:14b","mode":"auto","stats":{"queue_wait_ms":8200,"total_ms":41000,"output_tokens":120}}
```

With `stream:false`, the response is one JSON object: `route` fields, the full
`text`, and `stats`.

When the stream starts, the HTTP status is `200`. Problems after that are
reported in-band as an `error` event.

### 2.2 Disconnecting and cancelling (V1)

If the client closes the connection:

- A waiting request is removed from the wait list.
- A running request is cancelled upstream, so the worker stops generating and
  does not keep using its CPU or GPU.

This is how cancel works in V1. Explicit cancel by ID arrives with V2.

### 2.3 `GET /api/machines`

Returns a list. Each machine:

| Field | Meaning |
|---|---|
| `name` | Registry name, for example `mane` |
| `roles` | For example `["fast"]` or `["heavy"]` |
| `reachable` | Boolean, from the latest health check |
| `busy` | Boolean, true while the router has a request in flight on it |
| `waiting` | Number of requests waiting for it |
| `default_model` | Model used when none is pinned |
| `models` | Installed models |
| `loaded_models` | Models currently loaded in memory, if known |
| `last_checked` | Timestamp |

### 2.4 `GET /api/models`

Returns models, each with the list of machines that have it. Used to fill the
page's pin controls.

### 2.5 `GET /api/health`

Returns `{"ok": true, "version": "..."}` when the router is running. It does
not check workers (use `/api/machines`).

## 3. Errors before streaming starts

Routing and validation errors return an ordinary HTTP error with a JSON body:

```
{"error": {"code": "MODEL_NOT_INSTALLED", "message": "...", "retryable": false}}
```

| Code | HTTP | Meaning | Retryable |
|---|---|---|---|
| `BAD_REQUEST` | 400 | Invalid body, both or neither of `messages` and `prompt`, and so on | no |
| `MACHINE_UNKNOWN` | 404 | Pinned machine is not in the registry | no |
| `MODEL_NOT_INSTALLED` | 404 | Pinned model is not installed on any (or the pinned) machine | no |
| `MODEL_AMBIGUOUS` | 409 | Pinned model exists on several machines and no machine was named | no |
| `MACHINE_DOWN` | 503 | Target machine is unreachable (V1: immediate error) | yes (V2 retries automatically) |
| `NO_TARGET` | 503 | No machine has the needed role (for example no `heavy` machine registered) | no |

Errors after streaming started (in-band `error` event) use the same codes plus:

| Code | Meaning | Retryable |
|---|---|---|
| `MACHINE_FAILED` | The machine failed or the connection dropped mid-task (V1: no retry) | yes (V2) |
| `TIMEOUT` | No first token or no further tokens within the configured limits | yes (V2) |
| `UPSTREAM_ERROR` | Ollama returned an error | depends |

## 4. V2 endpoints (durable sessions)

Not part of V1. Specified here so V1 choices do not block them.

| Method and path | Purpose |
|---|---|
| `POST /api/jobs` | Submit a request. Same body as `/api/chat`. Returns `{"id": "..."}` immediately. The request runs in the background |
| `GET /api/jobs` | List saved requests. Filters: `status`, `limit`, `before` |
| `GET /api/jobs/{id}` | Details: prompt, flags, route, status, status history, attempts, output so far |
| `GET /api/jobs/{id}/events` | NDJSON stream of events. `?from=<seq>` resumes from a point. Replays saved events first, then continues live if still running |
| `POST /api/jobs/{id}/cancel` | Cancel a waiting, running or retrying request |
| `DELETE /api/jobs/{id}` | Delete a saved request |

Event types extend V1's: `status` gains `retrying` (with `attempt` and
`next_attempt_at`), `failed` and `cancelled`, and each event carries a
sequence number.

`POST /api/chat` may remain as a convenience that creates a job and attaches
to it. If the client disconnects, the job continues. Decide when V2 is built
and record it in [DECISIONS.md](DECISIONS.md).

## 5. Security notes for the API

- The router should check the `Host` header against the configured names, and
  reject unexpected `Origin` headers on `POST` requests, so a random web page
  open in the browser cannot make requests to the router. This is a cheap
  defence for a service with no login.
- Request body size is limited (configurable). See CONFIGURATION.
- Prompts are never written to logs (see [ARCHITECTURE.md](ARCHITECTURE.md)).

## 6. Compatibility with Ollama

Verified against Ollama's published API reference (November 2026). Re-check
against the installed version when building, because the API evolves. The
reference is at `docs.ollama.com/api`.

| Purpose | Ollama endpoint | Notes |
|---|---|---|
| Stream a chat | `POST /api/chat` | Body: `model`, `messages`, optional `options`, `stream`, `keep_alive`, `think`. Returns newline-delimited JSON objects. Set `"stream": false` for a single object |
| List installed models | `GET /api/tags` | Returns `models` with `name`, `size`, and `details` (parameter size, quantization). Also the cheap reachability check |
| List loaded models | `GET /api/ps` | Returns `models` currently in memory, with `expires_at` and `size_vram` |
| Version | `GET /api/version` | Optional. Useful for logging the Ollama version |
| Cancel generation | Close the HTTP connection | Ollama stops generating |
| Preload a model | `POST /api/chat` with only `model` | Empty request loads it. Optional, not needed for V1 |

How Ollama's chat stream looks (what the router must parse):

- Each line is a JSON object with `model`, `created_at`, `message`
  (`role`, `content`, and for reasoning models `thinking`), and `done: false`.
- The last object has `done: true`, an empty `content`, and the statistics
  listed in section 2.1, and may include a done reason.
- The router turns `message.content` into `token` events and
  `message.thinking` into `thinking` events.
- If a line contains an `error` field, treat it as `UPSTREAM_ERROR`.

Other points from the reference:

| Topic | Fact |
|---|---|
| Model names | `name:tag`. With no tag, Ollama uses `latest` |
| Keep-alive | The `keep_alive` request field overrides the server's `OLLAMA_KEEP_ALIVE`. Default is 5 minutes |
| Overload | If its queue is full, Ollama answers with HTTP 503. Map this to `UPSTREAM_ERROR` (retryable) |
| Context window | Default 4096 tokens. Set explicitly (see [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 4) |
| Options | Per-request `options` accept Modelfile parameters such as `temperature`, `num_ctx` and `num_thread` |

Still to confirm by trying it against the real installed version (the
reference does not make these explicit):

- The exact shape and HTTP status of an error for a missing model.
- Whether an error mid-stream arrives as an `error` line or as a closed
  connection. Handle both.
