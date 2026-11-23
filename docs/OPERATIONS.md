# Operations

Status: draft v1. A runbook of what to prepare and how to keep both machines
healthy. Both machines are assumed always on.

## 1. Mane (Mac mini)

| Item | Setting or action |
|---|---|
| Sleep | Never sleep the computer. Display may sleep |
| Power failure | Enable automatic startup after a power failure |
| Start on boot | Ollama and the router run as user launch agents that start at automatic login, with keep-alive so they restart if they crash |
| Updates | Install macOS updates deliberately, then confirm the services come back |
| Disk | Free space. Keep only the 3B and 8B models plus room for V2 data |
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
| Inference service | Runs as a managed service that starts on boot and restarts if it fails |
| Firewall | Allow the inference port only from Mane. See [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md) |
| Disk | Free space. Decide on reclaiming the unused Windows partitions |

## 2a. Ollama settings on each machine

Ollama is configured through environment settings of its service. The full
list, with reasons, is in [CONFIGURATION.md](CONFIGURATION.md) section 3, and
the steps are in [SETUP-GUIDE.md](SETUP-GUIDE.md). In brief:

| Machine | Setting | Value |
|---|---|---|
| Firmino | Listen address | All interfaces on the Ollama port, with the firewall allowing only Mane |
| Firmino | Parallel requests | 1 |
| Firmino | Loaded models | 1 (more only if memory allows and is needed) |
| Firmino | Keep-alive | Long enough that the 14B is not reloaded between requests. Tune |
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
| Updates | Replace the program, restart the service, and confirm it comes up (V1 loses active requests on restart) |
| Memory check | Look at the router's memory now and then against the 50 MB working budget |

## 3. Health checks

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
| "Firmino is down" | Is Firmino on and on the network? Is Ollama running there? Is the firewall allowing Mane? |
| "Model not installed" | Is it installed on that machine? Does the name match exactly (including the tag)? |
| A request waits a long time | Is another request running on that machine? Firmino on a long prompt can take minutes |
| First answer is slow | The model may be loading from disk. Later requests are faster while it stays loaded |
| Mane feels sluggish | Check memory pressure. Is a model larger than 8B loaded? |
| Firmino very hot or loud | Check thermals and ventilation. Consider limiting load |

## 5. After a reboot or power loss

1. Confirm both machines are on the network.
2. Confirm Ollama is up on each.
3. Confirm the router is up on Mane.
4. In V2, confirm waiting and retrying requests resumed.

## 6. V2 data on Mane

- Saved requests live on Mane's disk. Keep an eye on size.
- Results older than 90 days are removed automatically.
- Optional: copy the saved data elsewhere now and then if it becomes valuable.

## 7. Adding a machine (later)

1. Give it a fixed LAN address.
2. Install and secure its inference service. Firewall it to Mane only.
3. Register it with a role and capabilities.
4. Confirm it shows as reachable and its models are listed.
5. Update [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md).

## 8. Open questions

- Which BIOS options exist on the ThinkBook.
- None on service supervision: Ollama and the router run as user launch agents
  (see [SETUP-GUIDE.md](SETUP-GUIDE.md)).
