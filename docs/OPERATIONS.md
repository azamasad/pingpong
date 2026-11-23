# Operations

Status: draft v1. A runbook of what to prepare and how to keep both machines
healthy. Both machines are assumed always on.

## 1. Mane (Mac mini)

| Item | Setting or action |
|---|---|
| Sleep | Never sleep the computer. Display may sleep |
| Power failure | Enable automatic startup after a power failure |
| Start on boot | Router and Ollama start automatically after a reboot, without a manual login step if possible |
| Updates | Install macOS updates deliberately, then confirm the services come back |
| Disk | Free space. Keep only the 3B and 8B models plus room for V2 data |
| Model memory | Set Ollama's keep-alive so the 8B stays loaded when useful, but unloads when idle if memory is needed |
| Encryption | Enable FileVault (see security notes) |

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
- How services are supervised on Mane.
