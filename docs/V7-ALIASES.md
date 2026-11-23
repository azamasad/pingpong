# V7: Model aliases

Status: **built (M16–M19).** M20 (owner on real machines) remains.

Related: [ROADMAP.md](ROADMAP.md) phase 7, [ROUTING.md](ROUTING.md),
[CONFIGURATION.md](CONFIGURATION.md), [API.md](API.md), decision D-070.

## 1. Why

Model tags are long and change over time (`qwen2.5-coder:14b-instruct-q5_K_M`).
Profiles (V5) and the owner's own scripts hard-code them. An alias is a short
name for a model, defined once in config:

```text
code  ->  qwen2.5-coder:14b
```

Change the model behind `code` in one place and every client, profile and batch
script follows. Aliases do not change how routing works. They are a name
lookup, not a new routing mode.

## 2. Config

```yaml
aliases:
  code:
    model: "qwen2.5-coder:14b"
  quick:
    model: "llama3.2:3b"
    machine: mane
```

| Key | Required | Meaning |
|---|---|---|
| `model` | yes | The real Ollama model tag |
| `machine` | no | Pin to this machine as well. Must be a configured machine name |

Default: no aliases. Behaviour with none configured is exactly V5.

### Rules checked at load time

- Alias names are lowercase letters, digits, `-` and `_`. **No `:` and no `/`.**
  Real Ollama tags with a version always contain `:`, so this keeps almost all
  names apart from aliases.
- No alias may point at another alias. No chains (P1, P9).
- `model` must not be empty. `machine`, if set, must exist.
- A profile's `model` may be an alias name. It is expanded the same way.
- A profile's `machine` must exist. The loader does not check this today
  (`machine: typo` in a profile fails only when a request uses it), so V7
  adds the check while it is already validating aliases.
- An alias whose name equals a model installed on any machine (an untagged
  model such as `mistral`) is **not** a load error, because model lists are
  not known at load time. It becomes a warning from `pingpong doctor` (V6) and
  a startup log warning. The alias wins (see section 3).

## 3. Resolution

Aliases are expanded by one function, called right after profiles are applied
and before routing. Live chat and jobs both call it. Jobs call it at submit
and again at each attempt, which is where profiles are applied today, so the
two cannot drift apart:

1. Apply the profile, as in V5. An explicit `fast`, `model` or `machine` on the
   request still wins over the profile.
2. If `model` (from the request or the profile) equals an alias name:
   - `model` becomes the alias's `model`.
   - If the alias has a `machine` and the request did not set one, `machine`
     becomes the alias's `machine`.
   - If the request set a different `machine` from the alias's, the request's
     machine wins (explicit beats configured, P9) and the alias only supplies
     the model.
3. Routing then runs unchanged on the resolved values.

An alias is a pin. Everything in [ROUTING.md](ROUTING.md) that applies to a
pinned model applies to it:

| Situation | Result |
|---|---|
| Model installed on exactly one machine | Routed there, mode `pinned_model` |
| Alias has a `machine` | Mode `pinned_model_and_machine` |
| Model installed on several machines, no machine | `MODEL_AMBIGUOUS` (P10: no guessing) |
| Model installed nowhere | `MODEL_NOT_INSTALLED` |
| Machine never reached | `MODELS_UNKNOWN` (retryable) |
| Machine down | `MACHINE_DOWN`, retried in V2 jobs as today |

An alias name that is not defined and is not a real model is reported as the
real name: `MODEL_NOT_INSTALLED`. There is no fuzzy matching and no "did you
mean" (P2).

### Jobs and retries

The request body is stored as sent, so the alias name stays visible in history.
The alias is expanded when a job starts each attempt. A config change takes
effect only after a router restart (there is no hot reload), and then applies
to queued and retrying jobs on their next attempt. This is the same behaviour
profiles already have.

## 4. API changes

All changes add fields. Nothing is removed or renamed, so V1 to V5 clients work
unchanged.

| Where | Change |
|---|---|
| `POST /api/chat`, `POST /api/jobs`, batch items | `model` may be an alias name. No new field |
| `route` event and job detail | New optional field `alias`, present only when an alias was used. `model` always shows the resolved real tag (P11) |
| `GET /api/aliases` (new) | List of `{name, model, machine, installed, machines[]}`. `installed` and `machines` come from the last known model lists |
| `GET /api/profiles` | A profile's `model` is returned as written (it may be an alias) |

`GET /api/aliases` never returns a prompt, output or machine address. It is
subject to the same Host and Origin checks as every endpoint.

## 5. Web page and CLI

- The existing model dropdown (filled from `GET /api/models`) also gets
  `GET /api/aliases`. It lists aliases first, labelled with the real model, for
  example `code (qwen2.5-coder:14b)`, then the real model tags. The profile
  selector from V5 is unchanged.
- An alias that is not installed anywhere is shown greyed out with the reason.
- The route line already shows the resolved model. It adds `via code` when an
  alias was used.
- `pingpong batch` needs no change: an alias in an item's `model` field works.

## 6. Not in scope

- Per-alias options (temperature, context size). Options stay on the request
  (D-046 still fixes `num_ctx`).
- Alias chains, wildcards or fuzzy matching.
- Pulling or installing models. Aliases only name what is already installed.
- Different models per time of day or load. Aliases are static.

## 7. Milestones

| ID | Who | Work | Done when |
|---|---|---|---|
| M16 | Agent | Config: `aliases`, validation (including the profile `machine` check), example in `deploy/config.example.yaml`, and the config doc | Tests for each load rule in section 2 |
| M17 | Agent | Resolution after profiles, before routing; `alias` in route event and job detail | Tests for every row in sections 3 and 4, including the machine conflict rule |
| M18 | Agent | `GET /api/aliases`, page picker, `via` label | API and UI tests. A page without aliases looks like V5 |
| M19 | Agent | Doctor warnings: an alias equals an installed model name, or an alias's model is installed nowhere | Needs V6 M11a. Test with the fake Ollama. Machine checks are already load errors |
| M20 | Owner | Define two real aliases on Mane and Firmino and use them from the page and a batch | Resolved model shown correctly. Missing alias target gives a clear error |

## 8. Automated tests

Added to [TESTING.md](TESTING.md) when built.

- Config: invalid name (with `:`), alias to alias, empty model, unknown machine
  (in an alias or a profile) are each rejected. No aliases means no behaviour change from V5.
- Resolution table: one test per row in section 3, run through `POST /api/chat`
  and through a job.
- Explicit request `machine` beats the alias's `machine`. Explicit request
  `model` that is not an alias is untouched.
- Profile `model` set to an alias is expanded.
- `route` event carries `alias` only when used. `model` is the real tag.
- `GET /api/aliases` reports `installed` and `machines` correctly, and a model
  on several machines is listed with all of them.
- An alias with the same name as an installed untagged model resolves to the
  alias and produces the warning.
- Stored job request keeps the alias name. Memory budget (N1, N17) unchanged.

## 9. Open questions

- Whether to allow an alias to name a **list** of acceptable models for
  fallback. Not planned. It would blur "pinned means pinned" (P10) and should
  only be considered if real use shows a need.
- Whether `GET /api/models` should also return aliases. Kept separate for now
  so existing clients see no change.
