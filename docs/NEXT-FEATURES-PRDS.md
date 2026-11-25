# Next features: PRDs for V7.1 to V7.7

Status: **planned, owner-approved as roadmap items, not built.** Nothing here is
built until the owner says so (AGENTS.md). Versions and order are in
[ROADMAP.md](ROADMAP.md). Decisions: D-086 to D-100.
Versioning: minor steps after the last built version (V7.1, V7.2, ...). A new
major only for a large change (D-095).

Every feature below inherits these rules, so they are not repeated:

| Rule | Meaning here |
|---|---|
| P4, 50 MB budget | Work is streamed or row-at-a-time. No feature holds a whole batch, file or export in memory. Each feature's memory note says how. Measure router RSS before calling it done |
| P9, P10, D-059 | Routing never reads prompt content. A pinned model or machine is used exactly, never substituted. A down machine means wait and retry, never reroute |
| P11 | Every produced answer shows its machine and model. Multi-step features show each step |
| P14 | Everything runs as saved jobs, so it survives the client closing |
| P16 | Content only in the saved-request store. Logs get metadata only (D-076). New log fields go through the `logx` whitelist |
| Retention | Results and thread content: 90 days. Templates and schedules are definitions, kept until deleted |
| Storage | All of it needs `storage.data_dir`. Schema changes are additive SQLite migrations |
| Out of scope for all | Logins, multi-user, notifications (V7.9), OpenAI endpoint (V7.8), RAG |

Build order: **V7.1, V7.2, V7.3, V7.4, V7.5, V7.6, V7.7.**

---

## V7.1: Split a batch across both machines

**Goal.** Finish a batch sooner by running it on Mane and Firmino at once, each
with its own pinned model.

**Stories.** As the owner I submit 100 items and say "Mane runs `llama3.1:8b`,
Firmino runs the 30B", and both machines work until the batch is done. I can see
which machine and model produced each item.

**In scope.** A `split` plan on `POST /api/batches`. Per-machine progress.
**Out of scope.** Splitting one request across machines. Choosing machines by
item content. Picking models for the user. Changing models mid-batch.

**Behaviour.** The user gives a list of targets, each an explicit
`{machine, model}` pair, and an `assign` rule, **fixed at submit time**:

| `assign` | Meaning |
|---|---|
| `round_robin` (default) | Item i goes to target i mod n |
| `weights` | Targets get a share by user-set integer weights, for example 3 and 2 |

A down machine holds its own items, which retry on it (D-071 window) while the
other machine carries on. Nothing is moved. Every item records its pair (P11).
Items with a pinned `machine` or `model` of their own are not split. Interactive
requests still go first (D-066).

**Future note (not in this version).** A `when_free` mode, where an item takes
whichever machine is free at admission and is then bound to it, would balance
speed differences better. It needs an owner wording change on P10 first, so it is
left out (D-097).

**API.** `POST /api/batches` gains `"split":{"assign":"round_robin|weights","targets":[{"machine","model","weight"}]}`
and items need no routing fields. `GET /api/batches/{id}` gains per-target
`done`, `running`, `waiting`, `failed`. Page: a "Split" option in the batch form
and a per-machine progress bar. `pingpong batch submit --split mane:MODEL,firmino:MODEL`.

**Config.** `batches.max_items` (default 1000, a safety cap).

**Data.** Nothing new. Each job already stores machine and model. The batch row
stores the split plan.

**Memory.** Assignment is computed per item at submit or admission. No new
buffers.

**Tests.** Round-robin and weights counts. Each item runs on its assigned pair
only. Down machine: its items wait and the other finishes, none move. Pinned
items are skipped by the splitter. Lanes still honoured.

**Guardrails.** P10 and D-059 (wait, do not reroute). The split plan is an
explicit user-chosen mode (proposed P9 wording, "Proposed constitution changes"
item 1).

**Open questions.** Should the export mark mixed models with columns for machine
and model (default: yes)?

---

## V7.2: Answer checking (review or revise by a second model)

**Goal.** Run a request on one model, then have a second, deeper model check it.

**Stories.** I ask the fast model for a draft, and the 30B reviews it and lists
mistakes. Or I ask for a final answer and the 30B rewrites the draft. I see both
steps and which model did each.

