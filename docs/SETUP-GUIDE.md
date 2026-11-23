# Setup Guide (Phase 0)

Status: draft v1. A step-by-step checklist for the owner to prepare Mane and
Firmino. This is done on the real machines, before or alongside development.
A coding agent cannot do these steps.

Placeholders in angle brackets (such as `<MANE_LAN_IP>`) are values you fill in.
**Never commit real addresses to this public repository.** Commands marked
"verify" should be checked against the installed version's help output, because
tools change.

Related: [OPERATIONS.md](OPERATIONS.md), [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md),
[HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md), [CONFIGURATION.md](CONFIGURATION.md).

## 0. Decisions applied in this guide

| Decision | Detail |
|---|---|
| Starting models | Mane: `llama3.1:8b` and `llama3.2:3b`. Firmino: `qwen2.5:14b`. All non-thinking models (see D-038) |
| Context window | Set explicitly to 8192 tokens on both, because Ollama's default of 4096 silently limits long prompts (D-042) |
| Mane unattended restart | FileVault off and automatic login on, so it recovers by itself after a power failure (D-039) |
| Exposure | Mane's router listens on localhost only, and Tailscale Serve publishes it to the tailnet. Firmino's Ollama accepts connections from Mane only (D-040) |
| Ollama cloud features | Off, so everything stays local (D-043) |

## 1. Before you start

- [ ] Both machines on Ethernet.
- [ ] Note both machines' LAN addresses (keep them in your own notes, not the repo).
- [ ] In your router, reserve a fixed address (DHCP reservation) for each machine.
- [ ] Both machines are signed in to the same Tailscale account (Firmino already is).
- [ ] In the Tailscale admin console, enable MagicDNS and HTTPS certificates
      (needed for Tailscale Serve). Give Mane a clear name such as `mane`.

## 2. Firmino (Ubuntu)

### 2.1 Access and user

- [ ] Create a normal user with sudo (`adduser <you>`, then `usermod -aG sudo <you>`).
- [ ] Confirm you can log in as that user over Tailscale SSH.
- [ ] Stop using `root` for day-to-day work. In the Tailscale ACL (SSH rules),
      allow SSH as your normal user, not root (verify in the Tailscale docs).
- [ ] If the regular SSH server is also used, set `PermitRootLogin no` in its
      config and reload it.

### 2.2 Updates

- [ ] `sudo apt update && sudo apt full-upgrade -y`
- [ ] Reboot (`sudo reboot`). A restart is pending at the time of writing.

### 2.3 Never sleep, survive the lid

- [ ] In `/etc/systemd/logind.conf`, set `HandleLidSwitch=ignore`,
      `HandleLidSwitchExternalPower=ignore` and `HandleLidSwitchDocked=ignore`.
- [ ] Disable sleep: `sudo systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target`.
- [ ] Reboot so the changes apply.
- [ ] BIOS: look for "power on after AC loss" or similar and enable it if it
      exists. Note what the ThinkBook offers.
- [ ] Optional: set a battery charge limit (for example 60 to 80 percent) if the
      BIOS or a tool supports it.
- [ ] Place the laptop where air can flow. Plan to check temperatures
      (`sensors`, after installing `lm-sensors`) during long runs.

### 2.4 Install Ollama

- [ ] Install: `curl -fsSL https://ollama.com/install.sh | sh` (this also
      installs the systemd service named `ollama`).
- [ ] Set its settings with `sudo systemctl edit ollama.service`, adding under
      `[Service]`:

| Setting | Value | Reason |
|---|---|---|
| `OLLAMA_HOST` | `0.0.0.0:11434` | Reachable on the LAN. The firewall restricts it to Mane |
| `OLLAMA_NUM_PARALLEL` | `1` | One request at a time, matching the router's gate |
| `OLLAMA_MAX_LOADED_MODELS` | `1` | Memory control |
| `OLLAMA_KEEP_ALIVE` | `30m` | Avoids reloading the 14B between requests (tune) |
| `OLLAMA_CONTEXT_LENGTH` | `8192` | Explicit context window |
| `OLLAMA_NO_CLOUD` | `1` | Local only |

Each is written as a line of the form `Environment="NAME=value"`.

- [ ] Apply: `sudo systemctl daemon-reload && sudo systemctl restart ollama`.
- [ ] Confirm it runs: `systemctl status ollama`.

### 2.5 Firewall (do this carefully)

Keep your current SSH session open while changing the firewall, so you cannot
lock yourself out.

- [ ] `sudo ufw default deny incoming`
- [ ] `sudo ufw default allow outgoing`
- [ ] `sudo ufw allow in on tailscale0 to any port 22 proto tcp` (admin over Tailscale)
- [ ] `sudo ufw allow from <MANE_LAN_IP> to any port 11434 proto tcp` (Mane only)
- [ ] `sudo ufw enable`, then `sudo ufw status verbose`.

Do **not** allow the whole `tailscale0` interface, because that would let your
MacBook Air reach Firmino's Ollama directly.

### 2.6 Models

- [ ] `ollama pull qwen2.5:14b` (about 9 GB).
- [ ] Run it once and check `ollama ps`: the processor column should say
      `100% CPU`.

### 2.7 Checks

