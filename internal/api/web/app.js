const $ = (id) => document.getElementById(id);
let history = [];
let aborter = null;
let elapsedTimer = null;
let t0 = 0;
let jobsOn = false;
let activeJobId = null;
let selectedJobId = null;
let eventSource = null;
let templateTimer = null;
let selectedTemplateName = null;
let templateUseVars = [];
let threadTimer = null;
let selectedThreadId = null;
let threadEventSource = null;
let scheduleTimer = null;
let selectedScheduleId = null;
let currentChatThreadId = null;
let streamTarget = null;
let chatReady = false;

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
  const cm = $('check-mode');
  if (jobsOn && cm && cm.value) {
    body.check = { mode: cm.value };
    if ($('check-machine').value) body.check.machine = $('check-machine').value;
    if ($('check-model').value) body.check.model = $('check-model').value;
  }
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
    (streamTarget || $('out')).textContent = answerRef.text;
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

async function watchJobEvents(id, { addToHistory = false, reloadChat = false } = {}) {
  stopWatching();
  activeJobId = id;
  aborter = new AbortController();
  $('stop').disabled = false;
  if (streamTarget) streamTarget.textContent = '';
  const answerRef = { text: streamTarget ? '' : ($('out').textContent || ''), addToHistory };
  try {
    const res = await fetch(`/api/jobs/${id}/events?from=1`, { signal: aborter.signal });
    if (!res.ok) throw new Error('Could not attach to job');
    await consumeNDJSON(res, answerRef);
    refreshJobs();
    if (selectedJobId === id) openJob(id);
    if (reloadChat && currentChatThreadId) {
      const openID = currentChatThreadId;
      streamTarget = null;
      await openChatThread(openID, { attach: false });
    }
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
    const hasCheck = j.check && j.stage === 'answer';
    $('job-draft').classList.toggle('hidden', !hasCheck);
    $('job-review').classList.toggle('hidden', !hasCheck);
    $('job-out').classList.toggle('hidden', !!hasCheck);
    $('check-note').classList.add('hidden');
    if (hasCheck) {
      $('job-draft-meta').textContent = `${j.machine} / ${j.model}`;
      $('job-draft-out').textContent = j.output_text || '';
      const childId = j.check.child_id;
      if (childId) {
        fetch(`/api/jobs/${childId}`).then((r) => r.json()).then((c) => {
          $('job-review-meta').textContent = `${c.machine} / ${c.model} · ${c.status}`;
          $('job-review-out').textContent = c.output_text || '';
          fetch('/api/machines').then((r) => r.json()).then((machines) => {
            const ansM = machines.find((m) => m.name === j.machine);
            const revM = machines.find((m) => m.name === c.machine);
            if (ansM && revM && ansM.models && revM.models) {
              const ansSz = ansM.models.find((x) => x.name === j.model)?.parameter_size;
              const revSz = revM.models.find((x) => x.name === c.model)?.parameter_size;
              if (ansSz && revSz && parseFloat(revSz) < parseFloat(ansSz)) {
                $('check-note').textContent = 'Reviewer model is smaller than the answer model.';
                $('check-note').classList.remove('hidden');
              }
            }
          }).catch(() => {});
        }).catch(() => {});
      } else {
        $('job-review-meta').textContent = `check: ${j.check.state}`;
        $('job-review-out').textContent = '';
      }
    } else {
      $('job-out').textContent = j.output_text || '';
    }
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
    $('tab-batches').classList.toggle('hidden', !jobsOn);
    $('tab-templates').classList.toggle('hidden', !jobsOn);
    $('tab-threads').classList.toggle('hidden', !jobsOn);
    $('tab-schedules').classList.toggle('hidden', !jobsOn);
    $('check-wrap').classList.toggle('hidden', !jobsOn);
    $('check-machine-wrap').classList.toggle('hidden', !jobsOn);
    $('check-model-wrap').classList.toggle('hidden', !jobsOn);
    $('chat-threadbar').classList.toggle('hidden', !jobsOn);
    $('chat-log').classList.toggle('hidden', !jobsOn);
    $('chat-trim-wrap').classList.toggle('hidden', !jobsOn);
    $('chat-route-hint').classList.toggle('hidden', !jobsOn);
    syncChatOut();
    if (jobsOn) {
      refreshJobs();
      fillBatchModelSelects();
      fillBatchReduceSelectors();
      fillCheckSelectors();
      if (!chatReady) {
        chatReady = true;
        const saved = sessionStorage.getItem('pingpong.chatThread');
        await refreshChatThreads();
        if (saved) await openChatThread(saved, { attach: true });
        else startNewChat();
      }
    }
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
  if (jobsOn && !$('batch-lane').checked) {
    await sendChatThread(text);
    return;
  }
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

let selectedBatchId = null;
let batchTimer = null;

async function fillCheckSelectors() {
  try {
    const machines = await fetch('/api/machines').then((r) => r.json());
    const cm = $('check-machine');
    const md = $('check-model');
  if (!cm || !md) return;
    const prevM = cm.value;
    cm.innerHTML = '<option value="">Default (heavy)</option>';
    machines.forEach((m) => {
      const o = document.createElement('option');
      o.value = m.name;
      o.textContent = m.name;
      cm.appendChild(o);
    });
    if (prevM) cm.value = prevM;
    const models = await fetch('/api/models').then((r) => r.json());
    md.innerHTML = '<option value="">Default</option>';
    models.forEach((m) => {
      const o = document.createElement('option');
      o.value = m.name;
      o.textContent = m.name;
      md.appendChild(o);
    });
  } catch (_) {}
}

async function fillBatchReduceSelectors() {
  try {
    const machines = await fetch('/api/machines').then((r) => r.json());
    const models = await fetch('/api/models').then((r) => r.json());
    const ms = $('batch-reduce-machine');
    const md = $('batch-reduce-model');
    if (!ms || !md) return;
    ms.innerHTML = '<option value="">—</option>';
    md.innerHTML = '<option value="">—</option>';
    (machines || []).forEach((m) => {
      const o = document.createElement('option');
      o.value = m.name;
      o.textContent = m.name;
      ms.appendChild(o);
    });
    (models || []).forEach((m) => {
      const o = document.createElement('option');
      o.value = m.name;
      o.textContent = m.name;
      md.appendChild(o);
    });
  } catch (_) {}
}

function fillBatchModelSelects() {
  fetch('/api/models').then((r) => r.json()).then((models) => {
    document.querySelectorAll('.batch-model').forEach((sel) => {
      const prev = sel.value;
      sel.innerHTML = '<option value="">—</option>';
      models.forEach((m) => {
        const o = document.createElement('option');
        o.value = m.name;
        o.textContent = m.name;
        sel.appendChild(o);
      });
      if (prev) sel.value = prev;
    });
  }).catch(() => {});
}

async function refreshBatchesList() {
  if (!jobsOn) return;
  try {
    const list = await fetch('/api/batches?limit=30').then((r) => r.json());
    const ul = $('batches-list');
    ul.innerHTML = '';
    list.forEach((b) => {
      const li = document.createElement('li');
      const done = (b.counts && b.counts.done) || 0;
      const failed = (b.counts && b.counts.failed) || 0;
      li.textContent = `${b.kind} · ${done}/${b.total} done` +
        (failed ? `, ${failed} failed` : '') +
        ` · ${new Date(b.created_at).toLocaleString()}`;
      li.onclick = () => openBatchView(b.id);
      ul.appendChild(li);
    });
  } catch (_) {}
}

function progressPct(counts) {
  const total = (counts.done || 0) + (counts.failed || 0) +
    (counts.cancelled || 0) + (counts.waiting || 0) + (counts.running || 0) + (counts.retrying || 0);
  if (!total) return 0;
  return Math.round(((counts.done || 0) / total) * 100);
}

function renderBatchView(b) {
  $('batch-detail').classList.remove('hidden');
  $('batch-view-id').textContent = b.id.slice(0, 8) + '…';
  let meta = `${b.kind} · ${b.pending} pending · ${b.total} items`;
  if (b.reduce) {
    meta += ` · reduce ${b.reduce.status || 'pending'}`;
  }
  $('batch-view-meta').textContent = meta;
  const overall = $('batch-overall-bar').firstElementChild;
  overall.style.width = `${progressPct(b.counts || {})}%`;
  const tb = $('batch-target-bars');
  tb.innerHTML = '';
  if (b.split && b.split.targets) {
    b.split.targets.forEach((t) => {
      const wrap = document.createElement('div');
      wrap.className = 'target-bar';
      const done = t.done || 0;
      const total = t.total || 0;
      wrap.textContent = `${t.machine}: ${done}/${total} done`;
      const bar = document.createElement('div');
      bar.className = 'progress';
      const fill = document.createElement('div');
      const denom = total || 1;
      fill.style.width = `${Math.round((done / denom) * 100)}%`;
      bar.appendChild(fill);
      wrap.appendChild(bar);
      tb.appendChild(wrap);
    });
  }
  const jl = $('batch-jobs-list');
  jl.innerHTML = '';
  (b.jobs || []).forEach((j) => {
    const li = document.createElement('li');
    li.textContent = `${j.status} · ${j.machine || '?'} · ${j.prompt_preview || j.id}`;
    li.onclick = () => {
      selectedJobId = j.id;
      showTab('chat');
      openJob(j.id);
    };
    jl.appendChild(li);
  });
  const active = (b.pending || 0) > 0;
  $('batch-cancel').style.display = active ? 'inline-block' : 'none';
}

async function openBatchView(id) {
  selectedBatchId = id;
  try {
    const b = await fetch(`/api/batches/${id}?limit=200`).then((r) => r.json());
    renderBatchView(b);
  } catch (_) {}
}

async function refreshTemplatesList() {
  try {
    const list = await fetch('/api/templates').then((r) => r.json());
    const ul = $('templates-list');
    ul.innerHTML = '';
    (list || []).forEach((t) => {
      const li = document.createElement('li');
      li.textContent = `${t.name} (${t.bytes} B, ${(t.vars || []).join(', ') || 'no blanks'})`;
      const use = document.createElement('button');
      use.type = 'button';
      use.textContent = 'Use';
      use.onclick = () => openTemplateUse(t);
      const edit = document.createElement('button');
      edit.type = 'button';
      edit.textContent = 'Edit';
      edit.onclick = () => openTemplateEdit(t.name);
      li.appendChild(use);
      li.appendChild(edit);
      ul.appendChild(li);
    });
  } catch (_) {}
}

async function openTemplateEdit(name) {
  selectedTemplateName = name || null;
  $('template-editor').classList.remove('hidden');
  $('template-use').classList.add('hidden');
  $('template-delete').classList.toggle('hidden', !name);
  $('template-name').disabled = !!name;
  $('template-editor-title').textContent = name ? `Edit ${name}` : 'New template';
  $('template-editor-state').classList.add('hidden');
  if (name) {
    const t = await fetch(`/api/templates/${encodeURIComponent(name)}`).then((r) => r.json());
    $('template-name').value = t.name;
    $('template-body').value = t.body || '';
    $('template-defaults').value = t.defaults ? JSON.stringify(t.defaults) : '';
  } else {
    $('template-name').value = '';
    $('template-body').value = '';
    $('template-defaults').value = '';
  }
}

async function fillTemplateUseSelectors() {
  const profiles = await fetch('/api/profiles').then((r) => r.json()).catch(() => []);
  const ps = $('template-use-profile');
  ps.innerHTML = '<option value="">—</option>';
  (profiles || []).forEach((p) => {
    const o = document.createElement('option');
    o.value = p.name;
    o.textContent = p.name;
    ps.appendChild(o);
  });
  const machines = await fetch('/api/machines').then((r) => r.json());
  const models = await fetch('/api/models').then((r) => r.json());
  const ms = $('template-use-machine');
  const md = $('template-use-model');
  ms.innerHTML = '<option value="">—</option>';
  md.innerHTML = '<option value="">—</option>';
  (machines || []).forEach((m) => {
    const o = document.createElement('option');
    o.value = m.name;
    o.textContent = m.name;
    ms.appendChild(o);
  });
  (models || []).forEach((m) => {
    const o = document.createElement('option');
    o.value = m.name;
    o.textContent = m.name;
    md.appendChild(o);
  });
}

async function openTemplateUse(t) {
  selectedTemplateName = t.name;
  $('template-editor').classList.add('hidden');
  $('template-use').classList.remove('hidden');
  $('template-use-name').textContent = t.name;
  templateUseVars = t.vars || [];
  const full = await fetch(`/api/templates/${encodeURIComponent(t.name)}`).then((r) => r.json());
  templateUseVars = full.vars || templateUseVars;
  const box = $('template-vars');
  box.innerHTML = '';
  templateUseVars.forEach((v) => {
    const lab = document.createElement('label');
    lab.textContent = v + ' ';
    const inp = document.createElement('input');
    inp.type = 'text';
    inp.id = `template-var-${v}`;
    lab.appendChild(inp);
    box.appendChild(lab);
  });
  $('template-batch-rows').classList.add('hidden');
  $('template-use-state').textContent = '';
  await fillTemplateUseSelectors();
}

function templateUseBody(batchMode) {
  const vars = {};
  templateUseVars.forEach((v) => {
    vars[v] = $(`template-var-${v}`).value;
  });
  const body = { template: selectedTemplateName, vars, lane: batchMode ? 'batch' : undefined };
  if ($('template-use-profile').value) body.profile = $('template-use-profile').value;
  if ($('template-use-fast').checked) body.fast = true;
  if ($('template-use-model').value) body.model = $('template-use-model').value;
  if ($('template-use-machine').value) body.machine = $('template-use-machine').value;
  return body;
}

function showTab(which) {
  for (const name of ['chat', 'status', 'stress', 'batches', 'templates', 'threads', 'schedules']) {
    $('panel-' + name).classList.toggle('hidden', name !== which);
    $('tab-' + name).classList.toggle('active', name === which);
  }
  if (statusTimer) {
    clearInterval(statusTimer);
    statusTimer = null;
  }
  if (which === 'chat' && jobsOn && currentChatThreadId && !activeJobId) {
    openChatThread(currentChatThreadId, { attach: true });
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
  if (batchTimer) {
    clearInterval(batchTimer);
    batchTimer = null;
  }
  if (which === 'batches') {
    refreshBatchesList();
    batchTimer = setInterval(() => {
      refreshBatchesList();
      if (selectedBatchId) openBatchView(selectedBatchId);
    }, 4000);
  }
  if (templateTimer) {
    clearInterval(templateTimer);
    templateTimer = null;
  }
  if (which === 'templates') {
    refreshTemplatesList();
    templateTimer = setInterval(refreshTemplatesList, 8000);
  }
  if (threadTimer) {
    clearInterval(threadTimer);
    threadTimer = null;
  }
  if (which === 'threads') {
    refreshThreadsList();
    fillThreadPinSelects();
    threadTimer = setInterval(() => {
      refreshThreadsList();
      if (selectedThreadId) openThread(selectedThreadId);
    }, 4000);
  }
  if (scheduleTimer) {
    clearInterval(scheduleTimer);
    scheduleTimer = null;
  }
  if (which === 'schedules') {
    refreshSchedulesList();
    fillScheduleSelectors();
    scheduleTimer = setInterval(refreshSchedulesList, 8000);
  }
}

async function refreshSchedulesList() {
  if (!jobsOn) return;
  try {
    const list = await fetch('/api/schedules').then((r) => r.json());
    const ul = $('schedules-list');
    ul.innerHTML = '';
    (list || []).forEach((s) => {
      const li = document.createElement('li');
      const when = s.rule?.every === 'week'
        ? `${s.rule.weekday} ${s.rule.at}`
        : `daily ${s.rule?.at}`;
      li.textContent = `${s.name} · ${when} · ${s.machine}/${s.model} · next: ${s.next_due || '—'} · last: ${s.last_state || '—'} · ${s.enabled ? 'on' : 'paused'}`;
      const edit = document.createElement('button');
      edit.type = 'button';
      edit.textContent = 'Edit';
      edit.onclick = () => openScheduleEditor(s);
      const run = document.createElement('button');
      run.type = 'button';
      run.textContent = 'Run now';
      run.onclick = async () => {
        await fetch(`/api/schedules/${s.id}/run`, { method: 'POST' });
        refreshSchedulesList();
      };
      const runs = document.createElement('button');
      runs.type = 'button';
      runs.textContent = 'Runs';
      runs.onclick = () => openScheduleRuns(s);
      const tog = document.createElement('button');
      tog.type = 'button';
      tog.textContent = s.enabled ? 'Pause' : 'Resume';
      tog.onclick = async () => {
        await fetch(`/api/schedules/${s.id}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled: !s.enabled }),
        });
        refreshSchedulesList();
      };
      li.appendChild(document.createTextNode(' '));
      li.appendChild(edit);
      li.appendChild(run);
      li.appendChild(runs);
      li.appendChild(tog);
      ul.appendChild(li);
    });
  } catch (_) {}
}

function fillScheduleSelectors() {
  ['schedule-machine', 'schedule-model'].forEach((id) => {
    const sel = $(id);
    const prev = sel.value;
    if (id.endsWith('machine')) {
      sel.innerHTML = '<option value="">—</option>';
      document.querySelectorAll('#machine option').forEach((o) => {
        if (!o.value) return;
        const c = o.cloneNode(true);
        sel.appendChild(c);
      });
    } else {
      sel.innerHTML = '<option value="">—</option>';
      document.querySelectorAll('#model option').forEach((o) => {
        if (!o.value) return;
        sel.appendChild(o.cloneNode(true));
      });
    }
    if (prev) sel.value = prev;
  });
}

function openScheduleEditor(s) {
  selectedScheduleId = s?.id || null;
  $('schedule-editor').classList.remove('hidden');
  $('schedule-runs').classList.add('hidden');
  $('schedule-editor-title').textContent = s ? 'Edit schedule' : 'New schedule';
  $('schedule-name').value = s?.name || '';
  $('schedule-every').value = s?.rule?.every || 'day';
  $('schedule-weekday').classList.toggle('hidden', $('schedule-every').value !== 'week');
  if (s?.rule?.weekday) $('schedule-weekday').value = s.rule.weekday;
  $('schedule-at').value = s?.rule?.at || '02:00';
  $('schedule-machine').value = s?.machine || '';
  $('schedule-model').value = s?.model || '';
  $('schedule-template').value = s?.template || '';
  $('schedule-prompt').value = s?.prompt || '';
  $('schedule-vars').value = s?.vars ? JSON.stringify(s.vars) : '';
  $('schedule-rows').value = s?.rows ? JSON.stringify(s.rows) : '';
  $('schedule-missed').value = s?.missed || 'skip';
  $('schedule-overlap').value = s?.overlap || 'skip';
  $('schedule-enabled').checked = s?.enabled !== false;
  $('schedule-delete').classList.toggle('hidden', !s);
  $('schedule-run-now').classList.toggle('hidden', !s);
}

async function openScheduleRuns(s) {
  $('schedule-runs').classList.remove('hidden');
  $('schedule-editor').classList.add('hidden');
  $('schedule-runs-name').textContent = s.name;
  const runs = await fetch(`/api/schedules/${s.id}/runs`).then((r) => r.json());
  const ul = $('schedule-runs-list');
  ul.innerHTML = '';
  (runs || []).forEach((r) => {
    const li = document.createElement('li');
    li.textContent = `${r.due} · ${r.status} · ${r.state || ''} ${r.error_code || ''}`.trim();
    ul.appendChild(li);
  });
}

function schedulePayloadFromForm() {
  const every = $('schedule-every').value;
  const rule = { every, at: $('schedule-at').value };
  if (every === 'week') rule.weekday = $('schedule-weekday').value;
  const body = {
    name: $('schedule-name').value.trim(),
    rule,
    machine: $('schedule-machine').value,
    model: $('schedule-model').value,
    missed: $('schedule-missed').value,
    overlap: $('schedule-overlap').value,
    enabled: $('schedule-enabled').checked,
  };
  const tpl = $('schedule-template').value.trim();
  const prompt = $('schedule-prompt').value.trim();
  if (tpl) body.template = tpl;
  else if (prompt) body.prompt = prompt;
  const vars = $('schedule-vars').value.trim();
  if (vars) body.vars = JSON.parse(vars);
  const rows = $('schedule-rows').value.trim();
  if (rows) body.rows = JSON.parse(rows);
  return body;
}

$('schedule-every').onchange = () => {
  $('schedule-weekday').classList.toggle('hidden', $('schedule-every').value !== 'week');
};

$('schedule-new').onclick = () => openScheduleEditor(null);
$('schedule-cancel').onclick = () => $('schedule-editor').classList.add('hidden');
$('schedule-runs-back').onclick = () => $('schedule-runs').classList.add('hidden');

$('schedule-save').onclick = async () => {
  $('schedule-editor-state').classList.add('hidden');
  let body;
  try {
    body = schedulePayloadFromForm();
  } catch (_) {
    $('schedule-editor-state').textContent = 'Invalid JSON in vars or rows';
    $('schedule-editor-state').classList.remove('hidden');
    return;
  }
  const url = selectedScheduleId ? `/api/schedules/${selectedScheduleId}` : '/api/schedules';
  const method = selectedScheduleId ? 'PATCH' : 'POST';
  const res = await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  const j = await res.json();
  if (!res.ok) {
    $('schedule-editor-state').textContent = j.error?.message || 'Save failed';
    $('schedule-editor-state').classList.remove('hidden');
    return;
  }
  $('schedule-editor').classList.add('hidden');
  refreshSchedulesList();
};

$('schedule-delete').onclick = async () => {
  if (!selectedScheduleId || !confirm('Delete this schedule and its run history?')) return;
  await fetch(`/api/schedules/${selectedScheduleId}`, { method: 'DELETE' });
  $('schedule-editor').classList.add('hidden');
  refreshSchedulesList();
};

$('schedule-run-now').onclick = async () => {
  if (!selectedScheduleId) return;
  await fetch(`/api/schedules/${selectedScheduleId}/run`, { method: 'POST' });
  refreshSchedulesList();
};

async function refreshThreadsList() {
  if (!jobsOn) return;
  try {
    const list = await fetch('/api/threads').then((r) => r.json());
    const ul = $('threads-list');
    ul.innerHTML = '';
    (list || []).forEach((t) => {
      const li = document.createElement('li');
      li.textContent = `${t.title || '(untitled)'} · ${t.model} · ${t.turns} turns`;
      if (t.id === selectedThreadId) li.classList.add('selected');
      li.onclick = () => openThread(t.id);
      ul.appendChild(li);
    });
  } catch (_) {}
}

function fillThreadPinSelects() {
  const ms = $('thread-pin-machine');
  const md = $('thread-pin-model');
  if (!ms || !md) return;
  const msel = $('machine');
  const mdl = $('model');
  ms.innerHTML = msel.innerHTML;
  md.innerHTML = mdl.innerHTML;
}

async function openThread(id) {
  selectedThreadId = id;
  currentChatThreadId = id;
  sessionStorage.setItem('pingpong.chatThread', id);
  $('threads-view').classList.remove('hidden');
  try {
    const t = await fetch(`/api/threads/${id}`).then((r) => r.json());
    $('thread-title').textContent = t.title || 'Thread';
    $('thread-meta').textContent = `${t.machine}, ${t.model}`;
    const box = $('thread-turns');
    box.innerHTML = '';
    (t.turns || []).forEach((turn) => {
      const div = document.createElement('div');
      div.className = 'thread-turn';
      div.innerHTML = `<strong>Turn ${turn.turn}</strong> (${turn.status}) · ${turn.machine}, ${turn.model}` +
        `<pre class="user">${esc(turn.prompt)}</pre><pre class="out">${esc(turn.output || '')}</pre>`;
      box.appendChild(div);
    });
    const active = (t.turns || []).find((x) => ['waiting', 'running', 'retrying'].includes(x.status));
    if (active) watchThreadJob(active.job_id);
    refreshThreadsList();
  } catch (_) {}
}

function esc(s) {
  return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;');
}

function watchThreadJob(jobId) {
  if (threadEventSource) threadEventSource.close();
  threadEventSource = new EventSource(`/api/jobs/${jobId}/events`);
  $('thread-state').textContent = 'Running…';
  threadEventSource.onmessage = (ev) => {
    try {
      const data = JSON.parse(ev.data);
      if (data.type === 'token') {
        const pres = $('thread-turns').querySelectorAll('.out');
        const last = pres[pres.length - 1];
        if (last) last.textContent += data.text || '';
      }
      if (data.type === 'done' || data.type === 'status') {
        if (data.state === 'done' || data.type === 'done') {
          $('thread-state').textContent = '';
          if (selectedThreadId) openThread(selectedThreadId);
        } else if (data.state) {
          $('thread-state').textContent = data.state;
        }
      }
    } catch (_) {}
  };
}

$('thread-new').onclick = async () => {
  const body = {};
  if ($('machine').value) body.machine = $('machine').value;
  if ($('model').value) body.model = $('model').value;
  if ($('profile').value) body.profile = $('profile').value;
  if ($('fast').checked) body.fast = true;
  const res = await fetch('/api/threads', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const j = await res.json();
  if (!res.ok) {
    $('thread-state').textContent = j.error?.message || 'Create failed';
    return;
  }
  await refreshThreadsList();
  refreshChatThreads();
  openThread(j.id);
};

$('thread-send').onclick = async () => {
  if (!selectedThreadId) return;
  const content = $('thread-message').value.trim();
  if (!content) return;
  const body = { content };
  if ($('thread-trim').checked) body.trim = 'oldest';
  $('thread-trim').checked = false;
  $('thread-state').textContent = 'Sending…';
  const res = await fetch(`/api/threads/${selectedThreadId}/messages`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const j = await res.json();
  if (!res.ok) {
    $('thread-state').textContent = j.error?.message || 'Send failed';
    return;
  }
  $('thread-message').value = '';
  if (j.dropped_turns) $('thread-state').textContent = `Dropped ${j.dropped_turns} old turns`;
  watchThreadJob(j.job_id);
  openThread(selectedThreadId);
};

$('thread-rename').onclick = async () => {
  if (!selectedThreadId) return;
  const title = prompt('New title');
  if (!title) return;
  await fetch(`/api/threads/${selectedThreadId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  });
  openThread(selectedThreadId);
};

$('thread-apply-pin').onclick = async () => {
  if (!selectedThreadId) return;
  const machine = $('thread-pin-machine').value;
  const model = $('thread-pin-model').value;
  if (!machine || !model) {
    $('thread-state').textContent = 'Pick machine and model';
    return;
  }
  const res = await fetch(`/api/threads/${selectedThreadId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ machine, model }),
  });
  const j = await res.json();
  if (!res.ok) {
    $('thread-state').textContent = j.error?.message || 'Pin failed';
    return;
  }
  openThread(selectedThreadId);
};

$('thread-delete').onclick = async () => {
  if (!selectedThreadId) return;
  if (!confirm('Delete this thread and all its turns?')) return;
  const gone = selectedThreadId;
  await fetch(`/api/threads/${selectedThreadId}`, { method: 'DELETE' });
  selectedThreadId = null;
  $('threads-view').classList.add('hidden');
  if (gone === currentChatThreadId) startNewChat();
  refreshThreadsList();
  refreshChatThreads();
};

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

function setBatchFormMode(mode) {
  const file = mode === 'file';
  $('batch-lines-mode').classList.toggle('hidden', file);
  $('batch-file-mode').classList.toggle('hidden', !file);
  $('batch-submit').classList.toggle('hidden', file);
  $('batch-file-start').classList.toggle('hidden', !file);
}

$('batch-mode-lines').onchange = () => setBatchFormMode('lines');
$('batch-mode-file').onchange = () => setBatchFormMode('file');

function fileBatchForm(extra) {
  const fd = new FormData();
  const f = $('batch-file').files[0];
  if (!f) throw new Error('Choose a file');
  fd.append('file', f);
  const inst = $('batch-file-instruction').value.trim();
  if (!inst.includes('{{chunk}}')) throw new Error('Instruction needs {{chunk}}');
  fd.append('instruction', inst);
  const cc = Number($('batch-chunk-chars').value);
  if (cc >= 500 && cc <= 20000) fd.append('chunk_chars', String(cc));
  const red = $('batch-reduce-instruction').value.trim();
  const rm = $('batch-reduce-machine').value;
  const rmod = $('batch-reduce-model').value;
  if (red || rm || rmod) {
    if (!red.includes('{{results}}') || !rm || !rmod) {
      throw new Error('Reduce needs instruction with {{results}}, machine and model');
    }
    fd.append('reduce', JSON.stringify({ instruction: red, machine: rm, model: rmod }));
  }
  const assign = $('batch-assign').value;
  if (assign) {
    const targets = [];
    document.querySelectorAll('.batch-target').forEach((row) => {
      const machine = row.dataset.machine;
      const model = row.querySelector('.batch-model').value;
      const weight = Number(row.querySelector('.batch-weight').value) || 1;
      if (model) targets.push({ machine, model, weight });
    });
    if (targets.length) fd.append('split', JSON.stringify({ assign, targets }));
  }
  if (extra) Object.entries(extra).forEach(([k, v]) => fd.append(k, v));
  return fd;
}

$('batch-file-preview').onclick = async () => {
  $('batch-file-preview-state').textContent = 'Previewing…';
  try {
    const fd = fileBatchForm({ dry_run: '1' });
    const res = await fetch('/api/batches/from-file', { method: 'POST', body: fd });
    const j = await res.json();
    if (!res.ok) throw new Error(j.error?.message || res.statusText);
    $('batch-file-preview-state').textContent = `${j.chunks} chunks, ${j.chars} chars`;
  } catch (e) {
    $('batch-file-preview-state').textContent = e.message;
  }
};

$('batch-file-start').onclick = async () => {
  $('batch-submit-state').textContent = 'Uploading…';
  try {
    const fd = fileBatchForm();
    const res = await fetch('/api/batches/from-file', { method: 'POST', body: fd });
    const j = await res.json();
    if (!res.ok) throw new Error(j.error?.message || res.statusText);
    $('batch-submit-state').textContent = `Created ${j.id.slice(0, 8)}… (${j.chunks} chunks)`;
    $('batch-file').value = '';
    refreshBatchesList();
    openBatchView(j.id);
  } catch (e) {
    $('batch-submit-state').textContent = e.message;
  }
};

$('batch-submit').onclick = async () => {
  const lines = $('batch-prompts').value.split('\n').map((s) => s.trim()).filter(Boolean);
  if (!lines.length) {
    $('batch-submit-state').textContent = 'Add at least one line';
    return;
  }
  const items = lines.map((prompt) => {
    const it = { prompt, lane: 'batch' };
    const bcm = $('batch-check-mode').value;
    if (bcm) it.check = { mode: bcm };
    return it;
  });
  const body = { items };
  const assign = $('batch-assign').value;
  if (assign) {
    const targets = [];
    document.querySelectorAll('.batch-target').forEach((row) => {
      const machine = row.dataset.machine;
      const model = row.querySelector('.batch-model').value;
      const weight = Number(row.querySelector('.batch-weight').value) || 1;
      if (model) targets.push({ machine, model, weight });
    });
    body.split = { assign, targets };
  }
  $('batch-submit-state').textContent = 'Submitting…';
  try {
    const res = await fetch('/api/batches', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const j = await res.json();
    if (!res.ok) throw new Error(j.error?.message || res.statusText);
    $('batch-submit-state').textContent = `Created ${j.id.slice(0, 8)}…`;
    $('batch-prompts').value = '';
    refreshBatchesList();
    openBatchView(j.id);
  } catch (e) {
    $('batch-submit-state').textContent = e.message;
  }
};

function batchExportURL(format) {
  if (!selectedBatchId) return '';
  const q = new URLSearchParams({ format });
  if ($('batch-export-thinking').checked) q.set('include_thinking', '1');
  if ($('batch-export-partial').checked) q.set('partial', '1');
  return `/api/batches/${selectedBatchId}/export?${q}`;
}

function startBatchDownload(format) {
  const url = batchExportURL(format);
  if (!url) return;
  const a = document.createElement('a');
  a.href = url;
  a.rel = 'noopener';
  document.body.appendChild(a);
  a.click();
  a.remove();
}

$('batch-export-csv').onclick = () => startBatchDownload('csv');
$('batch-export-md').onclick = () => startBatchDownload('md');

$('batch-cancel').onclick = async () => {
  if (!selectedBatchId) return;
  if (!window.confirm('Cancel every active job in this batch?')) return;
  await fetch(`/api/batches/${selectedBatchId}/cancel`, { method: 'POST' });
  openBatchView(selectedBatchId);
  refreshBatchesList();
};

$('template-new').onclick = () => openTemplateEdit(null);

$('template-cancel-edit').onclick = () => {
  $('template-editor').classList.add('hidden');
};

$('template-save').onclick = async () => {
  const name = $('template-name').value.trim();
  const body = $('template-body').value;
  let defaults = {};
  const defRaw = $('template-defaults').value.trim();
  if (defRaw) {
    try {
      defaults = JSON.parse(defRaw);
    } catch (_) {
      $('template-editor-state').textContent = 'Defaults must be valid JSON';
      $('template-editor-state').classList.remove('hidden');
      return;
    }
  }
  const payload = { body, defaults };
  const url = selectedTemplateName
    ? `/api/templates/${encodeURIComponent(selectedTemplateName)}`
    : '/api/templates';
  const method = selectedTemplateName ? 'PUT' : 'POST';
  if (!selectedTemplateName) payload.name = name;
  const res = await fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  const j = await res.json();
  if (!res.ok) {
    $('template-editor-state').textContent = j.error?.message || 'Save failed';
    $('template-editor-state').classList.remove('hidden');
    return;
  }
  $('template-editor').classList.add('hidden');
  refreshTemplatesList();
};

$('template-delete').onclick = async () => {
  if (!selectedTemplateName) return;
  if (!window.confirm(`Delete template ${selectedTemplateName}? Past jobs are unchanged.`)) return;
  await fetch(`/api/templates/${encodeURIComponent(selectedTemplateName)}`, { method: 'DELETE' });
  $('template-editor').classList.add('hidden');
  refreshTemplatesList();
};

$('template-use-back').onclick = () => {
  $('template-use').classList.add('hidden');
};

$('template-run-one').onclick = async () => {
  $('template-batch-rows').classList.add('hidden');
  $('template-use-state').textContent = 'Submitting…';
  const res = await fetch('/api/jobs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(templateUseBody(false)),
  });
  const j = await res.json();
  if (!res.ok) {
    $('template-use-state').textContent = j.error?.message || 'Failed';
    return;
  }
  const warn = (j.warnings || []).join('; ');
  $('template-use-state').textContent = `Job ${j.id.slice(0, 8)}…${warn ? ' — ' + warn : ''}`;
  refreshJobs();
};

$('template-run-batch').onclick = async () => {
  $('template-batch-rows').classList.remove('hidden');
  const lines = $('template-batch-rows').value.split('\n').map((s) => s.trim()).filter(Boolean);
  if (!lines.length) {
    $('template-use-state').textContent = 'Add one JSON object per line, then click again';
    return;
  }
  let rows;
  try {
    rows = lines.map((l) => JSON.parse(l));
  } catch (_) {
    $('template-use-state').textContent = 'Each line must be valid JSON';
    return;
  }
  const body = templateUseBody(true);
  body.rows = rows;
  $('template-use-state').textContent = 'Submitting batch…';
  const res = await fetch('/api/batches', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const j = await res.json();
  if (!res.ok) {
    $('template-use-state').textContent = j.error?.message || 'Failed';
    return;
  }
  const warn = (j.warnings || []).join('; ');
  $('template-use-state').textContent = `Batch ${j.id.slice(0, 8)}…${warn ? ' — ' + warn : ''}`;
  showTab('batches');
  openBatchView(j.id);
};

function syncChatOut() {
  const batch = jobsOn && $('batch-lane').checked;
  $('out').classList.toggle('hidden', jobsOn && !batch);
}

function startNewChat() {
  currentChatThreadId = null;
  sessionStorage.removeItem('pingpong.chatThread');
  streamTarget = null;
  $('chat-log').textContent = '';
  $('chat-thread-meta').textContent = 'New chat. The next message starts it.';
  if ($('chat-thread')) $('chat-thread').value = '';
  $('out').textContent = '';
  $('provenance').textContent = '';
}

async function refreshChatThreads() {
  if (!jobsOn || !$('chat-thread')) return;
  try {
    const list = await fetch('/api/threads').then((r) => r.json());
    const sel = $('chat-thread');
    const cur = currentChatThreadId || '';
    sel.innerHTML = '';
    const neu = document.createElement('option');
    neu.value = '';
    neu.textContent = 'New chat';
    sel.appendChild(neu);
    (list || []).forEach((t) => {
      const o = document.createElement('option');
      o.value = t.id;
      o.textContent = t.title || 'Untitled';
      sel.appendChild(o);
    });
    sel.value = cur;
    if (sel.value !== cur) sel.value = '';
  } catch (_) {}
}

function appendChatTurn(box, turn) {
  const div = document.createElement('div');
  div.className = 'chat-turn';
  const h = document.createElement('div');
  h.textContent = `Turn ${turn.turn} · ${turn.status} · ${turn.machine || ''}`;
  const user = document.createElement('pre');
  user.className = 'chat-user';
  user.textContent = turn.prompt || '';
  const out = document.createElement('pre');
  out.className = 'chat-out';
  out.textContent = turn.output || '';
  div.append(h, user, out);
  box.appendChild(div);
}

async function openChatThread(id, { attach = true } = {}) {
  const res = await fetch(`/api/threads/${id}`);
  if (!res.ok) {
    startNewChat();
    await refreshChatThreads();
    return;
  }
  const t = await res.json();
  currentChatThreadId = id;
  sessionStorage.setItem('pingpong.chatThread', id);
  selectedThreadId = id;
  $('chat-thread-meta').textContent = `Continuing on ${t.machine}, ${t.model}`;
  const box = $('chat-log');
  box.textContent = '';
  (t.turns || []).forEach((turn) => appendChatTurn(box, turn));
  box.scrollTop = box.scrollHeight;
  await refreshChatThreads();
  const active = (t.turns || []).find((x) => ['waiting', 'running', 'retrying'].includes(x.status));
  if (attach && active && !activeJobId) {
    const outs = box.querySelectorAll('.chat-out');
    streamTarget = outs.length ? outs[outs.length - 1] : null;
    watchJobEvents(active.job_id, { addToHistory: false, reloadChat: true });
  }
}

async function sendChatThread(text) {
  $('err').classList.add('hidden');
  $('warn').classList.add('hidden');
  $('state').textContent = 'Sending…';
  $('provenance').textContent = '';
  t0 = Date.now();
  clearInterval(elapsedTimer);
  elapsedTimer = setInterval(setElapsed, 500);
  try {
    let id = currentChatThreadId;
    if (!id) {
      const createBody = {};
      if ($('profile').value) createBody.profile = $('profile').value;
      if ($('fast').checked && !$('machine').value && !$('model').value) createBody.fast = true;
      if ($('model').value) createBody.model = $('model').value;
      if ($('machine').value) createBody.machine = $('machine').value;
      const res = await fetch('/api/threads', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(createBody),
      });
      const j = await res.json();
      if (!res.ok) throw new Error(j.error?.message || 'Could not start chat');
      id = j.id;
      currentChatThreadId = id;
      sessionStorage.setItem('pingpong.chatThread', id);
    }
    const body = { content: text };
    if ($('chat-trim').checked) {
      body.trim = 'oldest';
      $('chat-trim').checked = false;
    }
    const cm = $('check-mode');
    if (cm && cm.value) {
      body.check = { mode: cm.value };
      if ($('check-machine').value) body.check.machine = $('check-machine').value;
      if ($('check-model').value) body.check.model = $('check-model').value;
    }
    const res = await fetch(`/api/threads/${id}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const j = await res.json();
    if (!res.ok) throw new Error(j.error?.message || 'Send failed');
    $('prompt').value = '';
    if (j.dropped_turns) {
      $('warn').textContent = `Dropped ${j.dropped_turns} old turns to fit.`;
      $('warn').classList.remove('hidden');
    }
    await openChatThread(id, { attach: false });
    const outs = $('chat-log').querySelectorAll('.chat-out');
    streamTarget = outs.length ? outs[outs.length - 1] : null;
    await watchJobEvents(j.job_id, { addToHistory: false, reloadChat: true });
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
    streamTarget = null;
  }
}

function fmtPct(n) {
  return n == null ? '—' : `${Math.round(n)}%`;
}

function fmtBps(n) {
  if (n == null) return '—';
  if (n < 1024) return `${Math.round(n)} B/s`;
  if (n < 1048576) return `${(n / 1024).toFixed(1)} KB/s`;
  return `${(n / 1048576).toFixed(1)} MB/s`;
}

function fmtTemp(s) {
  if (s.temp_c != null) return `${Math.round(s.temp_c)}°C`;
  if (s.thermal_level != null) return `level ${s.thermal_level}`;
  return '—';
}

function fmtRAM(s) {
  if (s.ram_percent == null) return '—';
  const pct = `${Math.round(s.ram_percent)}%`;
  if (s.ram_used_bytes == null || s.ram_total_bytes == null) return pct;
  const gb = (n) => (n / 1073741824).toFixed(1);
  return `${pct} · ${gb(s.ram_used_bytes)}/${gb(s.ram_total_bytes)} GB`;
}

function addHostRow(dl, name, value) {
  const dt = document.createElement('dt');
  dt.textContent = name;
  const dd = document.createElement('dd');
  dd.textContent = value;
  dl.append(dt, dd);
}

async function refreshHost() {
  const box = $('host-cards');
  if (!box) return;
  try {
    const data = await fetch('/api/host').then((r) => r.json());
    box.textContent = '';
    (data.machines || []).forEach((m) => {
      const card = document.createElement('div');
      card.className = 'host-card';
      const h = document.createElement('h2');
      h.textContent = m.name;
      card.appendChild(h);
      const s = m.sample || {};
      if (!s.ok) {
        const p = document.createElement('p');
        p.textContent = s.error === 'not_configured' ? 'Not set up' : 'No report';
        card.appendChild(p);
      } else {
        const dl = document.createElement('dl');
        addHostRow(dl, 'CPU', fmtPct(s.cpu_percent));
        addHostRow(dl, 'RAM', fmtRAM(s));
        addHostRow(dl, 'Temp', fmtTemp(s));
        addHostRow(dl, 'Down', fmtBps(s.net_rx_bps));
        addHostRow(dl, 'Up', fmtBps(s.net_tx_bps));
        addHostRow(dl, 'Load', s.load_1 == null ? '—' : Number(s.load_1).toFixed(2));
        card.appendChild(dl);
      }
      box.appendChild(card);
    });
  } catch (_) {
    box.textContent = 'No report';
  }
}

$('chat-new').onclick = () => startNewChat();
$('chat-thread').onchange = () => {
  const id = $('chat-thread').value;
  if (!id) startNewChat();
  else openChatThread(id, { attach: true });
};
$('batch-lane').onchange = () => syncChatOut();
refreshHost();
setInterval(refreshHost, 5000);

$('tab-chat').onclick = () => showTab('chat');
$('tab-status').onclick = () => showTab('status');
$('tab-stress').onclick = () => showTab('stress');
$('tab-batches').onclick = () => showTab('batches');
$('tab-templates').onclick = () => showTab('templates');
$('tab-threads').onclick = () => showTab('threads');
$('tab-schedules').onclick = () => showTab('schedules');