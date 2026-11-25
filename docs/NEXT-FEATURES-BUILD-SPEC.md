# Build spec for V7.1 to V7.9

Status: **built (M21 to M29).** This file stays the acceptance contract. Written so a coding agent can build each
version without guessing. Product intent and stories are in
[NEXT-FEATURES-PRDS.md](NEXT-FEATURES-PRDS.md). This file is the single source
for payloads, storage, config, errors, limits, UI and acceptance of the planned
versions (D-106 to D-117). Where this file and the PRDs file differ, this file
wins, because it is the later and more precise one. Constitution first, always.

When a version is built, move its contract into [API.md](API.md),
[CONFIGURATION.md](CONFIGURATION.md) and [JOBS-AND-RETRIES.md](JOBS-AND-RETRIES.md),
mark it built here, and keep the acceptance list as the test list.

## 0. Order, dependencies, one PR each

| # | Version | Needs | Provides to later versions |
|---|---|---|---|
| 1 | V7.1 split | none | `batches` table, atomic batch submit, paged batch view, Batches tab, fake Ollama upgrades (section 1.9) |
| 2 | V7.2 check | V7.1 (`batches`) | `parent_id` and `stage` on jobs, stage creation hook, restart recovery for stages |
| 3 | V7.3 templates | none | One substitution function, `templates` table, Templates tab |
| 4 | V7.4 export | V7.1, V7.2 (columns) | Streaming row writer |
| 5 | V7.5 threads | none | `thread_id` and `turn` on jobs, retention exemption, Threads tab |
| 6 | V7.6 file input | V7.1, V7.2 (stage hook), V7.3 (substitution) | Upload endpoint |
| 7 | V7.7 schedules | V7.1, V7.3 | Scheduler timer, Schedules tab |
| 8 | V7.8 OpenAI endpoint | none | Opt-in `/v1` |
| 9 | V7.9 notifications | V7.1, V7.7 | One webhook |

Build in this order, one at a time, one pull request per version. Do not start
V7.8 or V7.9 before V7.7 is merged.

Every version ends with this checklist (the "common done" list). Each section
below adds its own acceptance items on top.

- [ ] `make lint && make test && make build` pass. New behaviour has tests, time-dependent code uses `internal/clock`.
- [ ] Smoke run: build the binary, run it against two stand-in Ollama servers with a config that has `storage.data_dir` set, and run the version's smoke commands (each section lists them). Record the result in the pull request.
- [ ] Router RSS measured on Linux under the version's load (each section names it) and recorded in [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 8. Under 50 MB, and growth over the idle figure below the limit named in the section. Over the limit means fix or stop and ask, never raise the budget.
- [ ] No prompt, output, template body, vars, file text or webhook URL in any log line. New log fields are added to the `internal/logx` whitelist and nowhere else (section 1.5).
- [ ] No substitution or rerouting anywhere: every job runs on the pair it was given or stored with.
- [ ] Docs updated in the same pull request (contract moved into API.md and CONFIGURATION.md, TESTING.md section 2.10 ticked, AGENTS.md state table, ROADMAP, README), and a DECISIONS entry for any new decision.
- [ ] Config example (`deploy/config.example.yaml`) gains the new keys, commented, with placeholders only.
- [ ] Owner's real-machine checks for the version written into [TESTING.md](TESTING.md) section 4b (already listed there; update if behaviour changed).

## 1. Shared rules

### 1.1 Storage and migrations

- Only additive changes: `CREATE TABLE IF NOT EXISTS` and new columns through the existing `addColumn` pattern in `internal/store` (generalise it to take a table name). Never drop or rename. Old databases must open and old jobs must keep working. Test: open a database created by the current `main` and run.
- Timestamps are RFC 3339 UTC text, ids are `uuid.NewString()`, like jobs today.
- New columns on `jobs` (all nullable or defaulted): `parent_id TEXT`, `stage TEXT NOT NULL DEFAULT ''`, `check_pending INTEGER NOT NULL DEFAULT 0` (V7.2), `thread_id TEXT`, `turn INTEGER` (V7.5). Indexes: `idx_jobs_batch(batch_id)` (V7.1), `idx_jobs_parent(parent_id)` (V7.2), `idx_jobs_thread(thread_id, turn)` (V7.5), and a unique index on `(parent_id, stage)` where `parent_id` is not null (V7.2).
- `batches` table (V7.1): `id TEXT PRIMARY KEY, created_at TEXT, kind TEXT` (`batch`, `split`, `file`, `schedule`), `item_count INTEGER, plan_json TEXT` (the split plan, empty otherwise), `source TEXT` (a template name or schedule id, empty otherwise). V7.6 adds `reduce_json TEXT`, `reduce_job_id TEXT`. Every new batch gets a row. Batches made before V7.1 and Stress batches have no row and must still work: `GET /api/batches/{id}` falls back to the jobs list with `kind` `batch` and no plan.
- Retention: the existing sweep (`PurgeOlderThan`, by `updated_at`, terminal states only) stays. Additions: it also deletes `batches` rows with no jobs left; a child job (`parent_id` set) is deleted with its parent, never alone, and a parent is swept only when it and its children are all terminal and older than the window; jobs with a `thread_id` are exempt from the job sweep and are removed only with their thread (V7.5); `schedule_runs` follow the 90-day sweep; `templates` and `schedules` are never swept (D-101).
- Backup: `pingpong backup` already copies the whole database. Test that the copy contains the new tables.

### 1.2 Atomic submit

`SubmitBatch` today inserts jobs one by one and can leave a half batch if item 40 is rejected. From V7.1 on, every endpoint that creates several jobs (batches, templates with `rows`, file batches, split) validates **all** items first (routing, size, fields) and then inserts the batch row and all job rows in **one SQLite transaction**, then starts the run loops. Any validation failure creates nothing and returns the error. Test it.

### 1.3 Error codes

New codes go in `internal/apperrors` (status and retryable flag) and in [API.md](API.md) section 3.1 when built. Existing codes are reused where they fit.

| Code | HTTP | Retryable | Used by | Meaning |
|---|---|---|---|---|
| `CONTEXT_TOO_LARGE` | none (job error) | no | V7.2, V7.6 | A review, revise or reduce prompt does not fit `num_ctx`. Set on the stage job, which ends `failed` at once. Never truncated |
| `THREAD_TOO_LONG` | 409 | no | V7.5 | History plus the new turn does not fit `num_ctx`, or `threads.max_turns` is reached. Body also has `estimated_tokens` and `limit` |
| `THREAD_BUSY` | 409 | yes | V7.5 | The thread has an active turn |
| `TEMPLATE_VARS_MISSING` | 400 | no | V7.3, V7.6, V7.7 | A blank has no value. Body lists the names |
| `TEMPLATE_NOT_FOUND` | 404 | no | V7.3, V7.6, V7.7 | No template with that name |
| `FILE_TOO_LARGE` | 413 | no | V7.6 | Over `files.max_bytes` |
| `FILE_TYPE_UNSUPPORTED` | 415 | no | V7.6 | Not `.txt`, `.md` or `.markdown`, not valid UTF-8, or contains NUL. A `.pdf` says "PDF is not supported yet" |
| `PDF_NO_TEXT` | 422 | no | later | Reserved for PDF. Do not build |
| `CONFLICT` (existing, 409) | 409 | no | V7.3, V7.7 | Duplicate name, or a count limit reached. The message says which |

