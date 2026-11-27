# Operations

Status: v2. A runbook for keeping both machines healthy, based on what was
learned going live. Both machines are assumed always on.

## 1. Mane (Mac mini)

| Item | Setting or action |
|---|---|
| Sleep | Never sleep the computer. Display may sleep |
| Power failure | Enable automatic startup after a power failure |
| Start on boot | Ollama and the router run as user launch agents that start at automatic login, with keep-alive so they restart if they crash |
| Updates | Install macOS updates deliberately, then confirm the services come back |
| Disk | Free space. Keep only the 3B and 8B models plus room for V2 data |
| Ollama app | Do not run the Ollama desktop app. Run only the launch agent. The app holds the port and the launch agent then never starts, so its settings are silently not applied (D-074). **It is still installed (tech debt TD-1, accepted, D-082).** After every Mane reboot check that the listener on port 11434 is the launch agent, not the app: `lsof -nP -iTCP:11434 -sTCP:LISTEN`. If it is the app, quit it and run the kickstart command in section 4a |
| Model memory | Keep-alive 30 minutes (tune). Check `ollama ps` shows `100% GPU` for each model |
| Login | Automatic login on, FileVault off, so Mane recovers unattended after a power failure (see D-039 and [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md)) |

## 2. Firmino (Ubuntu laptop)

| Item | Setting or action |
|---|---|
| Lid | Closing or moving the lid must not suspend the machine |
| Sleep | Disable suspend and hibernate |
| Power failure | Enable "restore on AC power" in the BIOS if the option exists |
| Battery | Keep it in the machine as a short-term power buffer. Consider a charge limit to reduce wear |
| Thermals | Watch temperature during long CPU inference. Ensure ventilation |
| Updates | Apply the pending updates and reboot (a restart is required at the time of writing) |
| User | Create a normal user with sudo. Stop using root day to day |
| Inference service | The official install script creates `ollama.service`, which runs as the `ollama` user. Settings live in a systemd override file |
| Network | Ollama listens on Firmino's Tailscale address (Mane cannot reach its LAN address). A Tailscale ACL should restrict that port to Mane. See [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md) |
| Disk | Free space. Decide on reclaiming the unused Windows partitions |

## 2a. Ollama settings on each machine

Ollama is configured through environment settings of its service. The full
list, with reasons, is in [CONFIGURATION.md](CONFIGURATION.md) section 3, and
the steps are in [SETUP-GUIDE.md](SETUP-GUIDE.md). In brief:

| Machine | Setting | Value |
|---|---|---|
| Firmino | Listen address | Its Tailscale address on the Ollama port (D-072) |
| Firmino | Parallel requests | 1 |
| Firmino | Loaded models | 1 (more only if memory allows and is needed) |
| Firmino | Keep-alive | 30 minutes, so the 30B is not reloaded between requests |
| Firmino | Context | 8192 (`OLLAMA_CONTEXT_LENGTH`) |
| Firmino | How set | A systemd service override, so settings survive updates |
| Mane | Listen address | Local only |
| Mane | Parallel requests | 1 |
| Mane | Loaded models | 2 (the 3B and 8B) |
| Mane | Keep-alive | Tune. Free memory when idle if needed |
| Mane | How set | In the launch agent that runs `ollama serve` (not `launchctl setenv`, which is lost on reboot). Confirm after a reboot |

These match `max_concurrent = 1` in [CONFIGURATION.md](CONFIGURATION.md).

## 2b. Running the router on Mane

| Item | Guidance |
|---|---|
| Service | A user launch agent (`launchd`) that starts at automatic login and restarts on failure. The router listens on localhost, and Tailscale Serve publishes it to the tailnet |
| Config | A config file on Mane with real values. Never committed |
| Logs | Written by the service. Metadata only, never prompts. Check log size occasionally |
| Updates | Replace the program, restart the service, and confirm it comes up. Running jobs are stopped on purpose and retried after the restart (V6) |
| Memory check | Look at the router's memory now and then against the 50 MB working budget |

## 3. Health checks

Run `pingpong doctor` on Mane (same config path as the router). It checks config,
storage, and each machine without starting the router. Add `--json` for scripts.
Add `--chat` to send a tiny real request to each machine's default model. This
is what proves the real Ollama stream works. **Its speed figure is a 2-token
sample and is not a benchmark** (D-077). Use section 9 for speed.

