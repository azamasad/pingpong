# Network and Security

Status: draft v1.

## 1. Access paths

| Path | Transport | Notes |
|---|---|---|
| MacBook Air to Mane | Tailscale | The only client entry point |
| Mane to Firmino | Home LAN (Ethernet) | Inference traffic. Never via the internet |
| Admin access to Firmino | Tailscale SSH | Maintenance only |
| Future machines to Mane | Home LAN | Same pattern as Firmino |

## 2. Principles

- Reachable only through Tailscale. No public exposure of any port.
- Tailscale Serve is the bridge: the router stays on localhost, and Serve
  publishes it to the tailnet with HTTPS.
- Mane to workers uses LAN addresses directly, not Tailscale, so it does not
  depend on Tailscale or the internet.
- Workers accept inference traffic only from Mane.
- No logins. Tailscale access is the control. The owner is the only user.

## 3. LAN setup

| Item | Requirement |
|---|---|
| Firmino address | Fixed. Use a DHCP reservation on the router (preferred) or a static address. The actual address is not recorded in this public repo |
| Mane address | Fixed in the same way |
| Naming | Mane refers to workers by a stable address or name defined in the registry |

## 4. Firewall rules

| Machine | Rule |
|---|---|
| Firmino | Ollama binds to Firmino's LAN address only (`OLLAMA_HOST`), so it never listens on the tailnet address. |
| Firmino | The firewall allows the inference port only from Mane's LAN address and denies other LAN sources. |
| Firmino | The Tailscale ACL does not allow any tailnet device to reach port 11434 on Firmino (defence in depth). |
| Mane | The router listens on localhost only. Tailscale Serve publishes it to the tailnet, so the LAN cannot reach it directly. |
| Mane | Mane's own Ollama is local only, used by the router. |

