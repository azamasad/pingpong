"""Pingpong quality and speed comparison.

Sends the same prompts to each contestant (machine:model), hides which model
wrote which answer, and records speed. Run it on the router host.

Environment:
  PINGPONG_URL          chat endpoint (default http://127.0.0.1:8787/api/chat)
  PINGPONG_CONTESTANTS  comma list of machine:model, for example
                        mane:llama3.1:8b,firmino:qwen3:30b-a3b-instruct-2507-q4_K_M
Optional ~/my-prompts.txt adds one prompt per line. Outputs go to ~/.
"""
import json, os, random, statistics, time, urllib.request

URL = os.environ.get("PINGPONG_URL", "http://127.0.0.1:8787/api/chat")
HOME = os.path.expanduser("~")
DEFAULT_CONTESTANTS = "mane:llama3.1:8b,firmino:qwen3:30b-a3b-instruct-2507-q4_K_M"
CONTESTANTS = [
    tuple(item.split(":", 1))
    for item in os.environ.get("PINGPONG_CONTESTANTS", DEFAULT_CONTESTANTS).split(",")
    if item.strip()
]
ARTICLE = (
    "The city council voted on Tuesday to replace the aging downtown bridge. "
    "The project will cost 42 million dollars and take about three years. "
    "During construction one lane will stay open in each direction, but the "
    "pedestrian walkway will close for the first eight months. Local shop "
    "owners worry about fewer customers, while commuters welcome a safer "
    "crossing. The council promised weekly updates and a small fund to help "
    "affected businesses."
)


def words(t):
    return len(t.split())


def lines(t):
    return [x for x in t.strip().splitlines() if x.strip()]


PROMPTS = [
    ("math", "A train leaves at 2:15 pm and travels 150 km at 60 km/h. At what time does it arrive? Show the working briefly.",
     lambda t: "4:45" in t),
    ("format", "List exactly three benefits of regular exercise. Output exactly three lines, each starting with '- ', and nothing else.",
     lambda t: len(lines(t)) == 3 and all(x.startswith("- ") for x in lines(t))),
    ("summary", "Summarize this in one sentence of at most 25 words:\n\n" + ARTICLE,
     lambda t: words(t) <= 25),
    ("code", "Write a Python function is_palindrome(s) that ignores case and non-letter characters. Return only the code.", None),
    ("email", "Write a polite four-sentence email asking my landlord to fix a leaking kitchen tap.", None),
    ("explain", "Explain in plain words why the sky is blue, in under 100 words.",
     lambda t: words(t) < 100),
]
mine = os.path.join(HOME, "my-prompts.txt")
if os.path.exists(mine):
    for i, line in enumerate(open(mine), 1):
        if line.strip():
            PROMPTS.append(("mine%d" % i, line.strip(), None))


def ask(machine, model, prompt):
    body = json.dumps({
        "prompt": prompt, "machine": machine, "model": model, "stream": False,
        "options": {"num_predict": 220, "temperature": 0},
    }).encode()
    req = urllib.request.Request(URL, body, {"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=3600) as r:
            d = json.load(r)
    except Exception as e:
        return "", {}, str(e)
    return d.get("text", ""), d.get("stats", {}), json.dumps(d.get("error", "")) if d.get("error") else ""


results = {}
for ci, (machine, model) in enumerate(CONTESTANTS):
    print("running", machine, model, flush=True)
    for pi, (name, prompt, check) in enumerate(PROMPTS):
        t0 = time.time()
        text, stats, err = ask(machine, model, prompt)
        ok = None
        if check is not None and not err:
            ok = bool(check(text))
        results[(pi, ci)] = (text, stats, err, ok)
        print("  %-8s %5.0fs %s" % (name, time.time() - t0, err or ("ok" if ok is None else ("pass" if ok else "FAIL"))), flush=True)

rng = random.Random(time.time())
out_path = os.path.join(HOME, "pingpong-quality.txt")
key_path = os.path.join(HOME, "pingpong-quality-key.txt")
labels = "ABCDEFGH"
with open(out_path, "w") as out, open(key_path, "w") as key:
    for pi, (name, prompt, check) in enumerate(PROMPTS):
        order = list(range(len(CONTESTANTS)))
        rng.shuffle(order)
        out.write("=" * 70 + "\nPROMPT %d (%s): %s\n" % (pi + 1, name, prompt))
        key.write("PROMPT %d (%s)\n" % (pi + 1, name))
        for pos, ci in enumerate(order):
            text, stats, err, ok = results[(pi, ci)]
            out.write("\n--- Answer %s ---\n%s\n" % (labels[pos], text.strip() or "(none: %s)" % err))
            key.write("  %s = %s on %s  words=%d  check=%s\n" % (
                labels[pos], CONTESTANTS[ci][1], CONTESTANTS[ci][0], words(text),
                "n/a" if ok is None else ("pass" if ok else "FAIL")))
        out.write("\n")

print("\nSPEED AND AUTO-CHECKS")
summary = []
for ci, (machine, model) in enumerate(CONTESTANTS):
    rows = [results[(pi, ci)] for pi in range(len(PROMPTS))]
    tps = [r[1].get("output_tokens_per_s") for r in rows if r[1].get("output_tokens_per_s")]
    pps = [r[1].get("prompt_tokens_per_s") for r in rows if r[1].get("prompt_tokens_per_s")]
    tot = [r[1].get("total_ms") / 1000 for r in rows if r[1].get("total_ms")]
    checked = [r[3] for r in rows if r[3] is not None]
    errs = sum(1 for r in rows if r[2])
    line = "%-38s write %5.2f tok/s  read %6.1f tok/s  median reply %5.1fs  checks %d/%d  errors %d" % (
        model, statistics.median(tps) if tps else 0, statistics.median(pps) if pps else 0,
        statistics.median(tot) if tot else 0, sum(checked), len(checked), errs)
    print(line)
    summary.append(line)
with open(os.path.join(HOME, "pingpong-bench.log"), "a") as f:
    f.write(time.strftime("quality run %Y-%m-%d %H:%M\n") + "\n".join(summary) + "\n")
print("\nRead the answers:  cat ~/pingpong-quality.txt")
print("Reveal who wrote them after you pick:  cat ~/pingpong-quality-key.txt")