The Status tab on the page (and `GET /api/status`) shows machines, job counts,
router memory and uptime. It never shows prompts or outputs.

Basic checks the router performs, and the owner can run by hand:

| Check | Meaning |
|---|---|
| Machine reachable | The machine answers on its inference port |
| Models listed | Ollama returns the installed models |
| Disk free | Enough space remains for models and saved data |
| Recent errors | Retries in progress or failures |

The web page shows reachability. Anything deeper is out of scope for now.

## 4. Model management

| Task | Guidance |
|---|---|
| Adding a model | Install it on the target machine, then confirm it appears in the registry |
| Removing a model | Remove it on the machine. Check that no default points at it |
| Disk budget | Mane: a few models only. Firmino: the larger library |
| Size rule | Nothing over 8B on Mane |

## 4a. Troubleshooting

| Symptom | Check |
|---|---|
| Page does not load | Is the MacBook Air on the tailnet? Is the router running on Mane? |
| "Firmino is down" | Is Firmino on? Is Ollama running there? Check how the connection fails (next three rows) |
| `connection refused` to Firmino | The machine answered but nothing listens on that address and port. Check `OLLAMA_HOST` in the override file, and `systemctl status ollama` |
| `prohibited` or `Destination Unreachable` | The network blocks it, for example different subnets. Use the Tailscale address (D-072) |
| Timeout | A firewall is dropping the traffic, or the machine is off or asleep |
| Ollama on Firmino lists no models | The service runs as the `ollama` user with models in `/usr/share/ollama/.ollama/models`. Models pulled earlier as root are in root's home. Pull them again with the service running |
| Mane's Ollama ignores the settings | The Ollama desktop app is holding the port. Quit it, then `launchctl kickstart -k gui/$(id -u)/com.ollama.serve` |
| Router log shows only "listening" | Normal. Only errors and one line per finished request or job are logged. See section 10 |
| "Model not installed" | Is it installed on that machine? Does the name match exactly (including the tag)? |
| A request waits a long time | Is another request running on that machine? Firmino on a long prompt can take minutes |
| First answer is slow | The model may be loading from disk. Later requests are faster while it stays loaded |
| Mane feels sluggish | Check memory pressure. Is a model larger than 8B loaded? |
| Firmino very hot or loud | Check thermals and ventilation. Consider limiting load |

## 5. After a reboot or power loss

1. Confirm both machines are on the network.
2. Confirm Ollama is up on each.
3. Confirm the router is up on Mane.
4. Confirm waiting and retrying requests resumed (saved requests, V2 and later).
5. If a request fails right after boot because Ollama was not ready yet, wait one
   health interval (30 seconds by default). The router then loads the model list
   by itself. No restart is needed.

## 6. Saved-request data on Mane (V2 and later)

- Saved requests live on Mane's disk, in `storage.data_dir` (use an absolute
  path). Keep an eye on size.
- Results older than 90 days are removed automatically.
- Optional: copy the saved data elsewhere now and then if it becomes valuable.
  Stop the router first, or wait for `pingpong backup` in V6
  ([V6-OPERABILITY.md](V6-OPERABILITY.md)), which copies safely while it runs.
- The data holds prompts and outputs, and from V7.3 on also templates, threads and schedules (and from V7.7 the prompts they run). Treat copies as private.
- From V7.1, large batches (up to 1000 items) and V7.6 uploads make the file grow faster. Check its size now and then and prefer `pingpong backup` before an upgrade.

## 7. Adding a machine (V4)

1. Give it a fixed address. Prefer its Tailscale address if it is on a different subnet from Mane.
2. Install and secure its inference service. Restrict its port to Mane only.
3. Add it to the `machines:` list in the local config with a name, address,
   role (for example `heavy`), default model, `max_concurrent` and `num_ctx`,
   then restart the router. No rule changes are needed. If it shares a role
   with another machine, the least loaded machine that is up is chosen.
4. Confirm it shows as reachable and its models are listed.
5. Update [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md).

## 8. Open questions

- Which BIOS options exist on the ThinkBook.
- Whether to lock the Firmino Ollama port to Mane with a Tailscale ACL
  ([NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md) section 4).
- None on service supervision: Ollama and the router run as user launch agents
  (see [SETUP-GUIDE.md](SETUP-GUIDE.md)).

## 9. Measuring speed and quality

Run these **on Mane** (the router is at `127.0.0.1:8787` there), not on
Firmino. Everything is private and local. Results are appended to
`~/pingpong-bench.log`.

