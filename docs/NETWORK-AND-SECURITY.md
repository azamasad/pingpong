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
| Mane | The router listens for client access. Access comes via Tailscale. |
| Mane | Mane's own Ollama is local only, used by the router. |

Ollama has no built-in authentication, so the firewall and Tailscale are the
protection. Do not rely on obscurity.

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

- Prompts and outputs stay on the owner's machines.
- In V2, saved requests (including prompts) live on Mane's disk for 90 days.
  Disk encryption on Mane (FileVault) is recommended.
- The web page loads no external assets, so no third parties see page loads.

## 8. Threats considered

| Threat | Mitigation |
|---|---|
| Another device on the home LAN reaching a worker | Firewall to Mane only |
| Internet exposure | No port forwarding. Tailscale only |
| Tailscale account compromise | Strong account security and two-factor on the Tailscale identity |
| Lost laptop | Remove it from the tailnet and review ACLs |

## 9. Out of scope

Multi-user authentication, public sharing, and encryption beyond Tailscale and
disk encryption.

## 10. Open questions

- Which router handles the DHCP reservations.
- Exact ACL setup in Tailscale.
