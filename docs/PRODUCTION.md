# Production go-live (V1 to V5)

Status: for the owner. Automated CI covers unit and integration tests against a
fake Ollama, and the real binary has been smoke-tested against stand-in Ollama
servers. This is what remains on the real machines before calling V1 to V5
production.

## 1. Prerequisites

- Phase 0 complete: [SETUP-GUIDE.md](SETUP-GUIDE.md)
- Tailscale ACL applied: [NETWORK-AND-SECURITY.md](NETWORK-AND-SECURITY.md)
- Models installed on Mane and Firmino per [HARDWARE-AND-MODELS.md](HARDWARE-AND-MODELS.md)

## 2. Build and install on Mane

```bash
git clone https://github.com/azamasad/pingpong.git
cd pingpong
make build-darwin-arm64
```

Copy `bin/pingpong-darwin-arm64` to a permanent path (for example
`/usr/local/bin/pingpong`). Copy `deploy/config.example.yaml` to a path **outside**
the repo (for example `~/pingpong.local.yaml`) and fill in real addresses only
there.

### Turn on saved requests (V2 to V5)

Jobs, retries, the batch queue and history need a data folder. They are off
until `storage.data_dir` is set.

- Use an **absolute path** that exists on Mane's internal disk, for example
  `/Users/<you>/Library/Application Support/pingpong`. Do not rely on `~`.
- Keep it out of the repository and out of any synced folder.
- Saved requests hold prompts and outputs, so the folder is private data.
- The router creates it if missing and keeps a SQLite file in it.

Profiles need nothing: `fast`, `balanced` and `deep` exist by default. Add a
`profiles:` block only to change them.

### Launch agent

Install the launch agent from `deploy/com.pingpong.router.plist.example`
with absolute paths. Load it at login.

Ollama and the router start together at login. The router checks its machines
at start and then every 30 seconds (`health.interval`), so if Ollama is a few
seconds late, requests may fail for up to one health interval and then work
without a restart. This holds once the deploy-fix change
([PR #8](https://github.com/azamasad/pingpong/pull/8)) is merged. Before it,
the wait could be up to 5 minutes. Merge that change before deploying.

Publish:

```bash
tailscale serve --bg 8787
tailscale serve status
```

## 3. Verify

### 3.1 Basics

1. `curl -H 'Host: localhost' http://127.0.0.1:8787/api/health` on Mane. Expect
   `"ok":true` and `"jobs":true` when saved requests are on.
2. `curl -H 'Host: localhost' http://127.0.0.1:8787/api/machines`. Both machines
   should show `reachable: true` and list their models.
3. Open the page from the MacBook Air over Tailscale.

### 3.2 Real Ollama contract (never checked yet, D-052)

This is the largest unverified risk. Everything so far used a fake Ollama.

1. Send one auto request and one `fast` request from the page. Both must stream
   text and finish with a `done` line showing token counts.
2. Send one request to a model you have **not** installed. Expect
   `MODEL_NOT_INSTALLED`, not a hang.
3. If anything looks wrong (no text, no `done`, wrong token counts), stop and
   report the raw output. The router's reading of Ollama's stream is the
   likeliest place for a real-machine surprise.

V6's `pingpong doctor --chat` will automate this check
([V6-OPERABILITY.md](V6-OPERABILITY.md)).

### 3.3 V2 to V5 behaviours

1. **Survives the client.** Submit a long job, close the page, reopen it and
   see the result in history.
2. **Restart recovery.** Submit a job to Firmino, restart the router while it
   runs, and confirm it resumes (it restarts from the beginning, with the
   earlier partial output kept as history).
3. **Retry.** Stop Ollama on Firmino, submit a job, and confirm it shows
   `retrying`. Start Ollama again within a few minutes and confirm it finishes.
4. **Batch.** `pingpong batch submit` with two items. Check `pingpong batch get`
   until `pending` is 0.
5. **Lanes.** While a batch runs on a machine, send an interactive request.
   It should go next, ahead of waiting batch items.
6. **Profiles.** Choose `fast` in the page. The route line should show Mane.

### 3.4 Acceptance and memory

1. Run the manual acceptance tests in [TESTING.md](TESTING.md) section 4 (M10).
2. Confirm router RSS under 50 MB on Mane, with saved requests on (M9b). A
   Linux smoke run measured about 15 MB.

## 4. Backups

Until V6 adds `pingpong backup`, saved requests are protected only by Mane's own
disk. If they matter, stop the router and copy the data folder now and then.
Copies are private data. Never put them in the repository.

## 5. Rollback

- Stop the launch agent
- `tailscale serve reset` if needed
- Firmino and Mane Ollama keep running independently
- The data folder can stay. A later start picks up where it left off

## 6. After go-live

- Record measured speeds in HARDWARE-AND-MODELS section 8
- Tune `first_token_timeout` in local config if Firmino is slower than 5 tok/s
- Send the results of section 3.2 back. They decide what V6 (and the
  rest of the roadmap) must change.
