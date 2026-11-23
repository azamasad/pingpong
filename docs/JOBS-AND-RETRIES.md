# Jobs and Retries

Status: draft v1. Describes behaviour in V1 and the durable-session design
targeted for V2. The queue, planned after V2, builds on this.

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
- Disconnecting mid-request may lose the work. This is a documented limitation.

The retry loop is not in V1 because it needs requests to outlive the client,
which is V2. See DECISIONS D-014.

## 4. V2 behaviour

### Request states

| State | Meaning |
|---|---|
| `waiting` | Accepted, waiting for the intended machine to be free. |
| `running` | The machine is generating output. |
| `retrying` | The machine was down or failed. Waiting for the next attempt. |
| `done` | Finished. Output is saved. |
| `failed` | Gave up after the retry window, or hit a non-retryable error. |

Every state change is saved with a timestamp (status history).

### Retry rule

| Item | Value |
|---|---|
| Applies when | The target machine is down or unreachable at submission, or fails partway through a task |
| Interval | Every 2 minutes |
| Window | Up to 3 hours from the first failure (about 90 attempts) |
| After the window | Stop and mark `failed` |
| Restart point | From the beginning of the task. No resuming of partial output |
| Rerouting | None. The request stays with its intended machine |

Partial output from an interrupted attempt is kept in the history for
reference but is not used to resume.

Non-retryable errors (do not retry, mark `failed`): a pinned model that is not
installed anywhere, an unknown machine, or an ambiguous pinned model.

### What is saved

| Field | Notes |
|---|---|
| Request ID | Returned to the client on submit |
| Prompt and flags | As submitted |
| Chosen machine and model, and how chosen | auto, fast, or pinned |
| Status history | State changes with timestamps |
| Output | Saved as produced |
| Attempt count and last error | For retries |
| Timestamps | Created, started, finished |

### Retention

Finished results are kept for **90 days**, then removed. Storage stays small
because the data is text.

### Coming back later

1. Open the page on Mane (over Tailscale).
2. Find the request in the history.
3. See its status and read the output, or watch it continue if it is still
   running.

## 5. Waiting and busy

Without a full queue, "waiting" means the router holds a request until its
target is free. With several requests waiting for the same machine, their
order is simple first-come, first-served. Priorities and lanes belong to the
queue (below).

## 6. The queue (planned, not in V1 or V2)

The owner expects to need a real queue soon. It should be:

- Built on the V2 saved-request store, not a separate system.
- Simple: first-come, first-served with at most two lanes (interactive and
  batch), and a low per-machine limit on concurrent work.
- Usable from the terminal for submitting batches and fetching results.
- Free of heavy infrastructure (no brokers or job frameworks).

Details to decide when it is scheduled.

## 7. Open questions

- How many requests may run on one machine at the same time.
- How the router recovers saved requests after Mane restarts (resume waiting
  and retrying ones on startup).
- Whether the saved store is plain files or an embedded database.