**In scope.** An explicit `check` mode on a request or batch item.
**Out of scope.** Choosing the reviewer automatically. Deciding whether to check
based on the prompt or the answer. Loops of more than one review. Voting among
several models.

**Behaviour.** Turning on `check` is the user's explicit choice (P9, P10). The
default pairing is **Mane drafts (fast), Firmino reviews (deep)**. A fixed,
documented rule, not content-based:

- Stage 1 uses the request's own routing fields. If it has none, it runs as
  `fast`: Mane with its default model.
- Stage 2 uses `check.machine` and `check.model`. If omitted, it runs as the
  `heavy` role: Firmino with its default model.
- The user can override either per job: pin stage 1 with the usual fields
  (`machine`, `model`, `profile`), and stage 2 with `check.machine` and
  `check.model`. The pairs are then used exactly (P10).

Two stages, saved as two linked jobs (`parent_id`, `stage`):

1. `answer`: the request, as above.
2. `review`, run only after stage 1 is `done`. `mode`:
   - `review`: output is a verdict (`ok` or `problems`) and a short list of
     issues. The original answer is kept unchanged as the result.
   - `revise`: output is a corrected final answer. The draft is kept too.

The review prompt is a fixed text from the docs plus the original question and
the answer inserted verbatim. The router does not read or judge either. If the
combined text does not fit `num_ctx`, the review stage fails with
`CONTEXT_TOO_LARGE`. It is never truncated silently. Each stage retries
on its own pair (D-071). If stage 2 fails, the result shows the answer as
`unchecked` and the error. Stats and the log line are per stage.

**API.** `POST /api/jobs` and batch items gain `"check":{"mode":"review|revise","machine","model"}`
(`mode` required, `machine` and `model` optional, defaults above). Job detail gains `stage`, `parent_id`, `children`. Page: a
"Check with" selector, and the result shows two panels (draft, review).
`pingpong` batch items take the same field.

**Config.** None.

**Data.** Two job rows per checked request, linked. A `stage` and `parent_id`
column. Doubles storage and run time for checked requests.

**Memory.** None extra. Stages are ordinary jobs.

**Tests.** Stage 2 starts only after stage 1 is `done`. Default pairing is
Mane then Firmino. Overrides are honoured exactly and never swapped. `CONTEXT_TOO_LARGE` on oversize. Stage 2 failure keeps stage 1
visible and `unchecked`. Cancel cancels both. Restart recovery resumes the right
stage. A request without `check` is unchanged.

**Guardrails.** P9 and P10: reviewer is user-chosen, there is no content-based
decision. P11: both models shown. P16: the review prompt holds content, so it is
only stored, never logged. Proposed P10 wording: "Proposed constitution changes" item 2.

**Open questions.** If the user overrides so the reviewer is smaller than the
answerer, should the page show a note (proposed: yes)? Should `revise` be
allowed its own `num_predict`?

---

## V7.3: Prompt templates

**Goal.** Save prompts with blanks and reuse them.

**Stories.** I save "Summarise {{text}} in {{words}} words", fill the blanks, and
send it, or run it over many rows as a batch.

**In scope.** Named templates with `{{name}}` blanks and optional default routing
fields. Use from the page, API and terminal.
**Out of scope.** Logic, loops, conditions, nested templates, choosing a
template by content, sharing between users.

**Behaviour.** Plain text replacement. A name is `[a-z0-9_]+`. Values are
inserted verbatim and never re-scanned (no injection into other blanks). A
missing blank is an error (`TEMPLATE_VARS_MISSING`), an unused value is ignored
with a warning in the response. A template may carry defaults for `profile`,
`fast`, `model`, `machine`, `lane`, `check`. Explicit request fields win. Using a
template with `rows` creates one batch item per row.