Everything else is `BAD_REQUEST`, `NOT_FOUND`, `MACHINE_UNKNOWN`, `MODEL_NOT_INSTALLED`, `MODELS_UNKNOWN`, `MODEL_AMBIGUOUS`, `NO_TARGET` as today. Messages never contain prompt or output text. When several problems exist, report the first in the order of the request fields.

### 1.4 Job request field additions

A job body (`POST /api/jobs`, a batch item, a template default) may gain these fields over time. Unknown fields are ignored as today. `lane` is forced to `batch` inside any batch.

| Field | Version | Type |
|---|---|---|
| `check` | V7.2 | object: `mode` (`review` or `revise`), optional `machine`, `model`, `options` |
| `template`, `vars` | V7.3 | string, object of strings |
| `thread_id` | V7.5 | set only by the router. A client value is ignored |

`POST /api/chat` (connection-tied, D-064) rejects `check`, `template` and `split` with `BAD_REQUEST`.

### 1.5 Logging (P16, D-076)

Metadata only. Add these fields to the `logx` whitelist, nothing else: `stage`, `batch_id`, `items`, `schedule` (the schedule's name, a short label chosen by the owner), `source` already exists. New events: `batch_submitted` (`batch_id`, `items`, `source` = `split|file|template|schedule|plain`), `export_done` (`items`, `duration_ms`, `outcome`), `schedule_run` (`schedule`, `outcome` = `submitted|skipped_overlap|skipped_missed|failed`, `error_code`, `batch_id`, `request_id`), `check_created` and `check_failed` (`request_id`, `stage`, `error_code`), `reduce_created`. The existing `job_*` lines gain `stage` and `batch_id` when set. Add a test that a prompt, output, template body, var value, file text and webhook URL never appear in any log line produced during the version's smoke scenario.

### 1.6 Page (P6, UI-SPEC)

Still plain HTML and JavaScript, embedded, no framework, no external assets. The page gains one tab per feature that needs one, next to Chat, Status and Stress: **Batches** (V7.1, extended by V7.2, V7.4, V7.6), **Templates** (V7.3), **Threads** (V7.5), **Schedules** (V7.7). All of them need `storage.data_dir` and are hidden when jobs are off (as the lane checkbox is today). Each tab reuses the existing machine, model and profile selectors. Update [UI-SPEC.md](UI-SPEC.md) section 3a when each tab ships. Mobile width must work. The page never shows config values or webhook URLs.

### 1.7 Limits and sizes

| Limit | Value | Notes |
|---|---|---|
| Batch items | `batches.max_items`, default 1000 | Replaces the hard-coded 100 in `handleBatchCreate` (D-107) |
| Request body | `router.max_body_bytes`, default 4 MB | Applies to every JSON endpoint. 1000 items of about 4 KB fit |
| Upload body | `files.max_bytes` plus 64 KB | Only `POST /api/batches/from-file` (V7.6) |
| Template | `templates.max_bytes` 16384, `templates.max_count` 200 | V7.3 |
| Thread | `threads.max_turns` 200 | V7.5 |
| Schedules | `schedules.max_count` 50 | V7.7 |
| Export ids | at most 200 ids per request | V7.4 |
| Names | `[a-z0-9_-]{1,64}` for templates and schedules | |

### 1.8 Memory (P4)

The router keeps no whole batch, file, history or export in memory beyond one job's text. Known cost to watch: each saved active job has a goroutine (`runLoop`) and each is read from the store as it runs, so a 1000-item batch adds about 1000 small goroutines. Measure it (V7.1 gate): submit 1000 short items against the fake Ollama with `max_concurrent` 1 and read `VmRSS` before and after. If growth is above 10 MB, set the default of `batches.max_items` to 500 and record that in DECISIONS. Never raise the 50 MB budget. Each section names its own load.

### 1.9 Test tooling to add first (V7.1 pull request)

The current fake Ollama (`internal/fakeollama`) returns one scripted reply for every call and does not record requests. Features below need to check what was sent. Before V7.1 tests, extend it (tests only, no production code):

- Record each chat call: model name and the `messages` sent, in order (`Server.Calls()`).
- Allow a reply per call: `Reply func(call Call) []Step`, falling back to `ChatSteps`.
- Optional per-model `parameter_size` in `/api/tags` (used for the V7.2 note).
- A helper that starts two fake servers named `mane` and `firmino` with different models, and a config builder for tests (some of this exists in `internal/api` tests; share it).

### 1.10 Smoke run

A script-free checklist today (TESTING.md section 2.6 says the smoke run is manual). Keep it manual, but write each version's commands in its pull request. Sections below give the commands as `curl` lines against `http://127.0.0.1:8787` with a fake-backed config.

---

## 2. V7.1: split a batch

**Config.** `batches.max_items` (default 1000, minimum 1).

**Request.** `POST /api/batches`:

```json
{"items":[{"prompt":"one"},{"prompt":"two"}],
 "split":{"assign":"weights",
          "targets":[{"machine":"mane","model":"llama3.1:8b","weight":3},
                     {"machine":"firmino","model":"qwen3:30b-a3b-instruct-2507-q4_K_M","weight":2}]}}
```

`split` is optional. Without it the endpoint behaves as today (plus the new item cap and `batches` row).

**Validation** (all before anything is saved, any failure is `BAD_REQUEST` unless named):

- 1 to `batches.max_items` items.
- `assign` is `round_robin` (default) or `weights`.
- 2 or more targets, each on a **different** machine (a repeated machine is `BAD_REQUEST`; two models on one machine would swap models on Firmino). `machine` must exist (`MACHINE_UNKNOWN`). `model` goes through alias expansion, must be installed there (`MODEL_NOT_INSTALLED`, or `MODELS_UNKNOWN` if that machine was never reached). A machine that is down at submit is accepted, as for any pinned job.
- `weight` is an integer 1 to 100, default 1, and is allowed only with `weights` (given with `round_robin` is `BAD_REQUEST`).
- Items must not carry `lane` other than `batch`. Items that carry their own `machine`, `model`, `fast` or `profile` are **not split**: they keep their own routing and are not counted in the rotation.

**Assignment** (fixed at submit, deterministic). Number only the splittable items 0, 1, 2, ... in list order as `k`. Round robin: target `k mod n`. Weights: let `W` be the sum of weights, `slot = k mod W`, and targets own consecutive slot ranges in the listed order (weights 3 and 2: slots 0-2 to the first, 3-4 to the second). The assigned `machine` and `model` are written into each job's saved request as pins, so the existing routing, gate, retry and wait rules apply unchanged (mode `pinned_model_and_machine`). Nothing moves later, a down machine holds its own items (D-071 retry window) while the other carries on.

**Response.** `202`:

```json
{"id":"<batch id>","job_ids":["..."],
 "split":{"assign":"weights","targets":[{"machine":"mane","model":"llama3.1:8b","weight":3,"count":6},
                                        {"machine":"firmino","model":"...","weight":2,"count":4}]},
 "not_split":0}
```

**Batch view.** `GET /api/batches/{id}?limit=200&offset=0` (limit 1 to 1000, default 200, replacing the hard-coded 200):

```json
{"id":"...","created_at":"...","kind":"split","total":10,"pending":7,
 "counts":{"waiting":6,"running":1,"retrying":0,"done":3,"failed":0,"cancelled":0},
 "split":{"assign":"weights","targets":[{"machine":"mane","model":"...","weight":3,
          "total":6,"done":2,"running":1,"waiting":3,"failed":0,"cancelled":0}]},
 "jobs":[ <job detail> ],"next_offset":null}
```

`pending` is waiting plus running plus retrying (as today). Per-target `waiting` includes `retrying`. `split` is null for a batch without a plan. `jobs` are in submission order. `next_offset` is the next offset or null. Old fields (`id`, `pending`, `jobs`) keep their meaning.

**Cancel.** `POST /api/batches/{id}/cancel` cancels every `waiting`, `running` and `retrying` job of the batch (a running one is cancelled upstream, as for a single job), including stage children (V7.2 checks, V7.6 reduce). It also clears `check_pending` and any pending reduce, so no later stage is created. Answers `200 {"cancelled":N}` where N is the number of jobs cancelled. Finished jobs are untouched, so repeating it returns `{"cancelled":0}`. Unknown batch id (no jobs and no row) is `NOT_FOUND`. Works for batches made before V7.1 (by `batch_id`). Needs `storage.data_dir`. Host and Origin checks apply (it is a `POST`). The Batches tab shows a **Cancel batch** button with a confirm. CLI: `pingpong batch cancel --id <batch-id>`. The batch view then shows its jobs as `cancelled` and `pending` 0.

**Batch list.** `GET /api/batches?limit=50&before=<created_at>` returns `[{"id","kind","created_at","total","counts":{...}}]`, newest first, from the `batches` table (batches made before V7.1 and Stress runs are not listed). It needs `storage.data_dir`, like the others.

**CLI.** `pingpong batch submit --file items.json --split mane:MODEL,firmino:MODEL [--assign weights --weights 3,2]`. Split on the first `:` only (models contain `:`). `--weights` needs `--assign weights` and one weight per target.

**Page: Batches tab.** A list of recent batches (kind, created, `done/total`, failed count), a **New batch** form (one prompt per line in a text area, or paste the JSON items; routing selectors; **Split** section with a row per machine for model and weight and an assign select; the lane is always batch), and a batch view with an overall progress bar and one bar per target machine, and the job list (open a job as in the Chat tab history). Later versions add buttons here.

**Memory load.** 1000-item batch, see 1.8.

**Acceptance.**

- [ ] Round robin over 10 items gives 5 and 5. Weights 3 and 2 over 10 items give 6 and 4, and over 7 items follow the slot rule exactly.
- [ ] Every item runs only on its assigned pair (assert on the fake servers' recorded calls). No item ever runs elsewhere, also when one machine is down for the whole test.
- [ ] Down machine: its items go `retrying` (every 2 minutes on the fake clock) while the other machine finishes all its items. Items do not move. When the machine returns they run in submission order.
- [ ] Items with their own `machine`, `model`, `fast` or `profile` keep them and are not counted in the rotation.
- [ ] A bad item, a repeated machine, a missing model or `weight` with `round_robin` creates nothing.
- [ ] Item 1001 (or `max_items` + 1) is `BAD_REQUEST`. A batch of 1000 submits within the memory gate.
- [ ] Interactive jobs still go first on each machine (D-066). `max_concurrent` honoured.
- [ ] Cancel batch: waiting, running (upstream stopped, assert on the fake) and retrying jobs end `cancelled`, finished jobs are untouched, a second call returns 0, unknown id is `NOT_FOUND`, an old batch without a row works, and with V7.2 and V7.6 no stage is created afterwards (and an existing stage child is cancelled).
- [ ] Per-target counts in `GET /api/batches/{id}` match the jobs. Paging works. A pre-V7.1 batch (no row) still loads.
- [ ] Restart with a half-finished split batch resumes without moving items.
- [ ] The split plan is stored in `batches.plan_json` and survives restart.

**Smoke.** Submit a 6-item split batch, `curl` the batch view, check both fake servers received 3 calls each. Submit another and `POST /api/batches/{id}/cancel`.

**Owner on real machines.** Run a 10-item split with the 8B and the 30B and check the Batches tab shows both progressing, then cancel a queued batch with the button. Check Mane's RSS with the batch queued.

---

## 3. V7.2: answer checking

**Request.** A job or batch item gains `check`:

```json
{"prompt":"Explain X","check":{"mode":"review","machine":"firmino","model":"qwen3:30b-a3b-instruct-2507-q4_K_M"}}
```

`mode` required (`review` or `revise`), else `BAD_REQUEST`. `machine`, `model` optional. Optional `options` (the usual allow-list) apply to stage 2 only. Allowed on `POST /api/jobs` and batch items (also with `split`: the split pair is stage 1, stage 2 never follows the split). Not on `/api/chat`. With a `lane` the stage 2 job uses the same lane as stage 1.

**Pairs.**

- Stage 1: the request's own routing. With no routing fields it is resolved as `fast` (Mane, its default model). Because that differs from plain auto, the saved job shows `mode: fast`. This is the documented rule (D-096), not content-based.
- Stage 2: `check.machine` and `check.model` if given. If only `model` is given and it is installed on one machine, that machine. If neither, role `heavy` with its default model (auto role). Resolution uses the normal routing function with `fast` false. Both stage pairs are resolved and validated at submit; a non-retryable routing error rejects the request (nothing saved).
- Overrides are used exactly. If stage 1 and stage 2 end up the same pair, that is allowed.

**Jobs.** Stage 1 is saved with `stage = "answer"` and `check_pending = 1`. When it reaches `done`, the router creates the stage 2 job in the same transaction that sets `check_pending = 0`: `parent_id` = stage 1 id, `stage` = `review` or `revise`, same `batch_id` and lane, status `waiting`, a request built from the fixed text below. The unique index on `(parent_id, stage)` makes creation idempotent. On start, the router creates the stage 2 job for every `done` job with `check_pending = 1` (this is the restart recovery). A stage 1 that ends `failed` or `cancelled` clears `check_pending` and never creates stage 2.

**Fixed texts** (constants in code, not config, never changed without a DECISIONS entry). `{question}` is the saved user prompt, or for a `messages` request every message as `role: content` lines in order, inserted verbatim. `{answer}` is stage 1's saved output, without thinking text.

Review:

```
You are checking another model's answer to a question. Read both.
Start your reply with OK if the answer is correct and complete, or PROBLEMS if it is not.
If PROBLEMS, list each mistake or gap as a short numbered line. Do not rewrite the answer.

QUESTION:
{question}

ANSWER:
{answer}
```

Revise:

```
Below are a question and a draft answer. Write the corrected final answer to the question.
Output only the final answer, with no commentary.

QUESTION:
{question}

DRAFT ANSWER:
{answer}
```

The router does not parse or judge the reply (no verdict field). The page shows the text.

**Context check.** Before creating stage 2: `estimate = stage1.prompt_tokens + stage1.output_tokens + 150`. If stage 1 stats are missing or zero use `(len(question) + len(answer)) / 3 + 150`. If `estimate + 512 > num_ctx` of the stage 2 machine, create the stage 2 job already `failed` with `error_code = CONTEXT_TOO_LARGE`. Heuristic, tune after real use. The reviewer's own `context_warning` is reported as usual.

**Behaviour.**

- Each stage retries on its own pair (D-071). Stage 2 failure leaves stage 1 `done` and its answer intact.
- Cancelling stage 1 while stage 2 is not created: no stage 2. Cancelling either cancels both. Deleting a job deletes its children.
- A restart resumes whichever stage was active (the usual recovery plus the check_pending scan).
- Stats and log lines are per job, with `stage`.
- Batch counts cover item jobs (`stage` empty or `answer`). A batch's `pending` also counts items whose `check_pending` is 1 or whose stage 2 is active. Batch view adds `"checks":{"pending":N,"done":N,"failed":N}`.

**Job detail** gains `stage`, `parent_id`, `children` (ids) and, on a stage 1 job, `"check":{"mode","machine","model","state","child_id"}` where `state` is `pending` (stage 1 not done), `waiting|running|retrying|done|failed|cancelled` (the child's status), or `unchecked` when stage 1 is done and the child failed or was cancelled.

**Page.** A "Check with" select in the chat form (off, review, revise) with machine and model selectors (default heavy, Firmino default) and a Batches tab option for the same. The job view shows two panels, draft and review, each with its machine and model. A small note appears when the reviewer's `parameter_size` (from `/api/machines`) is smaller than the answerer's.

**Memory.** None extra. Load: 200 checked items.

**Acceptance.**

- [ ] Default pairing: stage 1 on Mane default, stage 2 on Firmino default (recorded calls).
- [ ] Stage 2 starts only after stage 1 is `done`, and its prompt contains the fixed text, the question and the answer verbatim (assert on the fake's recorded request).
- [ ] Overrides on either stage are used exactly, never swapped. A bad override rejects the request at submit.
- [ ] Oversize review gives `CONTEXT_TOO_LARGE` on the stage 2 job and leaves stage 1 `done`, `check.state` `unchecked`.
- [ ] Stage 2 failure after retry window, cancel of one cancels both, delete removes both, restart in each of the three moments (stage 1 running, stage 1 done before child exists, child running) ends with exactly one child.
- [ ] A request without `check` behaves as before. `/api/chat` with `check` is `BAD_REQUEST`.
- [ ] Router never reads the answer to decide anything (code review item: no branching on output text).

**Smoke.** One checked job with `review`, read both panels via `GET /api/jobs/{id}`.

**Owner on real machines.** One review and one revise with the real 8B and 30B. Judge whether the review prompt wording is useful. Note time and memory on Firmino with both models used in turn (`OLLAMA_MAX_LOADED_MODELS=1` means a reload).

---

## 4. V7.3: prompt templates

**Config.** `templates.max_bytes` (16384), `templates.max_count` (200).

**Endpoints.**

| Method and path | Body | Result |
|---|---|---|
| `POST /api/templates` | `{"name","body","defaults"?}` | `201` the template. Duplicate name `CONFLICT`. Over the caps `BAD_REQUEST` (size) or `CONFLICT` (count) |
| `GET /api/templates` | | `[{"name","vars":["text","words"],"bytes","defaults","created_at","updated_at"}]`, no bodies |
| `GET /api/templates/{name}` | | The above plus `body`. `NOT_FOUND` if missing |
| `PUT /api/templates/{name}` | `{"body","defaults"?}` | Replace. `NOT_FOUND` if missing. The name cannot change |
| `DELETE /api/templates/{name}` | | `204`. Past jobs are unaffected |

`defaults` may hold `profile`, `fast`, `model`, `machine`, `lane`, `check`, `options`, `think`. The set is validated at save (profile exists, machine exists; model installed is **not** checked at save).

**Syntax.** A blank is exactly `{{name}}`, name `[a-z0-9_]+`, no spaces. Anything else with braces is literal text (`{{ x }}` stays as written). One left-to-right pass over the body, values inserted verbatim and never scanned again. No escape syntax: to get a literal `{{name}}` in the output pass it as a value. `vars` lists each blank once, in order of first use.

**Use.**

- Job: `POST /api/jobs {"template":"summary","vars":{"text":"...","words":"50"}}`. `prompt` and `messages` with `template` is `BAD_REQUEST`. The saved job request holds the final `prompt` and a `template` name for display, so deleting the template never affects it.
- Batch: `POST /api/batches {"template":"summary","vars":{"words":"50"},"rows":[{"text":"a"},{"text":"b"}]}`. `rows` and `items` together is `BAD_REQUEST`. A row's value beats the shared `vars`. One item per row, same cap as items. `split` and `check` work as for items.
- Values must be strings. A missing blank is `TEMPLATE_VARS_MISSING` (names listed). An unused value is ignored and listed in `"warnings":["unused var: foo"]` in the response.
- Explicit request fields beat template defaults (`profile`, `machine`, `model`, `fast`, `lane`, `check`, `options`).

**Storage.** `templates(name TEXT PRIMARY KEY, body TEXT, defaults_json TEXT, created_at TEXT, updated_at TEXT)`.

**Page: Templates tab.** List with New, Edit, Delete (confirm), and a **Use** panel: one input per blank, the routing selectors, **Run** (one job) and **Run as batch** (a text area, one JSON object per line, each a row). Shows the warnings.

**Memory.** A template is read per use, rows expanded one at a time inside the submit transaction.

**Acceptance.**

- [ ] Substitution, repeated blanks, literal braces, values containing `{{other}}` not re-scanned.
- [ ] Missing and extra vars as specified. Name and size caps and the count cap.
- [ ] Defaults overridden by explicit fields. Template with `check` and with `split` works through batch rows.
- [ ] Row values override shared vars. 1000 rows submit atomically. One bad row creates nothing.
- [ ] Delete leaves past jobs readable. Rename is not possible (no endpoint).
- [ ] Template body never appears in a log line.

**Smoke.** Create, list, use as job, use as 3-row batch, delete.

**Owner on real machines.** Make one real template, run it as a 5-row batch, read the results.

---

## 5. V7.4: results export

**Endpoints.**

- `GET /api/batches/{id}/export?format=csv|md[&include_thinking=1][&partial=1][&raw=1][&bom=1]`
- `GET /api/jobs/export?ids=a,b,c&format=csv|md[...]` (at most 200 ids, unknown ids are skipped and counted in header `X-Pingpong-Skipped`).

Missing batch `NOT_FOUND`. Bad `format` `BAD_REQUEST`. Needs `storage.data_dir`. Host and Origin checks apply (it is a `GET`, so only Host).

**Response.** `Content-Type: text/csv; charset=utf-8` or `text/markdown; charset=utf-8`, `Content-Disposition: attachment; filename="pingpong-batch-<first 8 of id>-<YYYYMMDD>.csv"` (`pingpong-jobs-<YYYYMMDD>` for ids), `Cache-Control: no-store`. Written row by row, flushed.

**CSV.** RFC 4180, UTF-8, CRLF line ends, a header row always, fields quoted only when needed. `bom=1` writes a UTF-8 byte order mark first (for spreadsheets that need it). Columns in this order: `index, status, machine, model, lane, stage, prompt, output, output_tokens, output_tokens_per_s, error_code, finished_at` and, with `include_thinking=1`, `thinking` after `output`.

- One row per job. `index` is the 0-based position of the item in the batch (submission order). A stage 2 job has the same `index` as its stage 1 and its own `stage` value. Reduce jobs (V7.6) get `index` empty and stage `reduce`, written last.
- `prompt` is the saved `prompt`, or for a `messages` job the last user message. `output` is the saved output text. `output_tokens` and `output_tokens_per_s` come from the job's last `done` event stats, empty if absent. `finished_at` is the job's last update time for terminal jobs, empty otherwise.
- Default includes terminal jobs only. `partial=1` also writes waiting, running and retrying jobs with their current status and whatever output is saved so far.
- **Formula guard.** Any text cell (`status` through `error_code`, except numbers) whose first character is `=`, `+`, `-`, `@`, tab or carriage return gets a leading `'`. `raw=1` turns this off.

**Markdown.** One section per job: `## Item <index>` (with ` (<stage>)` for stages), a line with status, machine, model, tokens per second, a `### Prompt` block and an `### Output` block, each inside a code fence whose length is one more than the longest run of backticks in the text, at least 3. Thinking goes under `### Thinking` only with `include_thinking=1`. No formula guard.

**Errors.** If the store fails after the header was sent, abort the response (close the connection so the client sees a truncated download) and log `export_done` with `outcome=error`. Never write an error message into the file body.

**Data.** None stored. Read the job rows with a cursor, one at a time, ordered by `created_at` then `id`.

**Page.** On a batch in the Batches tab: **CSV** and **Markdown** buttons, a checkbox for thinking and one for partial. The files hold prompts and outputs: say so in the button tooltip.

**Memory.** One row at a time. Load: a 1000-item batch with 8 KB outputs. RSS growth under 5 MB.

**Acceptance.**

- [ ] CSV round-trips commas, quotes, newlines and non-ASCII through a standard CSV reader. CRLF endings. BOM only on `bom=1`.
- [ ] Formula guard on each of the six leading characters, off with `raw=1`.
- [ ] Markdown fence is longer than any backtick run in the content.
- [ ] Default excludes unfinished jobs. `partial=1` includes them. Thinking only on request.
- [ ] Split and checked batches show machine, model, stage per row.
- [ ] Mid-stream store error aborts the response. The log has `export_done` and no content.
- [ ] Memory gate met.

**Smoke.** Export a 3-item batch as CSV and Markdown with `curl -o`, open the CSV.

**Owner on real machines.** Download the CSV of a real 20-item batch and open it in the spreadsheet you use. Check accented text and one cell starting with `=`.

---

## 6. V7.5: conversation threads

**Config.** `threads.max_turns` (200).

**Endpoints.**

| Method and path | Body and result |
|---|---|
| `POST /api/threads` | `{"title"?,"machine"?,"model"?,"profile"?,"fast"?}`. Routing is resolved **now** with the normal rules (so no fields means auto, Firmino's current default) and the resulting machine and model are stored on the thread (P10). A routing error is returned and nothing is created. `201` `{"id","title","machine","model","turns":0,"created_at","updated_at"}`. Title default: first 40 characters of the first message, set when it arrives |
| `GET /api/threads?limit=50&before=<updated_at>` | `[{"id","title","machine","model","turns","updated_at","active_job_id"}]` newest activity first |
| `GET /api/threads/{id}?limit=50` | The thread plus `"turns":[{"turn","job_id","status","prompt","output","machine","model","created_at"}]` (last `limit` turns, oldest first, thinking excluded) and `"pin_changes":[{"after_turn","machine","model","at"}]` |
| `POST /api/threads/{id}/messages` | `{"content","trim"?,"options"?,"think"?,"check"?}` returns `202` `{"job_id","turn","dropped_turns"}`. The turn is an ordinary job (same events at `GET /api/jobs/{id}/events`, same cancel, retries). `check` is optional and uses the same `review` or `revise` object as a one-shot job (D-122) |
| `PATCH /api/threads/{id}` | `{"title"}` and/or `{"machine","model"}` (both required together, validated as routing). A pin change is recorded in `pin_changes` and affects later turns only. `THREAD_BUSY` while a turn is active |
| `DELETE /api/threads/{id}` | `204`. Deletes the thread and its turn jobs. `THREAD_BUSY` while a turn is active |

**Turns.** Each message creates a job with `thread_id`, `turn` (1-based, next number), lane `interactive`, pinned to the thread's pair. Its saved request holds the **full** `messages` list sent to Ollama (history plus the new user message), so the existing run path, retries and restart recovery need no change. Storage grows with turns squared; with 200 turns and an 8192-token window that stays within a few MB per thread. Only `done` turns are history: for each, the user prompt then the assistant output (thinking excluded), in turn order. Failed and cancelled turns are skipped. A second message while a turn is `waiting`, `running` or `retrying` is `THREAD_BUSY`.

**Length rule.** `estimate = (last done turn's prompt_tokens + output_tokens, or len(all history text)/3 if missing) + len(content)/3`. If `estimate + 512 > num_ctx` of the thread's machine, or turns reached `threads.max_turns`, respond `409 THREAD_TOO_LONG` with `estimated_tokens` and `limit`. With `"trim":"oldest"` drop the oldest done turns, whole turns, until it fits and report `dropped_turns`. Trim is per message, not remembered. Never silent.

**Retention.** Jobs with a `thread_id` are not swept on their own. A thread whose `updated_at` (last message) is older than `retention_days` (90) is deleted with all its jobs by the same daily sweep (D-103).

**Storage.** `threads(id TEXT PRIMARY KEY, title TEXT, machine TEXT, model TEXT, pin_changes_json TEXT, created_at TEXT, updated_at TEXT)`; `jobs.thread_id`, `jobs.turn`.

**Page: Threads tab.** A list on the left (title, model, last activity), a conversation view on the right showing each turn with its machine and model, a message box, a "trim oldest if too long" checkbox, a pin-change control, rename and delete (with confirm). A running turn streams through the existing job events. The Chat tab stays for unsaved chat.

**Memory.** History is built in a job-sized buffer only when a turn is created. Load: a 200-turn thread, add a message.

**Acceptance.**

- [x] History order, exclusion of failed and cancelled turns, thinking excluded (assert on the fake's recorded `messages`).
- [x] Pair stored at creation and used for every turn even if the config default changes. Explicit pin change recorded and applied from the next turn only.
- [x] `THREAD_TOO_LONG` with numbers, `trim` drops oldest whole turns and reports them, turn cap reached gives `THREAD_TOO_LONG`.
- [x] Parallel message, patch or delete while active gives `THREAD_BUSY`.
- [x] Thread turn jobs survive the job sweep at 91 days while the thread is active, and the whole thread is removed 91 days after the last message (fake clock).
- [x] Restart with an active turn resumes it. Cancel leaves history clean.
- [x] Delete removes thread and jobs.

**Smoke.** Create a thread, send three messages, `GET` the thread.

**Owner on real machines.** Hold a 10-turn conversation with the 30B, close the laptop mid-answer, reopen from the phone and check history. Time how long turn 10 takes to start (prompt reading grows with history).

---

## 7. V7.6: file input (text and Markdown)

**Config.** `files.max_bytes` (2097152), `files.chunk_chars` (6000).

**Request.** `POST /api/batches/from-file`, `multipart/form-data`:

| Field | Meaning |
|---|---|
| `file` | Required. `.txt`, `.md` or `.markdown` (by name, case-insensitive), valid UTF-8, no NUL bytes |
| `instruction` | A prompt containing `{{chunk}}`, and optionally `{{index}}` (1-based) and `{{total}}`. Either this or `template` is required |
| `template` | A V7.3 template that contains `{{chunk}}`. Other blanks come from `vars` (a JSON object string) |
| `machine`, `model`, `profile`, `fast` | Routing, as for any job. One of them or the default auto |
| `split` | Optional JSON string, as V7.1 |
| `chunk_chars` | Optional, 500 to 20000, default `files.chunk_chars` |
| `reduce` | Optional JSON string `{"instruction":"... {{results}} ...","machine":"...","model":"..."}`. `machine` and `model` required (P10) |
| `dry_run` | `1` returns the chunk count and creates nothing |

**Limits.** The request body is capped at `files.max_bytes` plus 64 KB for this endpoint only (independent of `router.max_body_bytes`). Over it: `FILE_TOO_LARGE` (413), detected from `Content-Length` and by a `MaxBytesReader`. Wrong extension, invalid UTF-8 or NUL: `FILE_TYPE_UNSUPPORTED` (415). No text after trimming: `BAD_REQUEST`. More chunks than `batches.max_items`: `BAD_REQUEST` ("too many chunks, raise chunk_chars").

**Chunking** (deterministic, on runes). Normalise line endings to `\n`. Paragraphs are separated by one or more blank lines. Add paragraphs in order, separated by a blank line, to the current chunk while it stays within `chunk_chars`. A paragraph longer than `chunk_chars` is cut at the last sentence end (`. `, `? `, `! ` or a newline) at or before the limit, else at the limit on a rune boundary. No overlap. Whitespace-only chunks are dropped. Markdown fenced blocks get no special treatment.

**Prompt per chunk.** The instruction (or template body) with `{{chunk}}`, `{{index}}`, `{{total}}` substituted by the V7.3 function (the chunk text is never re-scanned). Instruction without `{{chunk}}` is `BAD_REQUEST`. Chunks are saved as the job prompts, in order. The file is not kept.

**Streaming and atomicity.** Read the upload with `MultipartReader`, chunk as it streams, insert each chunk job inside one transaction, roll back on any error, and start the run loops after commit. Memory is one chunk plus the transaction. Response `202`: `{"id","job_ids","chunks":N,"reduce_job_id":null}`. `dry_run`: `200 {"chunks":N,"chars":M}`.

**Reduce.** If `reduce` is given, the batch row stores `reduce_json` and the reduce job is created when every chunk job is terminal, by the same hook as V7.2 stages: `stage = "reduce"`, `batch_id` set, lane batch, pinned to the given pair. Its prompt is the reduce instruction with `{{results}}` replaced by `### Part N` followed by that chunk's output, in order. A chunk that did not finish is written as `### Part N` and `[no result: <status>]`. If at least one chunk is `done` the reduce is created, if none is, it is not created and the batch view says so. The context rule of V7.2 applies: `CONTEXT_TOO_LARGE` on the reduce job. On start the router re-checks batches whose `reduce_json` is set and `reduce_job_id` is empty and all chunk jobs are terminal. The batch view gets `"reduce":{"job_id","status"}`.

**Failure.** One failed chunk does not stop the others. Failed chunks use the normal retry window.

**Page.** In the Batches tab's New batch form: a **From file** mode with a drop zone, the instruction box (or template select), routing and split controls, a **Reduce** section (instruction, machine, model), a **Preview** button (runs `dry_run`, shows the chunk count) and **Start**. The batch view shows the reduce job last.

**Not in the first cut.** PDF (D-098). A `.pdf` upload gives `FILE_TYPE_UNSUPPORTED`. Embeddings, retrieval, an index, OCR, images, Word files, keeping the file.

**Memory.** Load: a 2 MB file with 6000-character chunks (about 350 chunks). RSS growth under 8 MB.

**Acceptance.**

- [ ] Chunk sizes never exceed `chunk_chars`, order preserved, paragraph boundaries respected, a 20000-character paragraph is cut at sentence ends, multibyte text is never split inside a character.
- [ ] `FILE_TOO_LARGE`, `FILE_TYPE_UNSUPPORTED` (including `.pdf`, bad UTF-8, NUL) and the chunk cap behave as specified, creating nothing.
- [ ] `dry_run` creates nothing and reports the same chunk count as a real submit.
- [ ] Instruction without `{{chunk}}` rejected. Template with other blanks needs `vars`.
- [ ] Split works with file batches. One failed chunk leaves the rest running.
- [ ] Reduce: created once after all chunks are terminal (also after a restart in between), results in order, refusal path `CONTEXT_TOO_LARGE`, reduce pair used exactly.
- [ ] The uploaded file is not stored anywhere (no file under `data_dir` other than the database).
- [ ] Memory gate met and no new dependency was added.

**Smoke.** Upload a 20 KB text with `curl -F`, `dry_run` first, then real, then fetch the batch.

**Owner on real machines.** Upload a real report of about 100 KB, summarise each section, and with reduce. Say what chunk size suits your documents (the PRD's open question), and whether reduce fits Firmino's 8192 window.

---

## 8. V7.7: scheduled jobs

**Config.** `schedules.max_count` (50), `schedules.grace` (`1h`).

**Endpoints.**

| Method and path | Result |
|---|---|
| `GET /api/schedules` | `[{"id","name","rule","machine","model","enabled","missed","overlap","next_due","last_due","last_state","created_at"}]` (no prompt text) |
| `GET /api/schedules/{id}` | Full record including `template` or `prompt`, `vars`, `rows` |
| `POST /api/schedules` | Create, `201` the full record |
| `PATCH /api/schedules/{id}` | Any field, including `enabled`. Applies from the next due time |
| `DELETE /api/schedules/{id}` | `204`. Deletes the schedule and its run records. Jobs and batches it made stay until retention |
| `POST /api/schedules/{id}/run` | Run now. `202` with `run_id` and `job_id` (single job) or `batch_id` (rows) |
| `GET /api/schedules/{id}/runs?limit=50` | A list, newest first, of `id`, `due`, `started`, `kind` (`job` or `batch`), `job_id` or `batch_id`, `status`, `error_code`, `state` |

**Create body.**

```json
{"name":"nightly-notes",
 "rule":{"every":"day","at":"02:00"},
 "template":"notes-summary","vars":{"words":"50"},"rows":[{"text":"..."}],
 "machine":"firmino","model":"qwen3:30b-a3b-instruct-2507-q4_K_M",
 "missed":"skip","overlap":"skip","enabled":true}
```

- `name` unique, `[a-z0-9_-]{1,64}`. Duplicate is `CONFLICT`. Over `schedules.max_count` is `CONFLICT`.
- `rule`: `{"every":"day","at":"HH:MM"}` or `{"every":"week","at":"HH:MM","weekday":"mon|tue|wed|thu|fri|sat|sun"}`. 24-hour local time of Mane (`time.Local`). Anything else `BAD_REQUEST`.
- Exactly one of `template` (must exist, `TEMPLATE_NOT_FOUND`) or `prompt`. `vars` as V7.3. `rows` optional (needs a template): with rows the run is a batch (`kind` `schedule`), otherwise a single job. Rows over `batches.max_items` is `BAD_REQUEST`.
- `machine` and `model` required, machine must exist, model goes through alias expansion. Installed-ness is checked when the run starts, not at create. `check` and `options` are not supported on schedules in the first cut (use a template's defaults, whose `check` is honoured).
- `missed`: `skip` (default) or `run_once`. `overlap`: `skip` (default) or `allow`.
- Runs are submitted on the **batch** lane, pinned to the saved pair. The template is read at run time, so editing a template changes the next run; editing a schedule changes it from the next due time.

**Timer.** One goroutine and one timer for all schedules, using `internal/clock`. After any change or run it sleeps until the earliest `next_due`. Never polls faster than once a minute. `next_due` is computed from `last_due` (or `created_at` for a new schedule, so nothing runs at once).

**Due handling.**

1. At each due time, create a `schedule_runs` row (`due`, `started`).
2. `overlap: skip` and the previous run's job or batch is not terminal: status `skipped_overlap`, nothing submitted.
3. Otherwise resolve the pair with the normal routing. A non-retryable error (`MODEL_NOT_INSTALLED`, `MACHINE_UNKNOWN`, `TEMPLATE_NOT_FOUND`, `TEMPLATE_VARS_MISSING`) gives status `failed` with `error_code` and no job. A retryable one (machine down) is submitted, and the normal retry window applies from the job's own first failure (the due time does not extend it).
4. Otherwise status `submitted` with the job or batch id.
5. Set `last_due`, recompute `next_due`.

`state` in the runs list is derived when read: for a job its status, for a batch `done` if all jobs are done, `failed` if all are terminal and any failed or was cancelled, else `running`. Runs that never submitted show their `status` as `state`.

**Start-up and missed runs.** On start, for each enabled schedule take the first due time after `last_due`. If it is in the past: `missed: run_once` and now is within `schedules.grace` of it runs once now; otherwise record one run `skipped_missed`. Several missed times collapse into one record. Either way `last_due` moves to the latest past due time and `next_due` to the next future one. A restart never double-runs: `last_due` is saved before the job is submitted (write it in the same transaction that creates the run row).

**Run now.** Ignores `overlap`, does not change `last_due` or `next_due`, creates a run row with `due` = now.

**Pause and resume.** `enabled: false` stops due handling and clears `next_due`. Resuming recomputes from now, with no catch-up.

**Daylight saving.** Compute each due time with `time.Date(y, m, d, H, M, 0, 0, time.Local)`. If that wall time does not exist (spring forward), run at the instant `time.Date` returns for it, once. If it happens twice (fall back), run at the first one only. Weekly rules use the local weekday. Test with a fixed named zone (import `time/tzdata` in the test file only).

**Storage.** `schedules(id, name UNIQUE, rule_json, template, prompt, vars_json, rows_json, machine, model, missed, overlap, enabled INTEGER, last_due, created_at, updated_at)`, `schedule_runs(id, schedule_id, due, started, kind, job_id, batch_id, status, error_code)` with an index on `(schedule_id, due)`. Prompts live here, never logged.

**Page: Schedules tab.** As in the PRD: a table (name, when, target, next run, last state, enabled toggle) with **New**, **Edit**, **Pause/Resume**, **Run now**, **Runs** and **Delete** (confirm). The form has the fields above. A banner states that anyone on the tailnet can change schedules (D-102).

**Memory.** One timer. Load: 50 schedules.

**Acceptance.**

- [ ] Next-due maths for daily and weekly rules, including both daylight-saving cases and a weekday rule.
- [ ] `missed: skip` and `run_once` (inside and outside grace) on restart, many missed collapse to one record, and a restart right after a run does not double-run.
- [ ] `overlap: skip` skips while the last batch is active, `allow` does not. Run now ignores overlap and leaves `next_due`.
- [ ] Create, patch, pause, resume, delete via API with validation errors for every rule above. Cap of 50.
- [ ] The saved pair is always used. A machine down at the due time gives a retrying job that fails after 3 hours from its own first failure, and the run shows `failed`. A missing model gives a `failed` run with `MODEL_NOT_INSTALLED` and no job.
- [ ] Schedules survive restart. Deleting removes runs only.
- [ ] Log line `schedule_run` has the name and outcome and no content.

**Smoke.** Create a schedule due in 1 minute (a test config can allow it by creating at 00:00 on a fake clock; for the real binary create it for the next minute), watch `runs`.

**Owner on real machines.** One nightly schedule for a week. Check one run at the due time, one after a Mane restart, and one run-now. Check that Mane's time zone is what you expect.

---

## 9. V7.8: OpenAI-compatible endpoint

Approved by the owner (D-116). Opt-in: `openai.enabled` (default `false`). When off the paths return `404 NOT_FOUND`.

**Endpoints.**

| Path | Behaviour |
|---|---|
| `GET /v1/models` | `{"object":"list","data":[{"id":"...","object":"model","created":0,"owned_by":"pingpong"}]}`. Ids: every installed model name (deduplicated), every alias, `pingpong-auto`, `pingpong-fast` |
| `POST /v1/chat/completions` | Described below |

**Model names.** `pingpong-auto` is auto routing, `pingpong-fast` is the fast flag. `<machine>/<model>` pins both (the prefix counts only if it is a registered machine name). Any other value is a pinned model, aliases allowed. So the client chooses, the router never decides from content. Ambiguous or missing models return the usual errors in the OpenAI error shape.

**Request.** Fields used: `model`, `messages` (content is a string, or an array of `{"type":"text","text"}` parts joined with no separator), `stream` (default `false`), `temperature`, `top_p`, `max_tokens` (becomes `num_predict`), `stop`, `seed`. Ignored: `user`, `presence_penalty`, `frequency_penalty`. `BAD_REQUEST` (400): `n` other than 1, `tools`, `functions`, `response_format`, `logprobs`, image parts, an unknown role. The `Authorization` header is accepted and ignored (clients need a dummy key). Host and Origin checks apply.

**Response.** Non-stream: `{"id":"chatcmpl-<id>","object":"chat.completion","created":<unix>,"model":"<resolved model>","choices":[{"index":0,"message":{"role":"assistant","content":"..."},"finish_reason":"stop|length"}],"usage":{"prompt_tokens":N,"completion_tokens":N,"total_tokens":N},"pingpong":{"machine":"...","mode":"..."}}`. Stream (SSE, `Content-Type: text/event-stream`): `data: {...chat.completion.chunk...}\n\n` with `delta.content`, a last chunk with `finish_reason`, then `data: [DONE]\n\n`. While waiting, send `: ping\n\n` every `heartbeat_interval`. Thinking text is dropped, not mixed into `content`.

**Errors.** `{"error":{"message":"...","type":"invalid_request_error|server_error","param":null,"code":"<router code>"}}` with the router's HTTP status. 4xx is `invalid_request_error`, 5xx `server_error`.

**Behaviour.** Same engine and wait rules as `POST /api/chat`: connection-tied, disconnect cancels (D-064), no saved record, no retry. State this in the docs: a long wait on Firmino may exceed the client's own timeout. No new storage, no new dependency, no log content.

**Acceptance.** [x] SSE framing and `[DONE]`. [x] Non-stream shape and `usage` from Ollama stats. [x] Each rejected field. [x] `machine/model` pin, alias, `pingpong-fast`. [x] Disconnect cancels upstream. [x] Disabled by default returns 404. [ ] The official `curl` examples of the OpenAI docs work with only the base URL and a dummy key changed (owner on real machines).

**Owner on real machines.** Point one OpenAI-compatible client at `https://MANE_TAILNET_NAME/v1` and chat.

---

## 10. V7.9: notifications

Approved by the owner (D-116). The constitution was changed for it (P8 and the non-goal, D-117), so nothing blocks the build.

**Config** (the URL is private and lives only in the real config file, never committed, never logged, never shown in the page):

```yaml
notify:
  webhook_url: ""        # placeholder in the example, empty means off
  events: [batch_done, schedule_failed, job_failed]
  timeout: 10s
```

**Behaviour.** When an event happens the router makes one HTTP `POST` to `webhook_url`: body is a short plain-text line, headers `Title: Pingpong` and `X-Pingpong-Event: <event>`. That form works with ntfy and with generic receivers. One attempt, no retry, `timeout` applies, a failure is logged as `notify_failed` (`error_code`) and never affects the job. Redirects are not followed. Scheme must be `http` or `https`.

| Event | When | Body text (no content) |
|---|---|---|
| `batch_done` | Every item job of a batch is terminal (including reduce and checks) | `Batch <id8> finished: N done, M failed` |
| `schedule_failed` | A schedule run has status `failed`, or its batch ends with failures | `Schedule <name> run failed (<error_code>)` |
| `job_failed` | A job outside any batch, or a stage job, ends `failed` | `Job <id8> failed (<error_code>)` |

No prompts, outputs, file text or template text in any message. The schedule name is allowed. `pingpong doctor` validates the URL shape and warns if set but invalid. It never sends. Memory: none.

**Acceptance.** [ ] Each event fires once. [ ] Fake receiver gets the exact headers and a body without content. [ ] Failure, timeout, 500 and redirect do not change job state and are logged without the URL. [ ] Empty URL means nothing is sent. [ ] Restart does not resend (events are in-memory and a restart may lose one, which is accepted).

**Owner on real machines.** Send a test by finishing one small batch and check the phone.

---

## 11. Not built, on purpose

`when_free` split (needs a P10 sentence, D-105), PDF (D-098), a login layer, notification providers other than a webhook, search across threads, template versions, calendar schedules and cron syntax, schedules in the config file, export of the whole history.

## 12. Owner decisions (answered, D-116)

| # | Question | Answer |
|---|---|---|
| Q1 | Build V7.8 as specified (opt-in `openai.enabled`, `machine/model` naming, key ignored, connection-tied, no tools) | Yes |
| Q2 | Build V7.9 as one outbound webhook, public service such as ntfy.sh with a long random topic acceptable | Yes |
| Q3 | Constitution wording for notifications (P8 and the non-goal) | Approved and applied to CONSTITUTION.md |
| Q4 | Add `POST /api/batches/{id}/cancel` to V7.1 | Yes, specified in section 2 |
| Q5 | `batches.max_items` default 1000, fall back to 500 if the memory gate in 1.8 fails | Yes |

No open questions remain for V7.1 to V7.9.