**Tailscale bypasses the Linux firewall.** On Linux, Tailscale adds a rule that accepts everything arriving on its own interface before `ufw` runs (this is deliberate, and Tailscale's ACLs are the control for tailnet traffic). So a `ufw` rule can **not** keep your MacBook Air off Firmino's Ollama over the tailnet. The bind address and the Tailscale ACL do that job. `ufw` still protects against other devices on the home LAN.

### Tailscale ACL template (simple, tag-based)

Idea: give each machine a **tag** for its role, and write rules about tags, not
individual machines. Adding a machine later means giving it a tag, with no rule
changes. Anything not allowed is denied. Mane-to-Firmino traffic uses the LAN,
so it needs no tailnet rule.

Paste into the Tailscale admin console, Access controls (replace the
placeholders, and do not commit your real values to this public repo):

```
{
  "tagOwners": {
    "tag:router": ["autogroup:admin"],
    "tag:worker": ["autogroup:admin"]
  },
  "acls": [
    {
      "action": "accept",
      "src": ["autogroup:member"],
      "dst": ["tag:router:443", "tag:worker:22"]
    }
  ],
  "ssh": [
    {
      "action": "accept",
      "src": ["autogroup:member"],
      "dst": ["tag:router", "tag:worker"],
      "users": ["<LINUX_USER>", "<MAC_USER>"]
    }
  ],
  "tests": [
    {
      "src": "<YOUR_EMAIL>",
      "accept": ["tag:router:443", "tag:worker:22"],
      "deny": ["tag:worker:11434", "tag:router:22"]
    }
  ]
}
```

What it says: you (any member of your tailnet) may open Mane's page (443) and SSH
to Firmino (22), and nothing else. Nobody reaches port 11434. The `tests` block
makes the console refuse a policy that does not behave this way.

Safe order (so you cannot lock yourself out):

1. Add only `tagOwners` first, and save.
2. Tag Mane as `tag:router` and Firmino as `tag:worker` (admin console, machine
   menu, edit ACL tags). Tagged machines no longer expire their keys, which suits
   always-on machines.
3. Keep your current SSH session to Firmino open.
4. Replace `acls`, `ssh` and `tests` with the template, and save. If the tests
   fail, the console tells you and does not apply it.
5. Check from the MacBook Air: Mane's page loads, SSH to Firmino works, and
   `http://<Firmino tailnet address>:11434` fails.

A new worker later: install Tailscale, tag it `tag:worker`, done.

Ollama has no built-in authentication, so the firewall and Tailscale are the
protection. Do not rely on obscurity.

### Router access checks

The router has no login, so it adds two cheap checks (see
[API.md](API.md) section 5):

- It accepts only expected `Host` names (Mane's full tailnet name, `localhost` and `127.0.0.1`).
- It rejects `POST` requests with an unexpected `Origin`.

This stops other web pages open in the owner's browser from sending requests
to the router. It does not replace Tailscale.

### Ports

| Port | Where | Exposure |
|---|---|---|
| Router port | Mane | Localhost only. Published to the tailnet by Tailscale Serve |
| Inference port | Firmino | Reachable from Mane only |
| Inference port | Mane | Local only |
| SSH | Firmino | Via Tailscale SSH, key-based, no root login |

Port numbers are set in configuration. Do not record real addresses in this
public repository.

## 5. Tailscale

- The MacBook Air and Mane are on the same tailnet.
- Use access rules (ACLs) so that: your devices can reach Mane's published router
  port (HTTPS 443) and nothing else on Mane; your devices can reach Firmino
  on SSH (port 22) only; nobody reaches port 11434 on Firmino. The default ACL
  allows everything, so this needs editing.
- Tailscale SSH is handled by Tailscale itself, not by `ufw` or `sshd`, so no
  firewall rule is needed for it.
- Firmino stays on Tailscale for administration only. It is not part of the
  inference path.
- Prefer stable machine names (MagicDNS) over IP addresses in notes and config.
- HTTPS through Tailscale Serve needs HTTPS certificates enabled for the tailnet.
  Tailscale's certificates are public, so the machine's tailnet host name appears
  in public certificate logs. The name becomes visible but the machine is still
  unreachable to anyone outside the tailnet. If this bothers you, the fallback is
  plain HTTP on the tailnet (the traffic is already encrypted by Tailscale), at
  the cost of the browser marking the page "not secure" (D-040).

## 6. Hardening Firmino

| Item | Action |
|---|---|
| Root login | Stop working as root. Create a normal user with sudo and use that. Disable direct root login over SSH. |
| Updates | Apply pending updates and reboot (a restart is required at the time of writing). |
| Services | Keep only what is needed (P6). |

## 7. Data and privacy

- Prompts and outputs stay on the owner's machines. Ollama's cloud features are
  switched off (`OLLAMA_NO_CLOUD`), so local models never need ollama.com
  after they are downloaded.
- In V2, saved requests (including prompts) live on Mane's disk for 90 days.
- **FileVault is off on Mane**, so that it can restart unattended after a power
  failure (FileVault asks for a password at startup). The result is that anyone
  with physical access to the Mac could read its disk, including saved prompts.
  Keep it somewhere safe. This is a deliberate trade-off (D-039). If you prefer
  encryption, turn FileVault on and accept that a power failure needs you to
  type a password before Mane comes back.
- The web page loads no external assets, so no third parties see page loads.

## 8. Threats considered

| Threat | Mitigation |
|---|---|
| Another device on the home LAN reaching a worker | Firewall to Mane only |
| Internet exposure | No port forwarding. Tailscale only |
| Tailscale account compromise | Strong account security and two-factor on the Tailscale identity |
| Lost laptop | Remove it from the tailnet and review ACLs |
| Malicious web page sending requests via the owner's browser | Host and Origin checks on the router |
| Prompts leaking via logs | Prompts and outputs are never logged |
| Secrets or addresses committed to the public repository | Commit only placeholders. Real config stays on Mane |

## 9. Out of scope

Multi-user authentication, public sharing, and encryption beyond Tailscale and
disk encryption.

## 10. Open questions

- Which router handles the DHCP reservations.
- Whether macOS blocks the launch-agent router from reaching Firmino (see
  [SETUP-GUIDE.md](SETUP-GUIDE.md) section 3.5).
