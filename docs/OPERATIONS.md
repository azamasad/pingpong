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
| Ollama app | Do not run the Ollama desktop app. Run only the launch agent. The app holds the port and the launch agent then never starts, so its settings are silently not applied (D-074). Quit the app, remove it from Login Items, or delete it |
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
- The data holds prompts and outputs. Treat copies as private.

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
| Which model answers better | `python3 deploy/bench/quality.py`. Sends the same six prompts (plus your own, one per line in `~/my-prompts.txt`) to each contestant at temperature 0, shows the answers blind as A, B, C in `~/pingpong-quality.txt`, and writes the key to `~/pingpong-quality-key.txt`. Set contestants with `PINGPONG_CONTESTANTS=mane:llama3.1:8b,firmino:MODEL` |
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
| `job_done`, `job_retrying`, `job_failed`, `job_cancelled` | A saved job attempt ended | the same, plus `request_id`, `lane`, `error_code` |

Useful questions to ask of it: which machine is slowest, how often jobs retry
(and with which `error_code`), how long requests queue, and whether model
loads (`load_ms`) are frequent, which means the keep-alive is too short. Use
`jq` or `grep` on the file.
