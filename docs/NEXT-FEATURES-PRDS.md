# Next features: PRDs for V8 and V11 to V16

Status: **planned, owner-approved as roadmap items, not built.** Nothing here is
built until the owner says so (AGENTS.md). Versions and order are in
[ROADMAP.md](ROADMAP.md). Decisions: D-086 to D-094.

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
| Out of scope for all | Logins, multi-user, notifications (V10), OpenAI endpoint (V9), RAG |

Build order: **V11, V12, V13, V14, V8, V15, V16.**

---

## V11: Split a batch across both machines

**Goal.** Finish a batch sooner by running it on Mane and Firmino at once, each
with its own pinned model.

**Stories.** As the owner I submit 100 items and say "Mane runs `llama3.1:8b`,
Firmino runs the 30B", and both machines work until the batch is done. I can see
which machine and model produced each item.

**In scope.** A `split` plan on `POST /api/batches`. Per-machine progress.
**Out of scope.** Splitting one request across machines. Choosing machines by
item content. Picking models for the user. Changing models mid-batch.

**Behaviour.** The user gives a list of targets, each an explicit
`{machine, model}` pair, and an `assign` rule:

| `assign` | Meaning |
|---|---|
| `round_robin` (default) | Item i goes to target i mod n |
| `weights` | Targets get a share by user-set integer weights, for example 3 and 2 |
| `when_free` | Opt-in. An item takes the next target that is free **at admission**. Once admitted it is bound to that pair, and a retry stays on the same pair |

With `round_robin` and `weights` the pair is fixed at submit time. A down machine
holds its own items, which retry on it (D-071 window) while the other machine
carries on. Nothing is moved. Every item records its pair (P11). Items with a
pinned `machine` or `model` of their own are not split. Interactive requests
still go first (D-066).

**API.** `POST /api/batches` gains `"split":{"assign":"...","targets":[{"machine","model","weight"}]}`
and items need no routing fields. `GET /api/batches/{id}` gains per-target
`done`, `running`, `waiting`, `failed`. Page: a "Split" option in the batch form
and a per-machine progress bar. `pingpong batch submit --split mane:MODEL,firmino:MODEL`.

**Config.** `batches.max_items` (default 1000, a safety cap).

**Data.** Nothing new. Each job already stores machine and model. The batch row
stores the split plan.

**Memory.** Assignment is computed per item at submit or admission. No new
buffers.

**Tests.** Round-robin and weights counts. Each item runs on its assigned pair
only. Down machine: its items wait and the other finishes, none move. `when_free`
binds at admission and retries on the same pair. Pinned items are skipped by
the splitter. Lanes still honoured.

