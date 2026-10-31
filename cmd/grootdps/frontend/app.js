// SPDX-License-Identifier: EUPL-1.2
// Polls the meter inside the app (no network) and draws the rows.

const $ = (id) => document.getElementById(id);
let mode = 'dps';

const short = (n) =>
  n >= 1e9 ? (n / 1e9).toFixed(2) + 'B' :
  n >= 1e6 ? (n / 1e6).toFixed(2) + 'M' :
  n >= 1e4 ? (n / 1e3).toFixed(1) + 'k' : Math.round(n).toString();

const clock = (s) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`;

function row(r, top) {
  const li = document.createElement('li');
  li.className = 'row' + (r.self ? ' you' : '') + (!r.party && !r.self ? ' stranger' : '');
  const fill = document.createElement('span');
  fill.className = 'fill c-' + (r.class || '');
  fill.style.width = (top > 0 ? (r.amount / top) * 100 : 0) + '%';
  const name = document.createElement('span');
  name.className = 'name';
  name.textContent = r.name + (r.deaths ? ' ' : '');
  if (r.deaths) {
    const d = document.createElement('span');
    d.className = 'dead';
    d.textContent = '☠' + (r.deaths > 1 ? r.deaths : '');
    name.append(d);
  }
  const num = document.createElement('span');
  num.className = 'num';
  num.textContent = `${short(r.perSec)}  ${short(r.amount)}`;
  const pct = document.createElement('span');
  pct.className = 'pct';
  pct.textContent = r.share ? r.share.toFixed(1) + '%' : '';
  li.append(fill, name, num, pct);
  return li;
}

function draw(s) {
  const f = s.fight;
  const rows = $('rows');
  if (!f) {
    $('title').textContent = 'Waiting for a fight…';
    $('meta').textContent = '';
    rows.replaceChildren();
  } else {
    const title = $('title');
    title.replaceChildren();
    if (f.boss) {
      const b = document.createElement('span');
      b.className = 'boss';
      b.textContent = 'Boss ';
      title.append(b);
    }
    title.append(f.killed ? 'killed' : (f.boss ? '' : 'Fight'));
    $('meta').textContent = `${clock(f.seconds)} · ${short(f.total)}`;
    const list = (mode === 'dps' ? f.rows : f.heals) || [];
    const top = list.reduce((m, r) => Math.max(m, r.amount), 0);
    rows.replaceChildren(...list.map((r) => row(r, top)));
  }
  $('status').textContent = s.you ? 'Listening. Nothing is sent anywhere.' :
    'Listening. Use a potion or skill and you’ll show up as you.';
}

async function tick() {
  try {
    const res = await fetch('/api/meter');
    draw(await res.json());
  } catch (e) {
    $('status').textContent = 'Meter not answering: ' + e.message;
  }
}

$('mode').addEventListener('click', () => {
  mode = mode === 'dps' ? 'hps' : 'dps';
  $('mode').textContent = mode.toUpperCase();
  tick();
});
$('reset').addEventListener('click', async () => {
  await fetch('/api/reset', { method: 'POST' });
  tick();
});
$('close').addEventListener('click', () => window.wails?.Window?.Close());

setInterval(tick, 500);
tick();
