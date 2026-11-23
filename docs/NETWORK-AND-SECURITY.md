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
| Firmino | Allow the inference port only from Mane's LAN address. Deny all other sources. |
| Firmino | Inference service listens on the LAN interface, not on every interface. |
| Mane | The router listens on localhost only. Tailscale Serve publishes it to the tailnet, so the LAN cannot reach it directly. |
| Mane | Mane's own Ollama is local only, used by the router. |

Ollama has no built-in authentication, so the firewall and Tailscale are the
protection. Do not rely on obscurity.

### Router access checks

The router has no login, so it adds two cheap checks (see
[API.md](API.md) section 5):

- It accepts only expected `Host` names (Mane's Tailscale name and localhost).
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
- Use access rules (ACLs) so only the owner's devices can reach Mane's router.
- Firmino stays on Tailscale for administration only. It is not part of the
  inference path.
- Prefer stable machine names (MagicDNS) over IP addresses in notes and config.

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
- Exact Tailscale ACL rules (see SETUP-GUIDE section 4 for what they must achieve).
- Exact `tailscale serve` syntax for the installed version.
