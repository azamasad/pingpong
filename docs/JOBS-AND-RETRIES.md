# Jobs and Retries

Status: v3. Describes V1 behaviour and the durable sessions, queue and
graceful restart that are built and running (V2, V3, V6).

Related: [API.md](API.md) section 4, [CONFIGURATION.md](CONFIGURATION.md),
[TESTING.md](TESTING.md) section 2.5.

## 1. The idea

The owner should be able to start a long task, close the MacBook Air, and
return later to find it finished (or failed) and readable. For that to work,
a request must not depend on the client staying connected.

## 2. Why this is a real requirement

If the browser's open connection is what keeps a request alive, closing the
laptop ends it. To survive, Mane must:

1. Accept the request and keep running it with no client attached.
2. Save the output as it is produced, so it can be shown later.

This is a small saved record per request and uses little memory, so it fits the
RAM budget.

## 3. V1 behaviour

- The request is tied to the client connection.
- Busy target: wait, with a visible "waiting" state.
- Down target: return an error immediately. No retry.
- Failure mid-task: an in-band error. No retry.
- Disconnecting cancels the request: a waiting one is dropped, a running one is
  cancelled upstream so the worker stops. The work is lost. This is a
  documented limitation.

The retry loop is not in V1 because it needs requests to outlive the client,
which is V2. See D-014.

## 4. V2 behaviour

### Request states

| State | Meaning |
|---|---|
| `waiting` | Accepted, waiting for the intended machine to be free |
| `running` | The machine is generating output |
| `retrying` | The machine was down or failed. Waiting for the next attempt |
| `done` | Finished. Output is saved |
| `failed` | Gave up after the retry window, or hit a non-retryable error |
| `cancelled` | Cancelled by the owner |

Allowed moves:

| From | To |
|---|---|
| `waiting` | `running`, `retrying`, `failed`, `cancelled` |
| `running` | `done`, `retrying`, `failed`, `cancelled` |
| `retrying` | `waiting`, `running`, `failed`, `cancelled` |
| `done`, `failed`, `cancelled` | None. These are final |

Every state change is saved with a timestamp (status history).

### Retry rule

| Item | Value |
|---|---|
| Applies when | The target machine is down or unreachable at submission, or fails partway through a task |
| Interval | Every 2 minutes |
| Window | Up to 3 hours from the **first** failure (about 90 attempts). A later failure does not reset it |
| After the window | Stop and mark `failed` |
| Restart point | From the beginning of the task. No resuming of partial output |
| Rerouting | None. The request stays with its intended machine |
| Busy machine | Not a failure. The request is `waiting`, with no retry count |
| Timeouts | A first-token or idle timeout counts as a failure and is retried |

Partial output from an interrupted attempt is kept in the history, marked with
its attempt number, for reference. It is not used to resume. The final output
is that of the last successful attempt.

Non-retryable errors (do not retry, mark `failed`): a pinned model that is not
installed anywhere (`MODEL_NOT_INSTALLED`; `MODELS_UNKNOWN` is retryable), an unknown machine, an ambiguous pinned model, or a
malformed request. Where possible these are rejected at submission and never
saved.

While a request is `retrying`, other requests for other machines proceed
normally. When the machine returns, retrying requests for it run in
submission order.

### What is saved

| Field | Notes |
|---|---|
| Request ID | Returned to the client on submit. Unique and sortable by time |
| Messages and options | As submitted |
| Flags and pins | As submitted |
| Chosen machine and model, and mode | auto, fast, or a pinned mode |
| Status and status history | State changes with timestamps |
| Attempts | Count, and for each: start, end, outcome, error |
| First failure time | Starts the retry window |
| Next attempt time | While retrying |
| Output | Saved as produced, tagged by attempt. Reasoning (`thinking`) text, if any, is saved as a separate event type |
| Timestamps | Created, started, finished |

### Suggested storage shape

An embedded database file on Mane (SQLite is recommended, see
[IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md)). Two tables are enough:

| Table | Holds |
|---|---|
| Requests | One row per request: fields above, current status, final output |
| Events | One row per event: request ID, sequence number, time, type (status, output chunk, error, attempt), data |

Notes:

- Events carry a per-request sequence number, so `events?from=<seq>` can
  replay and then continue live.
- Output is saved in small batches (about once a second) to avoid a write for
  every token.
- Retention deletes requests and their events together.
- Storage size is roughly the text of prompts and outputs, which is small.

### Restart recovery

When the router starts, it reads saved requests:

| Saved state | What happens |
|---|---|
| `waiting` | Stays waiting. It runs when its machine is free |
| `retrying` | Continues retrying on schedule |
| `running` | The attempt was interrupted by the restart. Record it as a failed attempt, move to `retrying`, and try again at once. The retry window continues from the original first failure time (or starts now if this is the first failure) |
| `done`, `failed`, `cancelled` | Left as they are |

Built (V6): a clean shutdown stops running jobs on purpose and marks them
`retrying` with `error_code` `ROUTER_RESTARTING`, instead of relying on this
recovery after the process dies. The interrupted attempt counts toward the
retry window like any other failure (D-071), and the job resumes right after
the restart. See [V6-OPERABILITY.md](V6-OPERABILITY.md) section 4.

### Cancel

The owner can cancel a request in `waiting`, `running` or `retrying`. A
running request is cancelled upstream so the worker stops. The state becomes
`cancelled`, and partial output is kept. A saved request can also be deleted.

### Retention

Finished results (`done`, `failed`, `cancelled`) are kept for **90 days**, then
removed by a daily sweep. Requests still active are never removed.

### Coming back later

1. Open the page on Mane (over Tailscale).
2. Find the request in the history.
3. See its status and read the output, or watch it continue if it is still
   running.

## 5. Waiting and busy

Without a full queue, "waiting" means the router holds a request until its
target is free. With several requests waiting for the same machine, their
order is first come, first served. Priorities and lanes belong to the queue
(below).

## 6. The queue (V3)

Built on the V2 job store (D-066). No broker.

| Lane | Meaning |
|---|---|
| `interactive` | Default for `POST /api/jobs` and the page |
| `batch` | Set `"lane":"batch"` on a job, or submit `POST /api/batches` |

On one machine, waiting interactive jobs are admitted before waiting batch jobs.
Within a lane, order is oldest first. `max_concurrent` is unchanged. A job that
has already been admitted keeps its place (no preemption).

Terminal:

```bash
pingpong batch submit --url http://127.0.0.1:8787 --file items.json
pingpong batch get --url http://127.0.0.1:8787 --id <batch-id>
# V7.1 (planned): pingpong batch cancel --url http://127.0.0.1:8787 --id <batch-id>
```

`items.json` is `{"items":[{"prompt":"..."}, ...]}`. Fetch with `GET /api/batches/{id}`.

## 6a. Planned changes (V7.1 to V7.7, not built)

Jobs gain optional links, all additive: `parent_id` and `stage` (a check or
reduce job belongs to a parent), `thread_id` and `turn` (a conversation turn),
and a `batches` row per new batch. Retention changes: a child job goes with its
parent, jobs of a thread go with the thread (90 days after its last message),
and templates and schedules are never swept. A check stage waits for stage 1 to
be `done` and is created at that moment, so the state table above is unchanged.
Full rules: [NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md) section 1.

## 7. Open questions

- Whether `POST /api/chat` stays a separate path in V2 (see API.md).
- Whether finished requests should also be groupable into conversation
  threads (deferred).

### What is logged

Each finished attempt writes one metadata line to the router log: `job_done`,
`job_retrying`, `job_failed` or `job_cancelled`, with the machine, model, lane,
`error_code` and, for `job_done`, the speeds and token counts. It never
contains the prompt or the output. See [OPERATIONS.md](OPERATIONS.md) section 10.
A job's `done` event also carries these speeds as `stats`.
