const $ = (id) => document.getElementById(id);
let history = [];
let aborter = null;
let elapsedTimer = null;
let t0 = 0;

async function refreshMeta() {
  try {
    const [machines, models] = await Promise.all([
      fetch('/api/machines').then((r) => r.json()),
      fetch('/api/models').then((r) => r.json()),
    ]);
    $('machines').textContent = machines
      .map((m) => `${m.name}: ${m.reachable ? 'up' : 'down'}${m.busy ? ', busy' : ''}`)
      .join(' · ');
    const ms = $('machine');
    const md = $('model');
    ms.innerHTML = '<option value="">—</option>';
    md.innerHTML = '<option value="">—</option>';
    machines.forEach((m) => {
      const o = document.createElement('option');
      o.value = m.name;
      o.textContent = m.name;
      ms.appendChild(o);
    });
    models.forEach((m) => {
      const o = document.createElement('option');
      o.value = m.name;
      o.textContent = m.name;
      md.appendChild(o);
    });
  } catch (_) {
    $('machines').textContent = 'Could not load machines';
  }
}

let jobsOn = false;

async function refreshJobs() {
  if (!jobsOn) return;
  try {
    const list = await fetch('/api/jobs?limit=20').then((r) => r.json());
    const ul = $('jobs-list');
    ul.innerHTML = '';
    list.forEach((j) => {
      const li = document.createElement('li');
      li.textContent = `${j.status} · ${j.prompt_preview || j.id}`;
      ul.appendChild(li);
    });
  } catch (_) {}
}

async function refreshHealth() {
  try {
    const h = await fetch('/api/health').then((r) => r.json());
    jobsOn = !!h.jobs;
    $('jobs-panel').classList.toggle('hidden', !jobsOn);
    if (jobsOn) refreshJobs();
  } catch (_) {}
}

setInterval(refreshMeta, 5000);
setInterval(refreshJobs, 8000);
refreshMeta();
refreshHealth();

function setElapsed() {
  if (!t0) return;
  $('elapsed').textContent = `${Math.floor((Date.now() - t0) / 1000)}s`;
}

$('send').onclick = async () => {
  const text = $('prompt').value.trim();
  if (!text) return;
  history.push({ role: 'user', content: text });
  $('prompt').value = '';
  $('out').textContent = '';
  $('err').classList.add('hidden');
  $('warn').classList.add('hidden');
  $('provenance').textContent = '';
  $('stop').disabled = false;
  $('state').textContent = 'Sending…';
  t0 = Date.now();
  elapsedTimer = setInterval(setElapsed, 500);
  aborter = new AbortController();

  const body = { messages: [...history], stream: true };
  if ($('fast').checked && !$('machine').value && !$('model').value) body.fast = true;
  if ($('model').value) body.model = $('model').value;
  if ($('machine').value) body.machine = $('machine').value;

  try {
    const res = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: aborter.signal,
    });
    if (!res.ok) {
      const j = await res.json();
      throw new Error(j.error?.message || res.statusText);
    }
    const reader = res.body.getReader();
    const dec = new TextDecoder();
    let buf = '';
    let answer = '';
    let lastPing = Date.now();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += dec.decode(value, { stream: true });
      let idx;
      while ((idx = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, idx);
        buf = buf.slice(idx + 1);
        if (!line) continue;
        const ev = JSON.parse(line);
        if (ev.type === 'route') {
          $('provenance').textContent = `${ev.machine}, ${ev.model}, ${ev.mode}`;
        } else if (ev.type === 'status') {
          $('state').textContent =
            ev.state === 'waiting' ? `Waiting (${ev.position || 1})` : 'Running';
        } else if (ev.type === 'token') {
          answer += ev.text;
          $('out').textContent = answer;
          $('state').textContent = 'Running';
        } else if (ev.type === 'thinking') {
          $('thinking').textContent = ev.text;
          $('thinking').classList.remove('collapsed');
        } else if (ev.type === 'ping') {
          lastPing = Date.now();
          if (ev.elapsed_ms) t0 = Date.now() - ev.elapsed_ms;
        } else if (ev.type === 'done') {
          $('state').textContent = 'Done';
          if (ev.context_warning) {
            $('warn').textContent = 'Context window may be full; part of the prompt could be cut.';
            $('warn').classList.remove('hidden');
          }
          history.push({ role: 'assistant', content: answer });
        } else if (ev.type === 'error') {
          throw new Error(ev.message || ev.code);
        }
        if (Date.now() - lastPing > 45000) {
          $('state').textContent = 'Connection lost?';
        }
      }
    }
  } catch (e) {
    if (e.name !== 'AbortError') {
      $('err').textContent = e.message;
      $('err').classList.remove('hidden');
      $('state').textContent = 'Failed';
    } else {
      $('state').textContent = 'Stopped';
    }
  } finally {
    clearInterval(elapsedTimer);
    $('stop').disabled = true;
    aborter = null;
  }
};

$('stop').onclick = () => {
  if (aborter) aborter.abort();
};

$('machine').onchange = $('model').onchange = () => {
  if ($('machine').value || $('model').value) $('fast').checked = false;
};