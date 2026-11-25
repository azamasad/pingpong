#!/bin/sh
# Send one short and one long prompt to a machine and model, print speed.
# Usage: quick.sh MACHINE MODEL   (run on the router host)
# Results are appended to ~/pingpong-bench.log.
set -eu
MACHINE=${1:?machine}
MODEL=${2:?model}
URL=${PINGPONG_URL:-http://127.0.0.1:8787/api/chat}
date >> "$HOME/pingpong-bench.log"
for n in 1 2; do
  python3 -c "import json,sys;n=sys.argv[3];t='Write a 120 word paragraph about how a router chooses a machine.' if n=='1' else 'Summarize in one sentence: '+'The quick brown fox jumps over the lazy dog. '*150;print(json.dumps({'prompt':t,'model':sys.argv[2],'machine':sys.argv[1],'stream':False,'options':{'num_predict':128}}))" "$MACHINE" "$MODEL" "$n" \
    | curl -sS -H 'Content-Type: application/json' --data-binary @- "$URL" \
    | python3 -c "import sys,json;d=json.load(sys.stdin);s=d.get('stats',{});print('$MACHINE','$MODEL','prompt$n',{k:round(v,2) for k,v in s.items() if k in('output_tokens_per_s','prompt_tokens_per_s','load_ms','total_ms')},d.get('error',''))" \
    | tee -a "$HOME/pingpong-bench.log"
done