**API.** `POST/GET/PUT/DELETE /api/templates`. `POST /api/jobs` and
`/api/batches` gain `"template":"name"`, `"vars":{...}`, and for batches
`"rows":[{...}]`. Page: a Templates tab (list, edit, "use" form).
**Config.** `templates.max_bytes` (default 16 KB per template), `templates.max_count` (default 200).
**Data.** A `templates` table (name, body, defaults, timestamps). Stored content,
so never logged. Not subject to the 90-day sweep.
**Memory.** Templates are read per use. Rows are expanded one at a time.
**Tests.** Substitution, missing and extra vars, no re-scan, size caps, defaults
overridden by explicit fields, batch expansion, delete does not affect past jobs
(they store the final prompt).
**Guardrails.** P9: substitution does not read content for routing. P16.
**Open questions.** Allow `{{` escaping? Keep a version history of edits (default: no)?

---

## V7.4: Results export

**Goal.** Download a finished batch as CSV or Markdown.

**Stories.** I run 200 items overnight, then download one CSV and open it in a
spreadsheet.

**In scope.** Export of one batch, or of selected jobs by id. **Out of scope.**
Other formats, email or cloud upload, exporting the whole history, scheduled
exports.

**Behaviour.** `format=csv` (RFC 4180, UTF-8) or `md` (one section per item).
Columns: `index, status, machine, model, lane, stage, prompt, output,
output_tokens, output_tokens_per_s, error_code, finished_at`. Thinking text is
excluded unless `include_thinking=1`. Only terminal-state items by default.
`partial=1` includes the rest as `waiting` or `running` rows. Cells beginning with `=`,
`+`, `-` or `@` get a leading `'` so spreadsheets do not run them as formulas.
`raw=1` turns that off. The file is written row by row to the response.

**API.** `GET /api/batches/{id}/export?format=csv|md[&include_thinking=1][&partial=1][&raw=1]`,
`GET /api/jobs/export?ids=a,b,c&format=...`. Sent with `Content-Disposition:
attachment`. Page: Download buttons on a batch. Terminal: `curl -o`.
**Config.** None. **Data.** None stored. The download holds prompts and outputs,
so it is private data. It is not a log.
**Memory.** One row in memory at a time.
**Tests.** CSV quoting with commas, quotes and newlines. Formula guard and `raw`.
Markdown structure. `partial`. Large batch streams without growth in RSS. Host and Origin checks apply.
**Guardrails.** P16 (an export is not a log), P17 (no sample content in the repo).
**Open questions.** Spreadsheet-friendly BOM option? Include the split pair columns by default (yes)?

---

## V7.5: Conversation threads

**Goal.** Group requests into conversations that keep their history and survive
the client.

**Stories.** I start a thread with the 30B, ask follow-ups over days, and reopen
it from my phone with the history intact.

**In scope.** Create, list, open, continue, rename, delete a thread. A thread is
bound to one model and machine. **Out of scope.** Sharing, branching, search
across threads, automatic summaries, automatic model switching.

**Behaviour.** Each user turn is an ordinary durable job with a `thread_id`, so
retries, cancel and history already work. The server builds `messages` from the
earlier `done` turns plus the new one. The thread pins `machine` and `model` at its
first turn (P10). Changing them is an explicit user action, recorded in the
thread as a marker, and applies to later turns only. If history plus the new
turn would fill `num_ctx` (estimated from the last turn's reported token counts
plus the new text length), the turn is refused with `THREAD_TOO_LONG`. The user
can start a new thread or opt in to `"trim":"oldest"`, which drops the oldest turns and
says so in the response. Never silent. A failed turn is not part of history.

**API.** `POST /api/threads`, `GET /api/threads`, `GET /api/threads/{id}`,
`POST /api/threads/{id}/messages` (returns a job id, same events as jobs),
`PATCH` (rename, change pin), `DELETE`. Page: a thread list and the chat view
reading from the server instead of the browser (the browser-held history of V1
remains for unsaved chat).
**Config.** `threads.max_turns` (default 200).
**Data.** A `threads` table and a `thread_id` and `turn` on jobs. Retention: 90
days from last activity, then removed.
**Memory.** History is loaded only when a turn is built, and is bounded by
`num_ctx`.
**Tests.** History order and exclusion of failed turns. Pin kept across turns.
Explicit pin change. `THREAD_TOO_LONG` and `trim`. Parallel turns in one thread
are refused (one active turn). Retention sweep. Restart recovery.
**Guardrails.** P10 (no silent switching), P16, P14.
**Open questions.** Retention from last activity, or from creation? Turn on `trim` by
default (proposed: no)? Replace the browser-held chat or keep both?

