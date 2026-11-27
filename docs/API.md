# API

Status: v2, V1 to V7 built (examples use illustrative values, for example the model names). This is the contract between clients (the tiny web page, the
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
- An opt-in OpenAI-compatible `/v1` API (V7.8) uses the same routing engine as
  `POST /api/chat` but speaks SSE and the OpenAI JSON shapes. Off by default.

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
| `profile` | string | no | V5 named preset. Fills `fast`, `model`, and `machine` when those are unset. Unknown names are `BAD_REQUEST` |
| `model` | string | no | Pin a model. Exact Ollama name, for example `name:tag` |
| `machine` | string | no | Pin a machine by registry name |
| `think` | boolean | no | Pass through to Ollama for models that support reasoning. Leave unset for the default non-thinking models. If the model does not support it, Ollama returns an error, which is reported as `UPSTREAM_ERROR` |
| `stream` | boolean | no, default `true` | `false` returns one JSON object at the end |
| `options` | object | no | Generation settings passed through to Ollama. Only these keys are passed: `temperature`, `top_p`, `top_k`, `min_p`, `seed`, `num_predict`, `repeat_penalty`, `stop`, `num_thread`. Others are ignored. The router always sets `num_ctx` itself from configuration and ignores a client-supplied one, so the model is never reloaded with a different window. The router does not send `keep_alive`, so the server's own setting applies |

Rules:

- Provide `messages` or `prompt`, not both.
- `messages` must be non-empty, every `role` must be `system`, `user` or `assistant`,
  every `content` must be a string, and the last message must have role `user`.
  Otherwise `BAD_REQUEST`.
- Pins override `fast`. If `model` or `machine` is given, `fast` is ignored
  and the response mode is a pinned mode.
- Routing follows [ROUTING.md](ROUTING.md).

Response (streaming, `Content-Type: application/x-ndjson`), one JSON object
per line. The first line is always `route`. After that come any mix of
`status`, `ping`, `thinking` and `token` events, and the stream ends with
exactly one `done` or `error`.

**When the response starts.** The router validates the request, checks Host and
Origin, picks the target, and checks the target is not marked down, all
**before writing anything**. Failures up to that point are ordinary HTTP errors
(section 3). Once those pass, the router writes the `200` header and the
`route` event immediately, **before any waiting**, so the client can show where
the request is going. From then on, every problem is an in-band `error` event.

| Event | When | Fields |
|---|---|---|
| `route` | Once, first | `type`, `machine`, `model`, `mode` |
| `status` | On state changes | `type`, `state` (`waiting` or `running`), and for `waiting` a 1-based `position` |
| `thinking` | Many, only for reasoning models | `type`, `text`. The model's reasoning text, kept separate from the answer. The page may hide or collapse it. Never mixed into `token` text |
| `ping` | At the heartbeat interval (default 15 seconds) whenever nothing else was sent | `type`, `elapsed_ms`. A heartbeat, see below. Clients may ignore it |
| `token` | Many | `type`, `text` |
| `done` | Once, last on success | `type`, `machine`, `model`, `mode`, `stats` |
| `error` | Once, last on failure after streaming began | `type`, `code`, `message`, `retryable` |

`mode` values: `auto`, `fast`, `pinned_model`, `pinned_machine`,
`pinned_model_and_machine`.

`status` rules:

- `waiting` is sent only if the request has to wait because the machine is at its
  concurrency limit. `position` is 1-based (1 means next in line) and is sent
  again whenever it changes. If the machine is free, no `waiting` event is sent.
- `running` is sent exactly once, when the request starts executing on the
  machine (after any wait), before the first chunk from Ollama.
- "First chunk" for timeouts means the first line of any kind from Ollama,
  whether answer text, reasoning text, or an empty chunk. Every chunk resets the
  idle timer.

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

`done` also carries `context_warning` (boolean). The router sets it to `true`
if any of these hold:

- `prompt_tokens` is within about 5 percent of the configured `num_ctx` (the
  prompt filled the window, so part of it may have been cut);
- `prompt_tokens + output_tokens` reaches `num_ctx` (the answer ran into the end
  of the window);
- Ollama's done reason is `length`.

This guards against Ollama's silent truncation. If a statistic is missing or
zero (Ollama can omit or understate `prompt_eval_count` when it reuses cached
prompt text), skip the rule that needs it and never divide by zero. The
thresholds are heuristics to tune. The page shows a visible warning when the
flag is set.

**Heartbeats.** A request can legitimately sit for many minutes with nothing to
show: waiting for a busy machine, loading a model, or reading a long prompt on
Firmino. Silent connections get closed by proxies and browsers, and a closed
connection cancels the request in V1. So while a stream is open the router
sends a `ping` event whenever it has sent nothing for the heartbeat interval
(default 15 seconds, see CONFIGURATION). A failed write also tells the router
the client is gone, so it can drop the request and free the machine. Because
the router writes to Tailscale Serve's local proxy and not to the browser, it may
not notice a vanished browser at once. That is why total run time is also capped
(`max_run_time`), and why the page's Stop button and the manual test in
[TESTING.md](TESTING.md) matter.

Example (terminal):

```
curl -N https://MANE_TAILNET_NAME/api/chat -d '{"prompt":"Summarise this in one line: ..."}'
curl -N https://MANE_TAILNET_NAME/api/chat -d '{"prompt":"Hello","fast":true}'
curl -N https://MANE_TAILNET_NAME/api/chat -d '{"prompt":"Hello","model":"llama3.2:3b"}'
```

Raw NDJSON is awkward to read in a terminal. To show only the answer text, pipe
it through `jq` (install it on the MacBook Air once), for example
`... | jq -rj 'select(.type=="token").text'`. No router feature is needed.

Example stream:

```
{"type":"route","machine":"firmino","model":"qwen2.5:14b","mode":"auto"}
{"type":"status","state":"waiting","position":1}
{"type":"status","state":"running"}
{"type":"token","text":"Hel"}
{"type":"token","text":"lo"}
{"type":"done","machine":"firmino","model":"qwen2.5:14b","mode":"auto","stats":{"queue_wait_ms":8200,"total_ms":41000,"prompt_tokens":96,"output_tokens":120},"context_warning":false}
```

An in-band error ends a stream like this:

```
{"type":"error","code":"MACHINE_FAILED","message":"Firmino closed the connection","retryable":true}
```

With `stream:false` there is no stream, so errors use ordinary HTTP status codes
(section 3) and success returns one JSON object:

```
{"route": {"machine": "...", "model": "...", "mode": "..."}, "text": "...", "thinking": "...", "stats": {...}, "context_warning": false}
```

`thinking` is present only when the model produced reasoning text. Heartbeats
do not apply, so long requests should use streaming.

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
| `busy` | Boolean. True only when the router has as many requests in flight on this machine as its concurrency limit allows (see [ROUTING.md](ROUTING.md) section 5) |
| `waiting` | Number of requests waiting for it |
| `default_model` | Model used when none is pinned |
| `models` | Installed models |
| `loaded_models` | Models currently loaded in memory, if known |
| `last_checked` | Timestamp |

Example response entry. Timestamps are RFC 3339. Models are objects, so the page
can show details:

```
{"name":"firmino","roles":["heavy"],"reachable":true,"busy":false,"waiting":0,
 "default_model":"qwen2.5:14b",
 "models":[{"name":"qwen2.5:14b","size_bytes":8988124069,"parameter_size":"14.8B"}],
 "loaded_models":["qwen2.5:14b"],"last_checked":"2026-11-23T10:00:00Z"}
```

A machine that has never been reached has `reachable: false` and an empty
`models` list, and `last_checked` is null.

### 2.4 `GET /api/models`

Returns models, each with the list of machines that have it, for example
`[{"name":"llama3.2:3b","machines":["mane"]}]`, built from each machine's last
known list. Used to fill the page's pin controls.

### 2.5 `GET /api/health`

Returns `{"ok": true, "version": "..."}` when the router is running. It does
not check workers (use `/api/machines`).

## 3. Errors

### 3.1 HTTP errors and in-band errors

- Before the `200` header is written (see section 2.1), any problem is an
  ordinary HTTP error with this JSON body:

```
{"error": {"code": "MODEL_NOT_INSTALLED", "message": "...", "retryable": false}}
```

- After the header is written, a problem is an in-band `error` event with the
  same codes.
- With `stream:false`, nothing is written until the end, so every error uses its
  HTTP status.
- Messages never include prompt or output text.

| Code | HTTP status | Meaning | Retryable |
|---|---|---|---|
| `BAD_REQUEST` | 400 | Invalid body or fields (section 2.1 rules) | no |
| `HOST_NOT_ALLOWED` | 403 | `Host` header not in the allowed list | no |
| `ORIGIN_NOT_ALLOWED` | 403 | `POST` with an unexpected `Origin` | no |
| `NOT_FOUND` | 404 | Unknown path | no |
| `BODY_TOO_LARGE` | 413 | Body over the configured limit | no |
| `MACHINE_UNKNOWN` | 404 | Pinned machine is not in the registry | no |
| `MODEL_NOT_INSTALLED` | 404 | The model is not installed where it must be (pinned, or a machine's default model) | no |
| `MODEL_AMBIGUOUS` | 409 | Pinned model exists on several machines and no machine was named | no |
| `MODELS_UNKNOWN` | 503 | A pinned model was not found, but a machine has never been reached, so it cannot be ruled out | yes |
| `NO_TARGET` | 503 | No machine has the needed role | no |
| `MACHINE_DOWN` | 503 | Target machine is marked down, or could not be connected to (V1: immediate error) | yes (V2 retries automatically) |
| `MACHINE_FAILED` | 502 | The machine failed or dropped the connection after output began (V1: no retry) | yes (V2) |
| `TIMEOUT` | 504 | A time limit was hit: first chunk, gap between chunks, or total run time | yes (V2) |
| `UPSTREAM_ERROR` | 502 | Ollama returned some other error | depends |

### 3.2 Mapping Ollama problems to router errors

| What happens with Ollama | Router error |
|---|---|
| Connection refused, host unreachable, or connect timeout | `MACHINE_DOWN`. Mark the machine down at once |
| Connection drops after output began | `MACHINE_FAILED` |
| No first chunk within `first_token_timeout`, a gap longer than `idle_token_timeout`, or a run longer than `max_run_time` | `TIMEOUT`. Cancel upstream |
| HTTP 404 or an error saying the model is not found | `MODEL_NOT_INSTALLED`. Also refresh the registry |
| HTTP 503 (Ollama's own queue is full) | `UPSTREAM_ERROR`, retryable |
| Any other HTTP error, or an `error` line in the stream | `UPSTREAM_ERROR`. Pass Ollama's message along, never the prompt |

## 4. V2 endpoints (durable sessions)

Built in V2. They exist only when `storage.data_dir` is set, and otherwise
return `404 NOT_FOUND` ("jobs not configured"). `GET /api/health` reports this
as `"jobs": true` or `false`.

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

`POST /api/chat` stays connection-tied (D-064). Durable runs use `/api/jobs`.

### 4.1 V3 queue

| Method and path | Purpose |
|---|---|
| `POST /api/jobs` | Optional `"lane":"interactive"` (default) or `"batch"`. V7.3: `"template"` and `"vars"` instead of `prompt` (see 4.3b) |
| `GET /api/jobs` | Also filters `lane` and `batch_id` |
| `POST /api/batches` | Body `{"items":[ <job body>, ... ], "split"?:{...}}` or V7.3 `{"template","vars"?,"rows":[...], "split"?:{...}}` (`items` and `rows` together is `BAD_REQUEST`). All items run on the batch lane (up to `batches.max_items`, default 500). Optional `split` with `assign` `round_robin` or `weights` and two or more `{machine, model, weight?}` targets on **different** machines. Returns `202` `{"id","job_ids","split"?,"not_split","warnings"?}` |
| `GET /api/batches` | Recent batches: `[{"id","kind","created_at","total","counts":{...}}]`. Query `limit` (default 50), `before` |
| `GET /api/batches/{id}` | Batch view. Query `limit` (default 200, max 1000), `offset`. Fields: `id`, `kind`, `created_at`, `total`, `pending`, `counts`, `split` (per-target progress or null), `jobs`, `next_offset` |
| `POST /api/batches/{id}/cancel` | Cancel every waiting, running and retrying job. Returns `200` `{"cancelled":N}` |

Terminal: `pingpong batch submit|get|cancel` (see [JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md)).

### 4.2 V5 profiles

| Method and path | Purpose |
|---|---|
| `GET /api/profiles` | List the named presets: `[{"name","fast","model","machine"}]`. `model` and `machine` are omitted when unset. Always available, with or without saved requests |

The request field `profile` is described in section 2.1. An explicit `fast`,
`model` or `machine` always wins over the preset.

### 4.3b V7.3 templates (built)

Needs `storage.data_dir`. Template names are `[a-z0-9_-]{1,64}`. A blank is `{{name}}` with `name` matching `[a-z0-9_]+`. Values are plain strings, substituted once with no re-scan. Unused `vars` keys are listed in `"warnings"` on submit. Saved jobs store the final `prompt` and the template name for display; deleting a template does not change past jobs.

| Method and path | Purpose |
|---|---|
| `POST /api/templates` | Create `{"name","body","defaults"?}`. `201` returns the template. Duplicate name or count over `templates.max_count` is `CONFLICT`. Body over `templates.max_bytes` is `BAD_REQUEST` |
| `GET /api/templates` | List `[{"name","vars","bytes","defaults","created_at","updated_at"}]` (no bodies) |
| `GET /api/templates/{name}` | One template including `body`. Missing name is `NOT_FOUND` |
| `PUT /api/templates/{name}` | Replace `body` and optional `defaults`. Name cannot change |
| `DELETE /api/templates/{name}` | `204`. Past jobs unchanged |

`defaults` may set `profile`, `fast`, `model`, `machine`, `lane`, `check`, `options`, `think` (profile and machine are validated at save; model installation is not). Explicit fields on a job or batch request beat defaults.

`POST /api/jobs` accepts `"template":"<name>"` and `"vars":{...}` instead of `prompt` or `messages`. `POST /api/batches` with a template uses `"rows":[{...},...]`; each row supplies per-blank values and overrides shared `vars` for that item. `split` and `check` work as for plain items.

Errors: `TEMPLATE_VARS_MISSING` (400, body lists `missing` names), `TEMPLATE_NOT_FOUND` (404 when using an unknown template name).

### 4.3c V7.4 results export (built)

Needs `storage.data_dir`. Streams one row at a time. Query `format=csv` or `format=md`. Optional `include_thinking=1`, `partial=1`, `raw=1` (CSV only, turns off the formula guard), `bom=1` (CSV UTF-8 BOM). Missing batch or batch with no jobs: `NOT_FOUND`. Bad `format`: `BAD_REQUEST`.

| Method and path | Purpose |
|---|---|
| `GET /api/batches/{id}/export?format=csv\|md[&...]` | Download one batch. Filename `pingpong-batch-<first 8 of id>-<YYYYMMDD>.<ext>` |
| `GET /api/jobs/export?ids=a,b,c&format=...` | Download up to 200 jobs by id (unknown ids skipped; count in `X-Pingpong-Skipped`). Filename `pingpong-jobs-<YYYYMMDD>.<ext>` |

Response `Content-Type` is `text/csv; charset=utf-8` or `text/markdown; charset=utf-8`, with `Content-Disposition: attachment` and `Cache-Control: no-store`. CSV columns: `index`, `status`, `machine`, `model`, `lane`, `stage`, `prompt`, `output`, `output_tokens`, `output_tokens_per_s`, `error_code`, `finished_at`, and `thinking` when requested. Text cells that would inject spreadsheet formulas are prefixed with `'` unless `raw=1`. Completion is logged as `export_done` (counts only, no content).

### 4.3 V6 and V7 (built)

| Method and path | Version | Purpose |
|---|---|---|
| `GET /api/status` | V6 | Machines, job counts by status and lane, oldest waiting job age, router memory and uptime, storage. Never contains prompts or outputs. See [V6-OPERABILITY.md](V6-OPERABILITY.md) |
| `GET /api/host` | V7.10 | One hardware sample per configured machine: CPU percent, RAM, temperature in Celsius when the OS exposes it, Apple thermal level when degrees are not available, network bytes per second since the previous sample, and 1-minute load. A machine on this host is read here. A missing CPU tick counter does not drop the rest of that sample (D-124). Any other machine is read from its `stats_address`, or reported `not_configured`. Never contains prompts or outputs. See D-122 |
| `GET /api/aliases` | V7 | Configured aliases and where each model is installed. See [V7-ALIASES.md](V7-ALIASES.md) |

Error code `ROUTER_RESTARTING` (503, retryable) is returned, and recorded on a
job, when the router is shutting down on purpose. A job that was running is
marked retrying with this code and counts toward the 3-hour window (D-071). An
optional `alias` field appears on the `route` event and job detail (V7).

Terminal: `pingpong doctor [--chat] [--json]` and `pingpong backup`.

### 4.3a V7.5 threads, V7.6 file input, V7.7 schedules (built)

All need `storage.data_dir`. Payloads, validation and errors are in
[NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md) sections 6 to 8.

| Method and path | Version | Purpose |
|---|---|---|
| `POST/GET /api/threads`, `GET/PATCH/DELETE /api/threads/{id}`, `POST /api/threads/{id}/messages` | V7.5 | Saved conversations |
| `POST /api/batches/from-file` | V7.6 | Upload a text file, chunk it, run as a batch, optional reduce |
| `GET/POST /api/schedules`, `GET/PATCH/DELETE /api/schedules/{id}`, `POST /api/schedules/{id}/run`, `GET /api/schedules/{id}/runs` | V7.7 | Scheduled jobs |

New error codes: `CONTEXT_TOO_LARGE` (a job error, no HTTP status),
`THREAD_TOO_LONG` (409), `THREAD_BUSY` (409), `TEMPLATE_VARS_MISSING` (400),
`TEMPLATE_NOT_FOUND` (404), `FILE_TOO_LARGE` (413), `FILE_TYPE_UNSUPPORTED`
(415), `PDF_NO_TEXT` (reserved, not built). Duplicate names and count limits use
the existing `CONFLICT` (409). `POST /api/chat` rejects `check`, `template` and
`split` with `BAD_REQUEST`.

### 4.3b V7.8 OpenAI-compatible API (built)

Opt-in: set `openai.enabled` (default `false`). When off, `/v1/*` returns
`404 NOT_FOUND`. Works without `storage.data_dir`. Host and Origin checks apply
as for `/api/*`. The `Authorization` header is accepted and ignored (clients may
send a dummy key).

| Method and path | Purpose |
|---|---|
| `GET /v1/models` | Lists installed model names, aliases, `pingpong-auto` and `pingpong-fast` |
| `POST /v1/chat/completions` | Chat completion in OpenAI shape (JSON or SSE) |

**Model names.** `pingpong-auto` is auto routing; `pingpong-fast` sets the fast
flag. `<machine>/<model>` pins both when the prefix is a registered machine
name. Any other value pins a model (aliases allowed). Ambiguous or missing models
return errors in the OpenAI error shape.

**Request fields used:** `model`, `messages` (string content or an array of
`{"type":"text","text"}` parts joined with no separator), `stream` (default
`false`), `temperature`, `top_p`, `max_tokens` (maps to `num_predict`), `stop`,
`seed`. Ignored: `user`, `presence_penalty`, `frequency_penalty`. Rejected with
`400 BAD_REQUEST`: `n` other than 1, `tools`, `functions`, `response_format`,
`logprobs`, image parts, unknown roles.

**Non-stream response:** standard `chat.completion` object with `usage` from
Ollama stats and a `pingpong` object `{"machine","mode"}`.

**Stream:** `Content-Type: text/event-stream`, `data: {...}\n\n` chunks with
`delta.content`, a final chunk with `finish_reason`, then `data: [DONE]\n\n`.
While waiting, `: ping\n\n` every `heartbeat_interval`. Thinking text is dropped.

**Errors:** `{"error":{"message","type":"invalid_request_error|server_error","param":null,"code":"<router code>"}}` with the router's HTTP status.

Connection-tied like `/api/chat`: disconnect cancels upstream, no saved record,
no retry.

### 4.4 Stress test (test tooling)

Needs `storage.data_dir`, because it submits batch-lane jobs.

| Method and path | Purpose |
|---|---|
| `POST /api/stress` | Start. Body `{"target":"mane"\|"firmino"\|"both","seconds":1..7200}`. Answers `409 CONFLICT` if a run is already going |
| `GET /api/stress` | State: `running`, `target`, `started_at`, `until`, `remaining_seconds`, `queued`, `last_err`, and per machine `done`, `failed`, `output_tps_median`/`min`/`max`, `prompt_tps_median`, `load_ms_median`, `total_ms_median` |
| `DELETE /api/stress` | Stop and cancel the jobs it created |

Each tick it keeps two short batch jobs open per machine (a one-paragraph
prompt, `num_predict` 128). It runs inside the router, so closing the browser
does not stop it. Shutdown stops it.

### 4.5 Speed stats on a finished request

The `done` event of `/api/chat` and of a job carries `stats`: `total_ms`,
`load_ms`, `prompt_tokens`, `output_tokens`, `prompt_tokens_per_s`,
`output_tokens_per_s` (a job's `done` event has these only when Ollama's final
chunk provided them). `/api/chat` also adds `queue_wait_ms`. The page shows the
speeds beside the route line.

## 5. Security notes for the API

- The router checks the `Host` header against `allowed_hosts` (the full tailnet
  name such as `mane.<tailnet>.ts.net`, plus `localhost` and `127.0.0.1`). It
  compares the host name only, case-insensitively, ignoring the port. Anything
  else is `HOST_NOT_ALLOWED`. For `POST` requests it also checks `Origin`: the
  host part must be in `allowed_hosts` (scheme and port ignored, so the HTTP
  fallback works too). `Origin: null` or an unexpected host is
  `ORIGIN_NOT_ALLOWED`. A request with no `Origin` header, such as `curl`, is fine, so a random web page
  open in the browser cannot make requests to the router. This is a cheap
  defence for a service with no login.
- Request body size is limited (configurable). See CONFIGURATION.
- Prompts are never written to logs (see [ARCHITECTURE.md](ARCHITECTURE.md)).
  One log line per finished request or job records speeds and outcome only
  ([OPERATIONS.md](OPERATIONS.md) section 10).

## 6. Compatibility with Ollama

Verified against Ollama's published API reference (November 2026), and the stream checked against the real Ollama versions on both machines. Re-check
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

Verified on the real machines: the normal stream and final statistics chunk
parse correctly (`pingpong doctor --chat` passes on Mane and Firmino).

Still not captured from a real failure (the reference does not make these
explicit):

- The exact shape and HTTP status of an error for a missing model.
- Whether an error mid-stream arrives as an `error` line or as a closed
  connection. Handle both.
