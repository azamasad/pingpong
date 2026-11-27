# Setup Guide (Phase 0)

Status: v2. A step-by-step checklist to prepare Mane and Firmino, corrected
after the real go-live (the changes are listed in section 7). It is done on the
real machines. A coding agent cannot do these steps.

Placeholders in angle brackets (such as `<FIRMINO_TAILSCALE_IP>`) are values you
fill in. In a shell, do not type the angle brackets, and do not paste
placeholders literally: `<` and `>` are redirection characters.
**Never commit real addresses to this public repository.** Commands were checked
against the vendors' documentation when written. If a command fails, check the
installed version's help output, because tools change.

Related: [OPERATIONS.md](OPERATIONS.md), [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md),
[HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md), [CONFIGURATION.md](CONFIGURATION.md).

## 0. Decisions applied in this guide

| Decision | Detail |
|---|---|
| Starting models | Mane: `llama3.1:8b` and `llama3.2:3b`. Firmino: `qwen3:30b-a3b-instruct-2507-q4_K_M` (D-073). All non-thinking models (see D-038) |
| Context window | Set explicitly to 8192 tokens on both, because Ollama's default of 4096 silently limits long prompts (D-042) |
| Mane unattended restart | FileVault off and automatic login on, so it recovers by itself after a power failure (D-039) |
| Exposure | Mane's router listens on localhost only, and Tailscale Serve publishes it to the tailnet. Firmino's Ollama listens on its Tailscale address, because the machines are on different subnets, and the Tailscale ACL should allow only Mane to reach it (D-072, supersedes D-054) |
| Ollama cloud features | Off, so everything stays local (D-043) |

## 1. Before you start

- [ ] Both machines on Ethernet.
- [ ] Check whether they can reach each other directly: from Mane, `ping` Firmino's LAN address. If it says "Destination Unreachable" or "prohibited", the machines are on different subnets and the gateway blocks traffic between them. Use Tailscale addresses between them (this was the case here).
- [ ] Note Firmino's Tailscale address with `tailscale ip -4` on Firmino (keep it in your own notes, not the repo).
- [ ] Both machines are signed in to the same Tailscale account.
- [ ] In the Tailscale admin console, enable MagicDNS and HTTPS certificates
      (needed for Tailscale Serve). Give Mane a clear name such as `mane`.

## 2. Firmino (Ubuntu)

### 2.1 Access and user

- [ ] Create a normal user with sudo (`adduser <you>`, then `usermod -aG sudo <you>`).
- [ ] Confirm you can log in as that user over Tailscale SSH.
- [ ] Stop using `root` for day-to-day work. In the Tailscale ACL (SSH rules),
      allow SSH as your normal user, not root (the `users` field of the SSH rule).
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

- [ ] Install: `curl -fsSL https://ollama.com/install.sh | sh`. This creates the
      `ollama` user and the systemd service `ollama.service`, which runs as
      that user. If you ran `ollama serve` by hand earlier, stop it first, or the
      port will be busy.
- [ ] Set its settings in `/etc/systemd/system/ollama.service.d/override.conf`
      (`sudo systemctl edit ollama.service` opens it), adding under `[Service]`:

| Setting | Value | Reason |
|---|---|---|
| `OLLAMA_HOST` | `<FIRMINO_TAILSCALE_IP>:11434` | Its Tailscale address, so Mane can reach it. Not `0.0.0.0`, which would also listen on the LAN (D-072) |
| `OLLAMA_NUM_PARALLEL` | `1` | One request at a time, matching the router's gate |
| `OLLAMA_MAX_LOADED_MODELS` | `1` | Memory control |
| `OLLAMA_KEEP_ALIVE` | `30m` | Avoids reloading the model between requests (tune) |
| `OLLAMA_CONTEXT_LENGTH` | `8192` | Explicit context window |
| `OLLAMA_NO_CLOUD` | `1` | Local only |

Each is written as a line of the form `Environment="NAME=value"`.

- [ ] Apply: `sudo systemctl daemon-reload && sudo systemctl restart ollama`.
- [ ] Confirm it runs: `systemctl status ollama`.
- [ ] For the `ollama` command on Firmino itself, run
      `export OLLAMA_HOST=<FIRMINO_TAILSCALE_IP>:11434` first (add it to your shell
      profile), because the server no longer listens on localhost.
- [ ] Models are stored for the service user in
      `/usr/share/ollama/.ollama/models`. Models pulled earlier as root are
      elsewhere and will not show up. Pull them again with the service running.
- [ ] Note the version (`ollama --version`).

### 2.5 Firewall (do this carefully)

Keep your current SSH session open while changing the firewall, so you cannot
lock yourself out.

- [ ] `sudo ufw default deny incoming`
- [ ] `sudo ufw default allow outgoing`
- [ ] Do not open port 11434 on the LAN. Ollama does not listen there.
- [ ] `sudo ufw enable`, then `sudo ufw status verbose`.

No rule is needed for Tailscale SSH, because Tailscale handles it itself.