---

## V7.6: File and document input (not RAG)

**Goal.** Drop in a text or Markdown file, split it into chunks, and process every
chunk as a batch.

**Stories.** I upload a long report, ask "summarise each section in 3 bullets",
and read the results in order, or export them.

**In scope.** Text and Markdown files. Chunking. One batch job per
chunk, same instruction. Optional final `reduce` step. **Out of scope.**
**PDF (later, see below)**. Embeddings, vector search, retrieval, an index, OCR, images, Word files, keeping
the uploaded file, answering questions over the whole document. (That would be
RAG, a non-goal.)

**Behaviour.** The file is read once, text is extracted, chunks are cut on
paragraph boundaries to at most `files.chunk_chars` each (default 6000, about
1,500 tokens, so instruction plus chunk plus answer fits `num_ctx`), and one job
per chunk is created from an instruction template with `{{chunk}}` and `{{index}}`
(V7.3 templates work too). Chunks and their order are saved as ordinary job
prompts. The upload is not kept. `reduce` is optional and explicit: a final job
(V7.2 stage mechanism) on a user-named pair that receives the chunk outputs in
order, refused with `CONTEXT_TOO_LARGE` if they do not fit. Chunks are processed
with V7.1 split if the user asks. Failure of one chunk does not stop the others.

**API.** `POST /api/batches/from-file` (multipart: file, instruction or
template, routing fields, `chunk_chars`, optional `reduce`). Errors:
`FILE_TOO_LARGE`, `FILE_TYPE_UNSUPPORTED`. Page: a drop zone on
the batch form, a preview of the chunk count before starting. 
**Config.** `files.max_bytes` (default 2 MB of text, a separate larger body limit for this endpoint only),
`files.chunk_chars`.
**Data.** Chunk text as job prompts, 90-day retention. No file stored.
**Memory.** Read and chunk as a stream and hold one chunk at a time. Plain text
needs no new dependency.
**Later: PDF.** Not in the first cut. It needs a new pure-Go PDF reader (a
dependency to justify under P1) and its peak RSS **must be measured on Mane**
against the 50 MB budget first. It would then also add a `PDF_NO_TEXT` error for
scanned files and a `files.max_pdf_pages` cap. If it does not fit the budget, it
is not built.
**Tests.** Chunk boundaries and sizes. Order preserved. Oversize and wrong-type
errors. A large text file does not grow RSS. One failed chunk leaves the rest.
`reduce` size refusal.
**Guardrails.** Constitution non-goal "RAG, document search or vector databases"
and the implementation plan guardrail "no document features": **owner wording
change proposed (item 3 in "Proposed constitution changes")**. P4.
**Open questions.** Is `reduce` wanted in the first cut? What chunk size suits
your documents?

---

## V7.7: Scheduled jobs

**Goal.** Run a saved job or batch on a schedule, for example nightly or weekly,
set up from the web page.

**Stories.** From the page I add "every night at 02:00, run `notes-summary` on
Firmino". I pause it for a week, run it now to test, and read its past runs.

**In scope.** Daily and weekly schedules created, edited, paused, run now and
deleted from the page or the API. Run history. **Out of scope.** Full cron
syntax, calendar rules, dependencies between schedules, running while Mane is
off, notifications (V7.9), config-file schedules.

**Behaviour.** A schedule names a V7.3 template (or an inline prompt), its
`vars` or `rows`, and an explicit machine and model (required, P10). At the due
time the router submits a job or batch on the **batch lane**. Times use Mane's
local time zone. Rules per schedule: `missed: skip` (default) ignores runs missed
while the router was off, `run_once` runs once at startup if the miss was within
`schedules.grace` (default 1 hour). `overlap: skip` (default) skips a run while the
previous run's job or batch is still waiting, running or retrying.

**Persistence.** Schedules are stored in the SQLite store, so they survive a
restart and are not in the config file. They are definitions: kept until
deleted, not swept at 90 days. The next due time is recomputed at start from
`last_due`, so a restart never double-runs. Run records (`schedule_runs`) follow
the 90-day sweep.