| Goal | How |
|---|---|
| Quick speed of one model | `sh deploy/bench/quick.sh MACHINE MODEL`. Sends a short and a long prompt (`num_predict` 128) and prints generation and prompt tokens/s, load and total time |
| Which model answers better | `python3 deploy/bench/quality.py`. Sends the same six prompts (plus your own, one per line in `~/my-prompts.txt`) to each contestant at temperature 0, shows the answers blind as A, B, C in `~/pingpong-quality.txt`, and writes the key to `~/pingpong-quality-key.txt`. Set contestants with `PINGPONG_CONTESTANTS=mane:llama3.1:8b,firmino:MODEL`. Add `@N` to a contestant for a bigger answer budget (needed for reasoning models, see section 13) |
| Summarise the router log (speeds, retries, loads per machine) | `python3 deploy/bench/logsummary.py [LOGFILE] [--since UTC-time]` |
| Long soak on both machines | The **Stress** tab on the page. Choose a target (mane, firmino, both) and a duration (up to 2 hours), then Start. It keeps two short batch jobs queued per machine and shows done and failed counts and median speeds. Stop cancels its jobs. Closing the browser does not stop it |

The auto-checks in the quality script cover math, an exact 3-line format, a
25-word summary and a short explanation. They only catch obvious failures, so
read the answers yourself. Do not compare one run to another by less than about
10 percent. Never use `doctor --chat` as a benchmark.

## 10. Reading the router log

The router logs JSON lines to its log file (`/tmp/pingpong.err` in the example
launch agent). It never logs prompts or outputs (D-032). Events:

| Event | When | Notable fields |
|---|---|---|
| `route_error` | A request could not be routed | `error_code`, `message` |
| `request_done` | A live chat finished | `machine`, `model`, `mode`, `output_tokens_per_s`, `prompt_tokens_per_s`, `load_ms`, `queue_wait_ms`, `duration_ms` |
| `job_done`, `job_retrying`, `job_failed`, `job_cancelled` | A saved job attempt ended | the same, plus `request_id`, `lane`, `error_code` (and from V7.1 `batch_id`, from V7.2 `stage`) |
| `batch_submitted`, `export_done`, `schedule_run`, `check_created`, `check_failed`, `reduce_created` | V7.1 to V7.7 | `batch_id`, `items`, `source`, `schedule` (a name), `outcome`, `error_code`. Never content ([NEXT-FEATURES-BUILD-SPEC.md](NEXT-FEATURES-BUILD-SPEC.md) section 1.5) |
| `notify_failed` | V7.9 outbound webhook error | `error_code` only (never the webhook URL) |
| `host_sample` | The page asked for hardware numbers (about every 5 seconds while it is open) | `machine`, `cpu_percent`, `ram_percent`, `temp_c` or `thermal_level`, `load_1`, `net_rx_bps` (down), `net_tx_bps` (up). Time is the line's `time`. No prompts, outputs, or stats URL (D-125) |

Useful questions to ask of it: which machine is slowest, how often jobs retry
(and with which `error_code`), how long requests queue, and whether model
loads (`load_ms`) are frequent, which means the keep-alive is too short. Use
`jq` or `grep` on the file.

## 11. Runbook: soak test (Stress tab)

Goal: prove both machines run for an hour or more without failures, find speed
drift (heat), and check the router's memory.

**Before you start (once, on Mane).** The Stress tab, the speed log line and the
bench scripts are in the new build. Update Mane, using your own paths:

```bash
cd ~/pingpong        # your checkout
git fetch origin && git checkout main && git pull origin main
make build-darwin-arm64
cp bin/pingpong-darwin-arm64 /usr/local/bin/pingpong   # wherever your launch agent points
launchctl kickstart -k gui/$(id -u)/com.pingpong.router
```

(If the pull request is not merged yet, use `git checkout cursor/stress-run-aa2f`.)
Then run `pingpong doctor --chat` and check the Status tab. Both machines must
be ok before you begin. On Firmino, once: `sudo apt install lm-sensors`.

**Steps**

1. On Mane, note the start time in UTC: `date -u +%Y-%m-%dT%H:%M`.
2. On Firmino, in an SSH session, run `watch -n 10 'sensors | grep -i -E "temp|tctl"; free -h | head -2; ollama ps'`
   and leave it up. (Type `export OLLAMA_HOST=<FIRMINO_TAILSCALE_IP>:11434`
   first, with the real address and no brackets.)
