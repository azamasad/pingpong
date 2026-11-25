# Network and Security

Status: v2. Reflects the real setup. Mane and Firmino sit on different subnets
and the gateway blocks traffic between them, so they talk over Tailscale
(D-072). The Tailscale ACL in section 4 is **recommended (D-081) and ready to
apply, but not yet applied**, so any tailnet device can still reach Firmino's
Ollama port. The page's tailnet-only exposure was checked by the owner (D-080).

## 1. Access paths

| Path | Transport | Notes |
|---|---|---|
| MacBook Air to Mane | Tailscale | The only client entry point |
| Mane to Firmino | Tailscale address (the home LAN if both machines share a subnet) | Inference traffic. Never via the public internet |
| Admin access to Firmino | Tailscale SSH | Maintenance only |
| Future machines to Mane | Same pattern as Firmino | LAN if on the same subnet, otherwise Tailscale |

## 2. Principles

- Reachable only through Tailscale. No public exposure of any port.
- Tailscale Serve is the bridge: the router stays on localhost, and Serve
  publishes it to the tailnet with HTTPS.
- Mane to workers uses a private address: the LAN address when both are on the
  same subnet, otherwise the worker's Tailscale address. Tailscale paths are
  encrypted and direct on the home network, but depend on Tailscale running on
  both machines. Nothing goes through the public internet. (D-072 amends P7.)
- Workers should accept inference traffic only from Mane. With a Tailscale
  bind this is enforced by the ACL, not by the bind address (section 4).
- Tailscale Funnel stays **off**. Funnel would publish the page to the public
  internet. Check with `tailscale serve status`, which must say
  "(tailnet only)".
- No logins. Tailscale access is the control. The owner is the only user.

## 3. Addresses

| Item | Requirement |
|---|---|
| Firmino address | Its Tailscale address, which is stable for the machine. If you ever use a LAN address instead, reserve it on the router. The actual address is not recorded in this public repo |
| Mane address | Not needed by Firmino. Mane starts every connection |
| Naming | Mane refers to workers by a stable address or name defined in the registry |

## 4. Firewall rules

| Machine | Rule |
|---|---|
| Firmino | Ollama binds to Firmino's Tailscale address (`OLLAMA_HOST`), so it does not listen on the home LAN at all and is reachable only over the tailnet (D-072). It must not bind `0.0.0.0`. |
| Firmino | **Recommended, not yet applied:** the Tailscale ACL allows port 11434 on Firmino from Mane only. Until then, any device on the tailnet can reach Firmino's Ollama port, because the default ACL allows everything. |
| Firmino | `ufw` still guards the LAN side, but it cannot restrict tailnet traffic (below). |
| Mane | The router listens on localhost only. Tailscale Serve publishes it to the tailnet, so the LAN cannot reach it directly. |
| Mane | Mane's own Ollama is local only, used by the router. |