**Retry window.** A scheduled run is an ordinary job or batch, so the usual rules
apply: if the machine is down at the due time it waits and retries every 2
minutes for up to 3 hours, counted from its own first failure, then fails and the
run shows `failed`. The due time does not extend the window. A model that is not
installed fails the run with `MODEL_NOT_INSTALLED`, visible as failed. A long
retry keeps `overlap: skip` active, so the next run is skipped until it ends.

**API.** `GET /api/schedules` (name, rule, next run, last run, last status,
enabled). `POST /api/schedules` (create), `PATCH /api/schedules/{id}` (edit,
pause or resume with `enabled`), `DELETE /api/schedules/{id}`,
`POST /api/schedules/{id}/run` (run now, ignores `overlap`),
`GET /api/schedules/{id}/runs`. Validation: the machine exists, times parse, at
most `schedules.max_count`.

**UI.** A **Schedules** tab: a table (name, when, target, next run, last status,
enabled toggle) with buttons **New**, **Edit**, **Pause/Resume**, **Run now**,
**Runs** and **Delete**. The New/Edit form: name; daily or weekly and the time
(and weekday); template or prompt; vars; machine and model (required); `missed`
and `overlap` selects. Delete asks for confirmation. Runs open the linked job
or batch.

**Config.** `schedules.max_count` (default 50), `schedules.grace` (default `1h`).
**Data.** `schedules` table (id, name, rule, template or prompt, vars, machine,
model, missed, overlap, enabled, last_due, timestamps) and `schedule_runs` (id,
schedule id, due, started, job or batch id, status). Prompts live here, so never
logged (P16).
**Memory.** One timer in the router for all schedules.
**Tests.** Next-run maths including daylight-saving changes. `missed` and
`overlap`. Run now. Restart neither loses nor double-runs. Create, edit, pause,
delete via API. Validation errors. The pair is always as saved. Retry window
for a run with the machine down.
**Guardrails.** P10, P14, P16. **Risk:** the page has no login (D-079), so any
tailnet device could create a schedule that runs unattended. The Tailscale ACL
(D-081) limits that. The log records schedule name and outcome, never content.
**Open questions.** Does editing a schedule apply from the next due time only
(proposed: yes)?

---

## Proposed constitution changes

For the owner to approve. **The constitution is not edited.** If approved, apply
each as written and add a DECISIONS entry (its amendment rule).

**1. P9 (split plan and review as explicit modes).**
Before: "Routing follows simple, documented rules driven by clear signals: flags, pinned names, and what is installed where. The router does not read prompts to guess difficulty."
After: "Routing follows simple, documented rules driven by clear signals: flags, pinned names, what is installed where, and explicit user-chosen modes (a batch split plan, a review step). The router does not read prompts to guess difficulty."

**2. P10 (checking is a visible step, not a switch).**
Before: "If the user names a model or a machine, that is exactly what is used. No silent substitution, no switching mid-task. If it is unavailable, say so."
After: "If the user names a model or a machine, that is exactly what is used. No silent substitution, no switching mid-task. A second model used to check an answer is a separate, visible step the user chose, not a switch. If it is unavailable, say so."

**3. Non-goals (file input is not RAG).**
Before: "- RAG, document search or vector databases."
After: "- RAG, document search, embeddings or vector databases. Splitting a file into chunks to run as a batch is allowed."

**4. Retention (templates, schedules, threads) and P16.**
Before (hard limit): "| Retention of finished results (when stored) | 90 days |"
After: "| Retention of finished results and thread content (when stored) | 90 days (threads: from last activity). Templates and schedules are definitions and are kept until deleted |"
Before (P16, last sentence): "In V2 they are stored only in the saved-request store, for the retention period."
After: "They are stored only in the saved-request store: results for the retention period, and templates and schedules until deleted."

**5. P4 (50 MB): no change.** The PRDs require streaming, and a measurement on
Mane for anything heavy.

If a later `when_free` mode (V7.1 future note) is ever wanted, P10 would also
need: "A pair chosen at admission under a user-chosen mode is pinned from then
on." Not proposed now.

No conflict found with P3, P5 (a split or review that targets Mane must name an
8B or smaller model), P6, P7, P8, P17, or the hard limits on users, logins and
public exposure.