3. Open the page from the MacBook Air, Stress tab. Target **both**, seconds
   **3600**, Start. (The maximum is 7200. Start with an hour.)
4. After about 20 minutes, in the Chat tab, send one short question with the
   **fast** flag. It should answer within a few seconds, even though the stress
   run is busy: interactive requests are admitted ahead of the soak's batch jobs.
   It waits at most for the current short job (a few seconds on Mane).
5. Every 15 to 20 minutes, glance at the Stress tab (done, failed, speeds) and at
   the Firmino temperature. Note the highest temperature you see.
6. When it finishes, or after pressing Stop, copy the Stress tab numbers.
7. On Mane, summarise the whole run and then only the last third. With start
   time `T` and `T_LATE` (start plus 40 minutes for a 60-minute run), both in the
   same UTC format, run:

```bash
python3 deploy/bench/logsummary.py /tmp/pingpong.err --since T
python3 deploy/bench/logsummary.py /tmp/pingpong.err --since T_LATE
pingpong doctor --chat
```

**What to look for**

| Check | Good | Worry |
|---|---|---|
| Failed jobs | 0 | Any. Note the error code in the log |
| Mane speed | About 21 tokens/s median | A steady fall over time, or large swings |
| Firmino speed | About 12 tokens/s median | Below about 9, or falling over the hour (heat) |
| Last third against the whole run | Within about 10 percent | Firmino clearly slower later: it is throttling |
| Firmino temperature | Stays below about 85 degrees Celsius | Above about 90 sustained |
| Router memory (Status tab) | Under 50 MB, flat | Growing steadily |
| Retries and `error_code` in the log | None | `MACHINE_DOWN` or `MACHINE_FAILED` bursts |
| Model loads (`loads>0` in the summary) | About one per machine | Many: the keep-alive is too short, or another model is evicting it |
| Chat request during the soak | Answered promptly | Waits for minutes: lanes are not working |

**Send back:** the Stress tab numbers (copy the text), the two `logsummary`
outputs, the highest Firmino temperature and its `free -h` line, router memory
from the Status tab, the `doctor --chat` result, and anything odd you noticed.

## 12. Runbook: reboot and power-cut tests

Goal: both machines come back and work with no manual action. Run in this order,
and only continue if the previous part passed. Note the time each part takes.

**A. Firmino reboot**

1. From Mane: `pingpong doctor --chat` (baseline, must pass).
2. On Firmino: `sudo reboot`. Wait 3 minutes.
3. From Mane, without touching Firmino:
   - `curl -m 5 http://<FIRMINO_TAILSCALE_IP>:11434/api/tags` lists the models.
   - `pingpong doctor --chat` passes.
4. On Firmino: `systemctl status ollama` is active, and
   `systemctl show ollama -p Environment` still shows your settings (host,
   parallel, keep-alive, context, no-cloud).

**B. Mane reboot**

1. Note `tailscale serve status`. Submit three batch jobs to Firmino with
   `pingpong batch submit`, so work is in flight.
2. Reboot Mane (Apple menu, Restart). Wait 3 minutes. If the screen was locked
   or at a login window, note it: automatic login should avoid that.
3. From the MacBook Air, over SSH to Mane, check:
   - `lsof -nP -iTCP:11434 -sTCP:LISTEN` shows `ollama` started by the launch
     agent. **If it shows `Ollama`, the desktop app grabbed the port (tech debt
     TD-1, D-082).** Quit it, run `launchctl kickstart -k gui/$(id -u)/com.ollama.serve`,
     and report that it happened.
   - `launchctl print gui/$(id -u)/com.pingpong.router | head -20` is running.
   - `curl -H 'Host: localhost' http://127.0.0.1:8787/api/health` says ok.
   - `tailscale serve status` shows the router, tailnet only, with no manual step.
   - `pingpong doctor --chat` passes.
   - `pingpong batch get <id>` until `pending` is 0. Jobs must end `done`. One
     retry is fine. A job still `retrying` after 10 minutes is not.
4. Open the page from the MacBook Air and send one request to each machine.

**C. Mane power cut** (only after A and B passed)

1. Pull Mane's power plug for about 10 seconds, then plug it back in. Do not
   press anything.
2. Wait 3 minutes. It should start by itself and log in.
3. Repeat the checks in B step 3 and 4.

**D. Firmino power cut** (optional)