**Important:** on Linux, Tailscale accepts all traffic arriving on `tailscale0`
before `ufw` is consulted, so `ufw` cannot block your MacBook Air from Firmino
over the tailnet. Because Ollama has to listen on the tailnet address (2.4), the
ACL in section 4 is the only thing that keeps other tailnet devices off the
port. `ufw` here protects the LAN side.

### 2.6 Models

- [ ] `ollama pull qwen3:30b-a3b-instruct-2507-q4_K_M` (about 18 GB). Use this
      exact tag: the plain `qwen3:30b` tag is the thinking variant.
- [ ] Run it once and check `ollama ps`: the processor column should say
      `100% CPU`.

### 2.7 Checks

- [ ] From Mane: `curl http://<FIRMINO_TAILSCALE_IP>:11434/api/tags` lists the model.
- [ ] After the ACL in section 4 is applied, from the MacBook Air (not Mane), try
      Firmino's tailnet address on port 11434. It must be **refused or time out**.
- [ ] Reboot Firmino. Confirm Ollama is back without any action.
- [ ] Optional: unplug power briefly and confirm it stays up on battery.

### 2.8 Hardware counters (for the cards on the page)

The page reads Mane's CPU, memory, temperature and network inside the router.
Firmino's numbers come from a small extra process, `pingpong hoststats`. It
serves counters only. It has no page.

- [ ] On Mane, build the Linux binary and copy it to Firmino (placeholders only;
      do not commit the real address):

      ```bash
      cd ~/src/pingpong
      git fetch origin && git checkout main && git pull origin main
      GOOS=linux GOARCH=amd64 go build -o bin/pingpong-linux-amd64 ./cmd/pingpong
      ```

      Copy `bin/pingpong-linux-amd64` to Firmino as `/usr/local/bin/pingpong`.
- [ ] On Firmino, bind hoststats to Firmino's Tailscale address, not `0.0.0.0`:

      ```bash
      /usr/local/bin/pingpong hoststats --listen <FIRMINO_TAILSCALE_IP>:8788
      ```

      Keep it across reboots with a systemd unit whose `ExecStart` is that
      command.
- [ ] On Mane, install the router build, set Firmino's `stats_address`, and
      restart. The address is only in `~/pingpong.local.yaml`, never in the repo.

      ```bash
      cd ~/src/pingpong
      make build-darwin-arm64
      sudo cp bin/pingpong-darwin-arm64 /usr/local/bin/pingpong
      ```

      Under the `firmino` machine:

      ```yaml
      stats_address: "http://<FIRMINO_TAILSCALE_IP>:8788"
      ```

      ```bash
      launchctl kickstart -k "gui/$(id -u)/com.pingpong.router"
      ```

      Mane's own card needs no `stats_address`. The router reads it locally.
- [ ] The Tailscale ACL must allow Mane to reach port 8788 beside 11434. The
      example in [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md) includes
      both. From the MacBook Air, port 8788 on Firmino must be refused.
- [ ] With the page open, the router log gains one `host_sample` line per
      machine about every 5 seconds (Mane, and Firmino once the URL answers).

## 3. Mane (Mac mini)

### 3.1 Basics

- [ ] Install pending macOS updates.
- [ ] Never sleep and recover from power loss:
      `sudo pmset -a sleep 0`, `sudo pmset -a autorestart 1`, and optionally
      `sudo pmset -a womp 1`. Check with `pmset -g`. The display may still sleep.
- [ ] Turn FileVault **off** and enable automatic login for your user
      (System Settings, Users and Groups). If the automatic login option is greyed
      out, FileVault is still on, or the account is set up to log in with an
      iCloud password. Fix that first. FileVault asks for a password at
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
      **Do not also install the Ollama desktop app.** It starts its own server at
      login and holds port 11434, so the launch agent below stays stuck at
      "spawn scheduled" and its settings are silently never applied. If the
      app is installed, quit it, remove it from Login Items (or delete it), then
      run `launchctl kickstart -k gui/$(id -u)/<label>`.
- [ ] Create a user service so Ollama starts at login with the right settings.
      Create a launch agent file (for example in `~/Library/LaunchAgents/`) that
      runs `ollama serve` at load, keeps it alive, and sets these environment
      values. Use the **full path** to the program (Homebrew on Apple silicon is
      `/opt/homebrew/bin/ollama`), because launchd does not use your shell's
      search path:

| Setting | Value | Reason |
|---|---|---|
| `OLLAMA_HOST` | `127.0.0.1:11434` | Local only. Only the router uses it |
| `OLLAMA_NUM_PARALLEL` | `1` | Explicit, because Ollama may otherwise choose a value on Apple GPUs |
| `OLLAMA_MAX_LOADED_MODELS` | `2` | The 8B and the 3B |
| `OLLAMA_KEEP_ALIVE` | `30m` | Tune |
| `OLLAMA_CONTEXT_LENGTH` | `8192` | Explicit context window |
| `OLLAMA_NO_CLOUD` | `1` | Local only |