- [ ] From Mane: `curl http://<FIRMINO_LAN_IP>:11434/api/tags` lists the model.
- [ ] From the MacBook Air (not Mane): the same request is **blocked or times out**.
- [ ] Reboot Firmino. Confirm Ollama is back without any action.
- [ ] Optional: unplug power briefly and confirm it stays up on battery.

## 3. Mane (Mac mini)

### 3.1 Basics

- [ ] Install pending macOS updates.
- [ ] Never sleep and recover from power loss:
      `sudo pmset -a sleep 0`, `sudo pmset -a autorestart 1`, and optionally
      `sudo pmset -a womp 1`. Check with `pmset -g`. The display may still sleep.
- [ ] Turn FileVault **off** and enable automatic login for your user
      (System Settings, Users and Groups). FileVault asks for a password at
      startup, so it blocks unattended recovery after a power failure. This is a
      trade-off (see D-039). With FileVault off, anyone with physical access to
      the Mac can read its disk, so keep it somewhere safe.
- [ ] Confirm the Mac does not lock itself in a way that stops services (services
      here run as your logged-in user).

### 3.2 Tailscale

- [ ] Install Tailscale, sign in, and set it to launch at login.
- [ ] Confirm the MacBook Air can reach Mane's tailnet name.

### 3.3 Install Ollama

- [ ] Install the command-line version with Homebrew: `brew install ollama`.
      Do not also install the Ollama desktop app. It would start its own server
      at login and conflict.
- [ ] Create a user service so Ollama starts at login with the right settings.
      Create a launch agent file (for example in `~/Library/LaunchAgents/`) that
      runs `ollama serve` at load, keeps it alive, and sets these environment
      values:

| Setting | Value | Reason |
|---|---|---|
| `OLLAMA_HOST` | `127.0.0.1:11434` | Local only. Only the router uses it |
| `OLLAMA_NUM_PARALLEL` | `1` | Explicit, because Ollama may otherwise choose a value on Apple GPUs |
| `OLLAMA_MAX_LOADED_MODELS` | `2` | The 8B and the 3B |
| `OLLAMA_KEEP_ALIVE` | `30m` | Tune |
| `OLLAMA_CONTEXT_LENGTH` | `8192` | Explicit context window |
| `OLLAMA_NO_CLOUD` | `1` | Local only |

- [ ] Load the service (`launchctl bootstrap gui/$(id -u) <file>`), verify the
      syntax with `man launchctl` or `launchctl help`.
- [ ] Do **not** use `launchctl setenv` for these. Those settings do not survive a
      reboot.

### 3.4 Models

- [ ] `ollama pull llama3.1:8b` (about 5 GB) and `ollama pull llama3.2:3b` (about 2 GB).
- [ ] Run each once and check `ollama ps`: the processor column should say `100% GPU`.
      If part of a model is on the CPU, it is too big for the memory Ollama allows
      the GPU to use on a 16 GB Mac (roughly 11.3 GiB in total).
- [ ] Free disk: `ollama list`, and remove anything you do not need with `ollama rm`.

### 3.5 Checks

- [ ] `curl http://127.0.0.1:11434/api/tags` lists both models.
- [ ] Reboot Mane. Without touching it, confirm it logs in and Ollama is running.
- [ ] Simulate a power cut if you can (pull the plug for a few seconds). Confirm
      it starts again by itself.

## 4. Router exposure (later, when the router exists)

- [ ] The router listens on `127.0.0.1` only.
- [ ] Publish it to your tailnet with Tailscale Serve (for example
      `tailscale serve --bg <PORT>`; verify the syntax with `tailscale serve --help`).
- [ ] Set `allowed_hosts` in the router config to Mane's tailnet name.
- [ ] In the Tailscale ACL, allow only your own devices to reach Mane.
- [ ] Check from the MacBook Air that the page loads, and from a device **not**
      on your tailnet that it does not.

## 5. Measure real speeds

Do this for each model on its machine, and record the results in
[HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 8.

1. Run the model with timing output on (`ollama run <model> --verbose`). It prints
   prompt processing speed and generation speed after each reply.
2. Try a **short prompt** (a sentence) and a **long prompt** (a few thousand
   tokens of pasted text).
3. Record: cold load time (first run after a restart), prompt tokens per second,
   generation tokens per second, time to first token for the long prompt, and
   memory use (`ollama ps`).
4. On Firmino, watch temperature during a few minutes of generation.
5. On Firmino, also try limiting the thread count to the 4 physical cores
   (set `num_thread` to 4 in the request options) and compare speed. CPU
   inference sometimes does worse when it uses all 8 hardware threads.

Then tune the router's settings:

| Setting | How to tune |
|---|---|
| `first_token_timeout` | Must exceed the slowest long prompt on Firmino: (largest prompt in tokens) divided by (measured prompt tokens per second), plus load time, plus margin |
| `idle_token_timeout` | At least several times the slowest gap between tokens |
| `OLLAMA_KEEP_ALIVE` | Long enough to avoid reloads in normal use |
| Context window | Raise above 8192 only if you need it and speed allows |

## 6. Done when

- [ ] Both machines survive a reboot and a short power cut without help.
- [ ] Firmino's Ollama answers Mane and nobody else.
- [ ] Mane's Ollama is local only.
- [ ] The MacBook Air reaches Mane over Tailscale.
- [ ] Models are installed and the measurements are recorded.
- [ ] No real addresses have been written into the repository.
