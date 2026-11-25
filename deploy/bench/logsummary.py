#!/usr/bin/env python3
"""Summarise the router log: speeds, retries, failures, loads, per machine.

Usage: python3 deploy/bench/logsummary.py [LOGFILE] [--since 2026-11-25T14:00]
Default log file: /tmp/pingpong.err. Reads metadata lines only, never content.
"""
import collections, json, statistics, sys

path = "/tmp/pingpong.err"
since = ""
args = sys.argv[1:]
while args:
    a = args.pop(0)
    if a == "--since":
        since = args.pop(0)
    else:
        path = a

events = collections.Counter()
errors = collections.Counter()
by_machine = collections.defaultdict(lambda: collections.defaultdict(list))
for line in open(path, errors="replace"):
    line = line.strip()
    if not line.startswith("{"):
        continue
    try:
        e = json.loads(line)
    except ValueError:
        continue
    if since and e.get("time", "") < since:
        continue
    ev = e.get("event", "")
    events[ev] += 1
    if e.get("error_code"):
        errors[(ev, e.get("machine", "?"), e["error_code"])] += 1
    if ev in ("request_done", "job_done"):
        m = by_machine[e.get("machine", "?")]
        for k in ("output_tokens_per_s", "prompt_tokens_per_s", "load_ms", "queue_wait_ms", "duration_ms"):
            if e.get(k):
                m[k].append(float(e[k]))


def med(xs):
    return "%.1f" % statistics.median(xs) if xs else "-"


print("events:", dict(events) or "none")
for name, m in sorted(by_machine.items()):
    out = m["output_tokens_per_s"]
    print("%-8s n=%-4d out tok/s median %s min %s max %s | in tok/s median %s | loads>0: %d (median %s ms) | queue wait median %s ms | duration median %s ms" % (
        name, len(out), med(out), "%.1f" % min(out) if out else "-", "%.1f" % max(out) if out else "-",
        med(m["prompt_tokens_per_s"]), len(m["load_ms"]), med(m["load_ms"]),
        med(m["queue_wait_ms"]), med(m["duration_ms"])))
if errors:
    print("errors by (event, machine, code):")
    for k, v in errors.most_common():
        print("  %-14s %-8s %-22s %d" % (k[0], k[1], k[2], v))
else:
    print("no error codes logged")