Unplug Firmino's charger for 30 seconds. It should keep running on battery, and
`pingpong doctor --chat` should still pass. The battery covers short cuts only,
so do not test a full power loss.

**What to look for:** every service back without a manual step, the Ollama
listener check in B, the Serve status, and jobs finishing. **A manual action is
the finding**, so write down exactly what you had to do.

**Send back:** for each part, pass or fail, the time until the page loaded
again, the outputs of the commands that failed, and the `lsof` output from B.

## 13. Runbook: `gpt-oss:20b` blind comparison

Goal: decide by blind reading whether `gpt-oss:20b` beats the 30B model on your
kind of work, and what it costs in time. Assessment and expectations:
[HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 2a. It is a reasoning
model, so it needs a bigger answer budget and is judged on "median reply" time.

**1. Pull the model (on Firmino).** About 14 GB, so it takes a few minutes.

```bash
export OLLAMA_HOST=<FIRMINO_TAILSCALE_IP>:11434     # real address, no brackets
ollama pull gpt-oss:20b
ollama list
df -h /usr/share/ollama
```

Do not change `OLLAMA_MAX_LOADED_MODELS`. It stays at 1.

**2. Update Mane to the build with the bench scripts** (see section 11, "Before
you start"). The router learns about the new model within 5 minutes, or at once
when a request names it (`refresh_on_model_miss`). To check:
`curl -s -H 'Host: localhost' http://127.0.0.1:8787/api/machines` should list
`gpt-oss:20b` under Firmino.

**3. Optional: add your own prompts** to `~/my-prompts.txt`, one per line.

**4. Run on Mane** (not on Firmino). It takes about 20 to 30 minutes.

```bash
cd ~/pingpong
PINGPONG_CONTESTANTS="mane:llama3.1:8b,firmino:qwen3:30b-a3b-instruct-2507-q4_K_M,firmino:gpt-oss:20b@1500" \
  python3 deploy/bench/quality.py
```

What the setting means: `machine:model` for each contestant, and `@1500` gives
`gpt-oss:20b` a 1,500-token budget, because it spends tokens thinking before it
answers (the default budget is 220, which would leave it with an empty answer).
The environment variables are `PINGPONG_CONTESTANTS` and, if the router is not at
the default address, `PINGPONG_URL`.

Variations:

| Want | Command change |
|---|---|
| Add the dense 14B too | Append `,firmino:qwen2.5:14b`. It adds about 10 minutes |
| Fair comparison on long answers (your own prompts) | Give everyone the same budget, for example `mane:llama3.1:8b@600,firmino:qwen3:30b-a3b-instruct-2507-q4_K_M@600,firmino:gpt-oss:20b@1500`. It is slower |
| Only the two Firmino models | Drop the Mane entry |

While it runs, the script prints one line per prompt with time and pass or fail.
Each Firmino model loads once, because the script finishes all prompts for one
contestant before the next. After each Firmino contestant finishes, on Firmino
run `ollama ps` and keep the output (it shows the memory each model uses, which
fills a gap in the measurements).

**5. Read the results.**

1. `cat ~/pingpong-quality.txt`. Answers are labelled A, B, C in random order for
   each prompt. Pick the best answer for each prompt, **before** looking at the
   key.
2. `cat ~/pingpong-quality-key.txt` shows who wrote which answer and the
   automatic checks (math, 3-line format, 25-word summary, short explanation).
3. Read the speed summary the script printed (also saved in `~/pingpong-bench.log`):

| Column | How to read it for `gpt-oss:20b` |
|---|---|
| write tok/s | Counts the thinking tokens too, so it can look fine while answers are slow. Do not use it to judge |
| median reply | **The number that matters.** Time from sending to the complete answer |
| checks | Automatic pass count. Only obvious failures |
| errors | Includes "empty answer" when the model used its whole budget thinking. If that appears, raise the `@` budget and rerun just that model |

**Decision rule.** `gpt-oss:20b` is worth considering as a replacement default
only if it wins on at least 4 of 6 prompts (or most of your own) **and** its
median reply time is no more than about twice the 30B's. Otherwise keep the
30B as default and leave gpt-oss installed for pinned use, or remove it with
`ollama rm gpt-oss:20b`. A replacement would also need the reasoning-effort gap
fixed (tech debt TD-2).

**Send back:** the printed speed summary, your pick for each prompt and the key
file, the `ollama ps` outputs, and any error lines.