**Tailscale bypasses the Linux firewall.** On Linux, Tailscale adds a rule that accepts everything arriving on its own interface before `ufw` runs (this is deliberate, and Tailscale's ACLs are the control for tailnet traffic). So a `ufw` rule can **not** keep your MacBook Air off Firmino's Ollama over the tailnet. Only the Tailscale ACL can, now that Ollama has to listen on the tailnet address (D-072). This replaces the earlier design (D-054), which kept Ollama on the LAN address and blocked it with the ACL.

### Restrict Firmino's Ollama port to Mane (recommended: do this)

**Decision (D-081): restrict it with a Tailscale ACL.** Ollama has no login, and
Firmino's Ollama must listen on a tailnet address (D-072). With the default ACL
("everything may reach everything"), any device on your tailnet, for example a
phone, a work laptop that joined later, or a shared node, can run models on
Firmino or delete them. A two-rule ACL closes that and costs about ten
minutes. It is a **recommendation until you apply it**. Nothing in the router
changes.

Why this way and not another: `ufw` cannot do it (Tailscale traffic skips it),
binding to `127.0.0.1` would stop Mane reaching it, and a login proxy is more
moving parts than a personal system needs. The ACL below uses host names for
the two machines, so **no re-tagging is needed**. Re-tagging a machine changes
its identity and key handling, and is a bigger step than this needs. A
tag-based version for when you add more machines is given afterwards.

**Rules** (Tailscale admin console, Access controls). Replace the placeholders
with the real Tailscale addresses (`tailscale ip -4` on each machine) in the
console only, never in this repository:

```
{
  "hosts": {
    "mane":    "<MANE_TAILSCALE_IP>",
    "firmino": "<FIRMINO_TAILSCALE_IP>"
  },
  "acls": [
    {
      "action": "accept",
      "src":    ["autogroup:member"],
      "dst":    ["mane:443", "mane:22", "firmino:22"]
    },
    {
      "action": "accept",
      "src":    ["mane"],
      "dst":    ["firmino:11434"]
    }
  ],
  "tests": [
    { "src": "<MANE_TAILSCALE_IP>", "accept": ["<FIRMINO_TAILSCALE_IP>:11434"] },
    { "src": "<AIR_TAILSCALE_IP>",
      "accept": ["<MANE_TAILSCALE_IP>:443", "<FIRMINO_TAILSCALE_IP>:22"],
      "deny":   ["<FIRMINO_TAILSCALE_IP>:11434"] }
  ]
}
```

What it says: every device of yours may open Mane's page (443) and SSH to Mane
and Firmino (22). Only Mane may reach port 11434 on Firmino. Everything else on
the tailnet is denied. Keep your existing `ssh` block and any other blocks as
they are, and change only `hosts`, `acls` and `tests`. Mane is also one of your
devices, so it keeps the same access as the rest, plus the Ollama port. Replies
to connections Mane starts are allowed automatically.

If you also use other ports over the tailnet (for example screen sharing to
Mane), add them to the first rule's `dst`, or they will stop working. If the
console rejects a line in `tests`, delete the `tests` block: the checks in step 6
are the real proof.

**Step by step (about 10 minutes, from the MacBook Air):**

1. Open the Tailscale admin console, Access controls. Copy the whole current
   policy into a text note. This is your rollback (the console also keeps a
   version history).
2. Find the three Tailscale addresses: run `tailscale ip -4` on Mane, on
   Firmino and on the MacBook Air (or read them in the Machines page).
3. Keep one SSH session to Firmino open, in case you need to fix something.
4. Edit the policy: add `hosts`, replace `acls` with the two rules above, add
   `tests`. Leave other blocks alone. Click Save. The console refuses to save if a
   test fails.
5. Wait about 30 seconds.
6. Check, in this order:
   - The Mane page still loads from the MacBook Air (and from your phone).
   - SSH to Mane and to Firmino still works.
   - From the MacBook Air: `curl -m 5 http://<FIRMINO_TAILSCALE_IP>:11434/api/tags`
     must **fail or time out**. (Type the real address, without brackets.)
   - On Mane: `pingpong doctor --chat` still passes for both machines.
   - On Mane: `tailscale serve status` still shows the router.
7. If anything in step 6 fails: Access controls, version history, restore the
   previous policy, then tell the assistant exactly what failed.

When it is done, update the "not yet applied" lines in this document, the
SETUP-GUIDE and the ROADMAP, or ask the assistant to.

### Tailscale ACL template, tag-based (for when you add machines)

Use this only when you have more machines. Give each machine a **tag** for its
role, and write rules about tags, not individual machines. Adding a machine then
means giving it a tag, with no rule changes. Tagging changes how a machine is
owned and how its key expires, so do it deliberately. Paste into Access controls
(replace the placeholders, and do not commit real values to this public repo):

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
      "dst": ["tag:router:443", "tag:router:22", "tag:worker:22"]
    },
    {
      "action": "accept",
      "src": ["tag:router"],
      "dst": ["tag:worker:11434"]
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
      "accept": ["tag:router:443", "tag:router:22", "tag:worker:22"],
      "deny": ["tag:worker:11434"]
    },
    {
      "src": "tag:router",
      "accept": ["tag:worker:11434"]
    }
  ]
}
```

(An earlier version of this template denied SSH to Mane. Fixed: you use SSH on
both machines.) Safe order so you cannot lock yourself out:

1. Add only `tagOwners` first, and save.
2. Tag Mane as `tag:router` and Firmino as `tag:worker` (admin console, machine
   menu, edit ACL tags). Tagged machines no longer expire their keys, which suits
   always-on machines.
3. Keep your current SSH session to Firmino open.
4. Replace `acls`, `ssh` and `tests` with the template, and save. If the tests
   fail, the console tells you and does not apply it.
5. Run the checks in step 6 of the previous section.

A new worker later (tag-based setup): install Tailscale, tag it `tag:worker`, done.

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
| Inference port | Firmino | Tailscale address only. Should be reachable from Mane only (ACL, not yet applied) |
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
- Firmino is on Tailscale for administration **and** is the inference path,
  because the two machines are on different subnets (D-072).
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
- In V2, saved requests (including prompts) live on Mane's disk for 90 days. Planned V7.x versions also keep templates and schedules until deleted, and threads for 90 days after the last message. An exported file or a backup copy holds prompts and outputs: keep it private.
- Planned V7.7: schedules are created from a page with no login, so any device on the tailnet can create one (D-102). The Tailscale ACL (D-081) and keeping only your own devices on the tailnet are the control.
- Planned V7.9: the only outbound call from Mane to anything but the workers' Ollama is one optional webhook with a short message and no content (constitution P8, D-117). The owner accepts a public notification service such as ntfy.sh with a long random topic. The webhook URL is private and lives only in the real config.
- **FileVault is off on Mane**, so that it can restart unattended after a power
  failure (FileVault asks for a password at startup). The result is that anyone
  with physical access to the Mac could read its disk, including saved prompts.
  Keep it somewhere safe. This is a deliberate trade-off (D-039). If you prefer
  encryption, turn FileVault on and accept that a power failure needs you to
  type a password before Mane comes back.
- The web page loads no external assets, so no third parties see page loads.
- **The page has no login.** Anyone whose device is on the tailnet and who is
  allowed by the ACL can open it, and the default ACL allows every device. It
  is not reachable from outside the tailnet: Funnel is off (`tailscale serve
  status` says tailnet only), and **the owner confirmed from a device that is
  not on the tailnet that the page does not load (D-080)**. Re-check after any
  change to Tailscale Serve or Funnel. A login layer or a custom domain behind
  a proxy is possible later and is not planned.

## 8. Threats considered

| Threat | Mitigation |
|---|---|
| Another device on the home LAN reaching a worker | Ollama does not listen on the LAN address |
| Another tailnet device reaching Firmino's Ollama | Tailscale ACL (recommended, not yet applied) |
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

- Applying the ACL in section 4 (owner, about 10 minutes). It is the one
  remaining gap in the Firmino path.
- Whether to add a login layer or custom domain later.
- Which router handles the DHCP reservations, if the machines ever share a
  subnet.
