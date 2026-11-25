const $ = (id) => document.getElementById(id);
let history = [];
let aborter = null;
let elapsedTimer = null;
let t0 = 0;
let jobsOn = false;
let activeJobId = null;
let selectedJobId = null;
let eventSource = null;

async function refreshMeta() {
  try {
    const [machines, models, aliases] = await Promise.all([
      fetch('/api/machines').then((r) => r.json()),
      fetch('/api/models').then((r) => r.json()),
      fetch('/api/aliases').then((r) => r.json()).catch(() => []),
    ]);
    $('machines').textContent = machines
      .map((m) => `${m.name}: ${m.reachable ? 'up' : 'down'}${m.busy ? ', busy' : ''}`)
      .join(' · ');
    const profiles = await fetch('/api/profiles').then((r) => r.json()).catch(() => []);
    const ps = $('profile');
    const prev = ps.value;
    ps.innerHTML = '<option value="">—</option>';
    (profiles || []).forEach((p) => {
      const o = document.createElement('option');
      o.value = p.name;
      o.textContent = p.name;
      ps.appendChild(o);
    });
    if (prev) ps.value = prev;
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
    (aliases || []).forEach((a) => {
      const o = document.createElement('option');
      o.value = a.name;
      o.textContent = `${a.name} (${a.model})`;
      md.appendChild(o);
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

function buildBody() {
  const body = { messages: [...history], stream: true };
  if ($('profile').value) body.profile = $('profile').value;
  if ($('fast').checked && !$('machine').value && !$('model').value) body.fast = true;
  if ($('model').value) body.model = $('model').value;
  if ($('machine').value) body.machine = $('machine').value;
  if (jobsOn && $('batch-lane').checked) body.lane = 'batch';
  return body;
}

function applyStreamEvent(ev, answerRef) {
  if (ev.type === 'route') {
    const via = ev.alias ? ` via ${ev.alias}` : '';
    $('provenance').textContent = `${ev.machine}, ${ev.model}, ${ev.mode}${via}`;
  } else if (ev.type === 'status') {
    if (ev.state === 'waiting') {
      $('state').textContent = `Waiting (${ev.position || 1})`;
    } else if (ev.state === 'retrying') {
      $('state').textContent = `Retrying (attempt ${ev.attempt || '?'})`;
      if (ev.next_attempt_at) $('state').textContent += ` until ${ev.next_attempt_at}`;
    } else {
      $('state').textContent = ev.state === 'running' ? 'Running' : String(ev.state);
    }
  } else if (ev.type === 'token') {
    answerRef.text += ev.text;
    $('out').textContent = answerRef.text;
    $('state').textContent = 'Running';
  } else if (ev.type === 'thinking') {
    $('thinking').textContent = ($('thinking').textContent || '') + ev.text;
    $('thinking').classList.remove('collapsed');
  } else if (ev.type === 'ping') {
    if (ev.elapsed_ms) t0 = Date.now() - ev.elapsed_ms;
  } else if (ev.type === 'done') {
    $('state').textContent = 'Done';
    if (ev.stats && ev.stats.output_tokens_per_s) {
      const st = ev.stats;
      $('provenance').textContent += ` · ${st.output_tokens_per_s.toFixed(1)} tok/s out` +
        (st.prompt_tokens_per_s ? `, ${st.prompt_tokens_per_s.toFixed(0)} tok/s in` : '') +
        (st.load_ms ? `, load ${(st.load_ms / 1000).toFixed(1)} s` : '');
    }
    if (ev.context_warning) {
      $('warn').textContent = 'Context window may be full; part of the prompt could be cut.';
      $('warn').classList.remove('hidden');
    }
    if (answerRef.addToHistory) history.push({ role: 'assistant', content: answerRef.text });
  } else if (ev.type === 'error') {
    throw new Error(ev.message || ev.code);
  }
}

async function consumeNDJSON(res, answerRef) {
  const reader = res.body.getReader();
  const dec = new TextDecoder();
  let buf = '';
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
      applyStreamEvent(ev, answerRef);
      if (Date.now() - lastPing > 45000 && ev.type === 'ping') lastPing = Date.now();
    }
  }
}

function stopWatching() {
  if (eventSource) {
    eventSource.close();
    eventSource = null;
  }
  if (aborter) aborter.abort();
}

async function watchJobEvents(id, { addToHistory = false } = {}) {
  stopWatching();
  activeJobId = id;
  aborter = new AbortController();
  $('stop').disabled = false;
  const answerRef = { text: $('out').textContent || '', addToHistory };
  try {
    const res = await fetch(`/api/jobs/${id}/events?from=1`, { signal: aborter.signal });
    if (!res.ok) throw new Error('Could not attach to job');
    await consumeNDJSON(res, answerRef);
    refreshJobs();
    if (selectedJobId === id) openJob(id);
  } catch (e) {
    if (e.name !== 'AbortError') {
      $('err').textContent = e.message;
      $('err').classList.remove('hidden');
    }
  } finally {
    $('stop').disabled = true;
    aborter = null;
    activeJobId = null;
  }
}

async function refreshJobs() {
  if (!jobsOn) return;
  const status = $('jobs-filter').value;
  const q = status ? `?limit=30&status=${encodeURIComponent(status)}` : '?limit=30';
  try {
    const list = await fetch(`/api/jobs${q}`).then((r) => r.json());
    const ul = $('jobs-list');
    ul.innerHTML = '';
    list.forEach((j) => {
      const li = document.createElement('li');
      li.className = j.id === selectedJobId ? 'selected' : '';
      const when = j.updated_at ? new Date(j.updated_at).toLocaleString() : '';
      const lane = j.lane && j.lane !== 'interactive' ? `${j.lane} · ` : '';
      li.textContent = `${lane}${j.status} · ${j.prompt_preview || j.id}${when ? ` · ${when}` : ''}`;
      li.onclick = () => openJob(j.id);
      ul.appendChild(li);
    });
  } catch (_) {}
}

async function openJob(id) {
  selectedJobId = id;
  refreshJobs();
  try {
    const j = await fetch(`/api/jobs/${id}`).then((r) => r.json());
    $('job-detail').classList.remove('hidden');
    const bits = [`${j.status}`, j.machine && `${j.machine}/${j.model}`, `attempt ${j.attempt}`];
    if (j.next_attempt_at) bits.push(`next ${j.next_attempt_at}`);
    if (j.error) bits.push(`${j.error.code}: ${j.error.message}`);
    $('job-meta').textContent = bits.filter(Boolean).join(' · ');
    $('job-out').textContent = j.output_text || '';
    const hist = $('job-history');
    hist.innerHTML = '';
    (j.status_history || []).forEach((h) => {
      const li = document.createElement('li');
      li.textContent = `${h.state || h.type || ''} ${h.next_attempt_at || ''}`.trim();
      hist.appendChild(li);
    });
    const active = j.status === 'waiting' || j.status === 'running' || j.status === 'retrying';
    $('job-cancel').style.display = active ? 'inline-block' : 'none';
    $('job-reattach').style.display = active ? 'inline-block' : 'none';
    $('job-delete').style.display = 'inline-block';
  } catch (_) {}
}

async function refreshHealth() {
  try {
    const h = await fetch('/api/health').then((r) => r.json());
    jobsOn = !!h.jobs;
    $('jobs-panel').classList.toggle('hidden', !jobsOn);
    $('lane-wrap').classList.toggle('hidden', !jobsOn);
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

$('jobs-filter').onchange = () => refreshJobs();

$('job-reattach').onclick = () => {
  if (selectedJobId) watchJobEvents(selectedJobId);
};

$('job-cancel').onclick = async () => {
  if (!selectedJobId) return;
  await fetch(`/api/jobs/${selectedJobId}/cancel`, { method: 'POST' });
  refreshJobs();
  openJob(selectedJobId);
};

$('job-delete').onclick = async () => {
  if (!selectedJobId) return;
  await fetch(`/api/jobs/${selectedJobId}`, { method: 'DELETE' });
  $('job-detail').classList.add('hidden');
  selectedJobId = null;
  refreshJobs();
};

$('send').onclick = async () => {
  const text = $('prompt').value.trim();
  if (!text) return;
  history.push({ role: 'user', content: text });
  $('prompt').value = '';
  $('out').textContent = '';
  $('thinking').textContent = '';
  $('thinking').classList.add('collapsed');
  $('err').classList.add('hidden');
  $('warn').classList.add('hidden');
  $('provenance').textContent = '';
  $('state').textContent = 'Sending…';
  t0 = Date.now();
  elapsedTimer = setInterval(setElapsed, 500);

  const body = buildBody();

  try {
    if (jobsOn) {
      const res = await fetch('/api/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const j = await res.json();
        throw new Error(j.error?.message || res.statusText);
      }
      const { id } = await res.json();
      refreshJobs();
      await watchJobEvents(id, { addToHistory: true });
      return;
    }
    const res = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const j = await res.json();
      throw new Error(j.error?.message || res.statusText);
    }
    $('stop').disabled = false;
    await consumeNDJSON(res, { text: '', addToHistory: true });
    $('stop').disabled = true;
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
    aborter = null;
    $('stop').disabled = true;
  }
};

$('stop').onclick = async () => {
  if (jobsOn && activeJobId) {
    await fetch(`/api/jobs/${activeJobId}/cancel`, { method: 'POST' });
    stopWatching();
    $('state').textContent = 'Cancelled';
    refreshJobs();
    return;
  }
  stopWatching();
};

$('machine').onchange = $('model').onchange = () => {
  if ($('machine').value || $('model').value) $('fast').checked = false;
};

let statusTimer = null;
let stressTimer = null;

async function refreshStatus() {
  try {
    const s = await fetch('/api/status').then((r) => r.json());
    $('status-out').textContent = JSON.stringify(s, null, 2);
  } catch (_) {
    $('status-out').textContent = 'Could not load status';
  }
}

function showTab(which) {
  for (const name of ['chat', 'status', 'stress']) {
    $('panel-' + name).classList.toggle('hidden', name !== which);
    $('tab-' + name).classList.toggle('active', name === which);
  }
  if (statusTimer) {
    clearInterval(statusTimer);
    statusTimer = null;
  }
  if (which === 'status') {
    refreshStatus();
    statusTimer = setInterval(refreshStatus, 5000);
  }
  if (stressTimer) {
    clearTimeout(stressTimer);
    stressTimer = null;
  }
  if (which === 'stress') refreshStress();
}

function fmtTps(n) {
  return n ? n.toFixed(2) : '—';
}

function renderStress(s) {
  const lines = [];
  lines.push(s.running ? `Running until ${s.until || '?'}` : 'Idle');
  if (s.target) lines.push(`Target: ${s.target}`);
  if (s.queued) lines.push(`Still in queue: ${s.queued}`);
  if (s.last_err) lines.push(`Last error: ${s.last_err}`);
  const machines = s.machines || {};
  const names = Object.keys(machines).sort();
  if (!names.length) lines.push('No finished jobs yet.');
  names.forEach((name) => {
    const m = machines[name];
    lines.push(
      `${name}: ${m.done} done, ${m.failed} failed, ` +
      `output ${fmtTps(m.output_tps_median)} tok/s ` +
      `(min ${fmtTps(m.output_tps_min)}, max ${fmtTps(m.output_tps_max)}), ` +
      `prompt ${fmtTps(m.prompt_tps_median)} tok/s, ` +
      `load ${m.load_ms_median || 0} ms, total ${m.total_ms_median || 0} ms`
    );
  });
  $('stress-out').textContent = lines.join('\n');
  $('stress-state').textContent = s.running ? 'running' : '';
}

async function refreshStress() {
  try {
    const s = await fetch('/api/stress').then((r) => r.json());
    renderStress(s);
    if (stressTimer) clearTimeout(stressTimer);
    if (s.running && !$('panel-stress').classList.contains('hidden')) {
      stressTimer = setTimeout(refreshStress, 3000);
    }
  } catch (e) {
    $('stress-out').textContent = e.message;
  }
}

$('stress-start').onclick = async () => {
  $('stress-state').textContent = 'Starting…';
  const res = await fetch('/api/stress', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      target: $('stress-target').value,
      seconds: Number($('stress-seconds').value),
    }),
  });
  const body = await res.json();
  if (!res.ok) {
    $('stress-state').textContent = body.error?.message || 'Could not start';
    return;
  }
  renderStress(body);
  refreshStress();
};

$('stress-stop').onclick = async () => {
  const res = await fetch('/api/stress', { method: 'DELETE' });
  renderStress(await res.json());
};

$('tab-chat').onclick = () => showTab('chat');
$('tab-status').onclick = () => showTab('status');
$('tab-stress').onclick = () => showTab('stress');