**Guardrails.** P10 and D-059 (wait, do not reroute). `when_free` is an explicit
user-chosen mode, so it is not silent substitution. **Flag for the owner:**
confirm `when_free` is acceptable under P10 (item 1 in section "Constitution
check").

**Open questions.** Is `when_free` wanted in the first cut, or only fixed
assignment? Should results of mixed models in one batch carry a visible marker
in the export (default: yes, columns for machine and model)?

---

## V12: Answer checking (review or revise by a second model)

**Goal.** Run a request on one model, then have a second, deeper model check it.

**Stories.** I ask the fast model for a draft, and the 30B reviews it and lists
mistakes. Or I ask for a final answer and the 30B rewrites the draft. I see both
steps and which model did each.

**In scope.** An explicit `check` mode on a request or batch item.
**Out of scope.** Choosing the reviewer automatically. Deciding whether to check
based on the prompt or the answer. Loops of more than one review. Voting among
several models.

**Behaviour.** The user names the reviewer pair, so it is explicit (P9, P10).
Two stages, saved as two linked jobs (`parent_id`, `stage`):

1. `answer`: the request as normal.
2. `review`, run only after stage 1 is `done`, on the reviewer pair. `mode`:
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
(all required). Job detail gains `stage`, `parent_id`, `children`. Page: a
"Check with" selector, and the result shows two panels (draft, review).
`pingpong` batch items take the same field.

**Config.** None.

**Data.** Two job rows per checked request, linked. A `stage` and `parent_id`
column. Doubles storage and run time for checked requests.

**Memory.** None extra. Stages are ordinary jobs.

**Tests.** Stage 2 starts only after stage 1 is `done`. Both stages pinned and
never swapped. `CONTEXT_TOO_LARGE` on oversize. Stage 2 failure keeps stage 1
visible and `unchecked`. Cancel cancels both. Restart recovery resumes the right
stage. A request without `check` is unchanged.

**Guardrails.** P9 and P10: reviewer is user-chosen, there is no content-based
decision. P11: both models shown. P16: the review prompt holds content, so it is
only stored, never logged. **Flag:** item 2 in "Constitution check".

**Open questions.** Which pairing do you want as the habit: cheap draft on Mane,
deep review on Firmino, or the reverse? A weak reviewer can miss errors, so
should the page show the reviewer is smaller than the answerer? Should
`revise` be allowed to differ in `num_predict`?

---

## V13: Prompt templates

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

## V14: Results export

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

## V8: Conversation threads

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

## V15: File and document input (not RAG)

**Goal.** Drop in a text or PDF file, split it into chunks, and process every
chunk as a batch.

**Stories.** I upload a long report, ask "summarise each section in 3 bullets",
and read the results in order, or export them.

**In scope.** Text, Markdown and PDF with a text layer. Chunking. One batch job per
chunk, same instruction. Optional final `reduce` step. **Out of scope.**
Embeddings, vector search, retrieval, an index, OCR, images, Word files, keeping
the uploaded file, answering questions over the whole document. (That would be
RAG, a non-goal.)

**Behaviour.** The file is read once, text is extracted, chunks are cut on
paragraph boundaries to at most `files.chunk_chars` each (default 6000, about
1,500 tokens, so instruction plus chunk plus answer fits `num_ctx`), and one job
per chunk is created from an instruction template with `{{chunk}}` and `{{index}}`
(V13 templates work too). Chunks and their order are saved as ordinary job
prompts. The upload is not kept. `reduce` is optional and explicit: a final job
(V12 stage mechanism) on a user-named pair that receives the chunk outputs in
order, refused with `CONTEXT_TOO_LARGE` if they do not fit. Chunks are processed
with V11 split if the user asks. Failure of one chunk does not stop the others.

**API.** `POST /api/batches/from-file` (multipart: file, instruction or
template, routing fields, `chunk_chars`, optional `reduce`). Errors:
`FILE_TOO_LARGE`, `FILE_TYPE_UNSUPPORTED`, `PDF_NO_TEXT`. Page: a drop zone on
the batch form, a preview of the chunk count before starting. 
**Config.** `files.max_bytes` (default 10 MB, a separate larger body limit for this endpoint only),
`files.max_pdf_pages` (default 200), `files.chunk_chars`.
**Data.** Chunk text as job prompts, 90-day retention. No file stored.
**Memory.** The main risk. Extract and chunk as a stream, hold one chunk at a
time, and cap bytes and pages. A pure-Go PDF reader is a new dependency to justify
(P1, AGENTS rules) and its peak RSS **must be measured on Mane** against the 50
MB budget before release. If it does not fit, PDF support is dropped and text
and Markdown stay.
**Tests.** Chunk boundaries and sizes. Order preserved. Oversize and wrong-type
errors. Scanned PDF gives `PDF_NO_TEXT`. Peak memory test on a large file. One
failed chunk leaves the rest. `reduce` size refusal.
**Guardrails.** Constitution non-goal "RAG, document search or vector databases"
and the implementation plan guardrail "no document features": **owner wording
change needed (item 3 in "Constitution check")**. P4. P1 (dependency).
**Open questions.** Is PDF in the first cut or text and Markdown only? Is `reduce`
wanted in the first cut? What chunk size suits your documents?

---

## V16: Scheduled jobs

**Goal.** Run a saved job or batch on a schedule, for example nightly or weekly.

**Stories.** Every night at 02:00 the router runs "summarise yesterday's notes"
on Firmino, and I read the result in the morning.

**In scope.** Daily and weekly schedules defined in config. Run now. History of
runs. **Out of scope.** Full cron syntax, calendar rules, dependencies between
schedules, running while Mane is off, notifications (V10), a schedule editor in
the page.

**Behaviour.** Each schedule names a V13 template (or an inline prompt), its
`vars` or `rows`, and routing fields, so the pair is explicit. At the due time
the router submits a job or batch on the **batch lane**. Times use Mane's local
time zone. `missed: skip` (default) ignores runs missed while Mane was off,
`run_once` runs once at startup if the miss was within `grace`. `overlap: skip`
(default) skips a run if the previous one is still active. Each run is recorded
and links to its job or batch. No hot reload: change config and restart (D-069).

**API.** `GET /api/schedules` (name, next run, last run, last status),
`POST /api/schedules/{name}/run` (run now). Page: a Schedules list.
**Config.**
```yaml
schedules:
  nightly-notes:
    at: "02:00"            # daily, local time
    weekday: sat           # optional: weekly instead
    template: notes-summary
    vars: {days: "1"}
    machine: firmino
    model: <MODEL>
    missed: skip
    overlap: skip
```
**Data.** A `schedule_runs` table (name, due, started, job or batch id, status).
Subject to the 90-day sweep. Job content is the usual stored content.
**Memory.** One timer per schedule.
**Tests.** Next-run maths including daylight-saving changes. `missed` and
`overlap` rules. Run now. Restart does not double-run. Invalid config fails at
start with a clear message. Pair always as configured.
**Guardrails.** P10 (pair named in config), P14, P16. Retry rules unchanged.
**Open questions.** Define schedules in config only (proposed), or also through
the API? Should a failed scheduled run retry for the 3-hour window (proposed:
yes, same as any job)?

---

## Constitution check

No principle is edited here. These need the owner's decision:

| # | Item | Why | Suggested wording, if the owner agrees |
|---|---|---|---|
| 1 | **P10, P9 and V11 `when_free`** | P10 says pinned means pinned and P9 lists the signals routing may use. `when_free` binds an item to a pair at admission, then never moves it. It is explicit and user-chosen, but it is a new kind of assignment | P9: "...flags, pinned names, what is installed where, and explicit user-chosen modes (split plan, review model)." P10: add "A pair chosen at admission under a user-chosen mode is pinned from then on." Or drop `when_free` |
| 2 | **P10 and V12 review** | A second model touches the answer. It is a separate, visible, user-chosen step, not switching mid-task | P10: add "A second model used for checking is a separate visible step the user chose, not a switch." |
| 3 | **Non-goals: "RAG, document search or vector databases" and V15** | File input chunks and processes a document. It has no retrieval, embeddings or index. The wording could be read to forbid it. IMPLEMENTATION-PLAN section 7 has the same issue ("no document features"), now reworded to say retrieval | Non-goals: "RAG, document search, embeddings, vector databases. Splitting a file into chunks to process as a batch is allowed." |
| 4 | **Hard limit: retention of finished results, 90 days** | Templates and schedules are definitions with no expiry. Thread retention needs a rule | Add: "Templates and schedules are kept until deleted. Threads follow the same 90 days from last activity." |
| 5 | **P4, 50 MB** | V15 (PDF) and V14 (large export) can break it if built carelessly | No change. The PRDs require streaming and a measurement on Mane |

No conflict found with P3 (all routing stays on Mane), P5 (Mane models stay at 8B
or less: a split or review that targets Mane must name an 8B or smaller
model), P6, P7, P8, P16, P17, or the hard limits on users, logins and public
exposure.