- [ ] Load the service: `launchctl bootstrap gui/$(id -u) <file>`. If it does not
      start, `launchctl print gui/$(id -u)/<label>` shows why.
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
- [ ] Note the Ollama version on both machines (`ollama --version`) in your own
      notes. They are installed differently and can drift apart. Update them
      deliberately, and re-run the measurements after an update.
- [ ] Simulate a power cut if you can (pull the plug for a few seconds). Confirm
      it starts again by itself.

## 4. Router exposure and access rules (when the router exists)

- [ ] The router listens on `127.0.0.1` only.
- [ ] Publish it to your tailnet with Tailscale Serve: `tailscale serve --bg 8787`
      (use your configured port). Serve only forwards to a `127.0.0.1` target
      and publishes over HTTPS. With `--bg` it keeps running after a reboot.
      Check with `tailscale serve status`. On a Mac, the command is inside the
      Tailscale app (`/Applications/Tailscale.app/Contents/MacOS/Tailscale`)
      unless you installed the command-line tool separately.
- [ ] Set `allowed_hosts` in the router config to Mane's **full** tailnet name
      (`mane.<tailnet>.ts.net`), plus `localhost` and `127.0.0.1`.
- [ ] Apply the Tailscale ACL from [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md) section 4 (the host-based two-rule version, D-081)
      (the default allows everything). Follow its safe order, and keep an SSH
      session to Firmino open while you do it.
- [ ] Check from the MacBook Air that the page loads, and from a device **not**
      on your tailnet (a phone with Tailscale off) that it does not. Also check
      that `tailscale serve status` says "(tailnet only)". Done by the owner (D-080).
- [ ] Check from the MacBook Air that `http://<Firmino tailnet address>:11434` is
      refused or times out (after the ACL is applied).
- [ ] Reboot Mane and confirm `tailscale serve status` still shows the router,
      with no manual step.
- [ ] **macOS Local Network permission.** Recent macOS versions can block programs
      from reaching devices on your home LAN until you allow them. This did not
      affect the Tailscale path used here, but check it if you ever use LAN addresses. After a
      reboot, with the router started by its launch agent (not from Terminal),
      send a request to Firmino through the page. If it fails with
      `MACHINE_DOWN` while the same request works from Terminal, this is the
      likely cause. Check System Settings, Privacy and Security, Local Network.
      The fix may be to run the router from a signed program or add it to that
      list.

## 5. Measure real speeds

The easy way is through the router, using the scripts in `deploy/bench/` on
Mane ([OPERATIONS.md](OPERATIONS.md) section 9), and record the results in
[HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md) section 8. The manual way,
directly on each machine:

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
| `first_token_timeout` | Must exceed the slowest long prompt on Firmino: (largest prompt in tokens) divided by (measured prompt tokens per second), plus load time, plus margin. The default of 30 minutes assumes 5 tokens per second or better at 8,192 tokens. Measured on the 30B model: about 30 to 35 tokens per second, so 8,192 tokens take about 4 to 5 minutes |
| `max_run_time` | Longer than the slowest full answer you expect, so a stuck request is cleared without cutting off real work |
| `idle_token_timeout` | At least several times the slowest gap between tokens |
| `OLLAMA_KEEP_ALIVE` | Long enough to avoid reloads in normal use |
| Context window | Raise above 8192 only if you need it and speed allows |

## 6. Done when

- [ ] Both machines survive a reboot and a short power cut without help.
- [ ] Firmino's Ollama answers Mane and nobody else (checked from the Air over the tailnet, after the ACL is applied).
- [ ] Mane to Firmino traffic uses the Tailscale address: while a request runs, `lsof -i -nP | grep <FIRMINO_TAILSCALE_IP>` on Mane shows the connection.
- [ ] `pingpong doctor --chat` passes for both machines.
- [ ] Mane's Ollama is local only.
- [ ] The MacBook Air reaches Mane over Tailscale.
- [ ] Models are installed and the measurements are recorded.
- [ ] No real addresses have been written into the repository.

## 7. What changed after the real go-live

Differences from the first version of this guide, all learned on the real
machines:

| Was | Now | Why |
|---|---|---|
| Ollama on Firmino bound to its LAN address, Mane connects over the LAN | Bound to its Tailscale address, Mane connects over Tailscale | The machines are on different subnets and the gateway blocks traffic between them (D-072) |
| `ufw` rule to allow Mane's LAN address | No LAN rule. The ACL restricts the port | Tailscale traffic skips `ufw` |
| Firmino model `qwen2.5:14b` | `qwen3:30b-a3b-instruct-2507-q4_K_M` | A mixture-of-experts model is about 5.6 times faster on this CPU (D-073) |
| "Buy matched memory modules" | Not possible | 8 GB of Firmino's memory is soldered |
| Install steps assumed a fresh Ollama | Install script, `ollama` user, models directory, override file | Models pulled as root were invisible to the service |
| Ollama desktop app "may conflict" | Should not be present on Mane. It is still installed today, kept as known tech debt (D-082), so check the port after each reboot | It silently blocked the launch agent (D-074) |
