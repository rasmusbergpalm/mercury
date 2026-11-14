"use strict";

// ---- basics --------------------------------------------------------------------
const $ = (id) => document.getElementById(id);
const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
// a little "i" that opens a note on how something works (see the info
// popover below); html is the note
const info = (html) => `<button type="button" class="info-btn" aria-label="How this works" title="How this works" data-tip="${esc(html)}">i</button>`;
const fmt = (x, d = 2) => (Math.abs(x) >= 1000 ? Math.round(x).toLocaleString("en") : x.toFixed(d));
const pct = (x) => `${Math.round(100 * x)}%`;
// compact numbers for axes and plaques: 950, 9.5k, 1.2M
const money = (v) => (v < 0 ? "−£" : "£") + short(Math.abs(v));
function short(x) {
  const a = Math.abs(x);
  if (a >= 1e9) return (x / 1e9).toFixed(a >= 1e10 ? 0 : 1) + "bn";
  if (a >= 1e6) return (x / 1e6).toFixed(a >= 1e7 ? 0 : 1) + "M";
  if (a >= 1e4) return (x / 1e3).toFixed(0) + "k";
  if (a >= 1e3) return (x / 1e3).toFixed(1) + "k";
  if (a >= 100 || a === 0) return x.toFixed(0);
  if (a >= 1) return x.toFixed(a >= 10 ? 1 : 2);
  return x.toPrecision(2);
}
const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);

let gid = null;          // current game
let world = null;        // its static description
let state = null;        // its latest state
let detail = null;       // selected tile's details
let selected = null;     // selected tile id
let selectedArmy = null; // selected army id
let moving = false;      // next map tap orders the selected army to march there
let tab = "tile";
let viewNation = 0;      // the nation shown in the Nation and Trade tabs
let pollTimer = null;
let lastEventTick = -1;  // newest event seen, for auto-pause
let hist = {};           // history cache: "nations/0" -> {at, data, loading}
const chartPick = {};    // which chart each panel shows

// seats (player tokens) are kept per game in this browser
const seatKey = (g) => `mercury:seat:${g}`;
function getSeat(g) {
  try { return JSON.parse(localStorage.getItem(seatKey(g))) || null; } catch { return null; }
}
function setSeat(g, seat) {
  try { seat ? localStorage.setItem(seatKey(g), JSON.stringify(seat)) : localStorage.removeItem(seatKey(g)); } catch { /* storage blocked */ }
}
function pref(key, fallback) {
  try { const v = localStorage.getItem(`mercury:${key}`); return v === null ? fallback : JSON.parse(v); } catch { return fallback; }
}
function setPref(key, v) { try { localStorage.setItem(`mercury:${key}`, JSON.stringify(v)); } catch { /* storage blocked */ } }

async function api(method, path, body) {
  const headers = {};
  const seat = gid && getSeat(gid);
  if (seat) headers["X-Player-Token"] = seat.token;
  const r = await fetch(path, { method, headers, body });
  if (!r.ok) throw new Error((await r.text()).trim() || r.statusText);
  return r.status === 204 ? null : r.json();
}

function say(where, msg, isError = true) {
  const el = $(where);
  el.textContent = msg || "";
  el.classList.toggle("ok", !isError);
  if (msg) setTimeout(() => { if (el.textContent === msg) el.textContent = ""; }, 6000);
}
const showError = (where, e) => say(where, e ? e.message || String(e) : "");

// ---- theme ---------------------------------------------------------------------
// Colours are validated for each mode (contrast against the parchment and
// colour-blind separation); dark mode uses its own steps, not an inversion.
const NATION_DARK = { "#b8322e": "#d0473f", "#2c5aa0": "#4f7fc8", "#c08a1e": "#b9861f" };
const SERIES = { light: ["#7a4a8c", "#0f8a68", "#b0602a"], dark: ["#a77bbb", "#17a07a", "#cf7a3c"] };
let dark = false;
const readTheme = () => { dark = getComputedStyle(document.documentElement).getPropertyValue("--scheme").trim() === "dark"; };
const cssVar = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const UNOWNED = "#9a8f7c";
const nationColor = (id) => (id < 0 ? UNOWNED : dark ? NATION_DARK[world.nations[id].color] || world.nations[id].color : world.nations[id].color);
const series = (i) => SERIES[dark ? "dark" : "light"][i];
function applyTheme() {
  const t = pref("theme", null);
  if (t) document.documentElement.dataset.theme = t; else delete document.documentElement.dataset.theme;
  readTheme();
  terrainCache = null;
}
document.addEventListener("click", (ev) => {
  if (!ev.target.closest(".theme")) return;
  setPref("theme", dark ? "light" : "dark");
  applyTheme();
  if (state) render(true);
});
window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => { applyTheme(); if (state) render(true); });

// ---- routing -------------------------------------------------------------------
function route() {
  const m = location.hash.match(/^#\/game\/([0-9a-f]+)$/);
  if (m) openGame(m[1]).catch((e) => { location.hash = "#/"; showError("lobby-err", e); });
  else openLobby();
}
window.addEventListener("hashchange", route);

// ---- lobby ---------------------------------------------------------------------
async function openLobby() {
  stopPolling();
  gid = null;
  $("dialog").hidden = true; prompted = false; resumeAfter = false;
  $("game").hidden = true;
  $("lobby").hidden = false;
  document.title = "Mercury";
  try { await refreshLobby(); } catch (e) { showError("lobby-err", e); }
}

// the game this browser created that is still being prepared: the lobby
// polls until it opens, then goes to it
let awaiting = null, lobbyTimer = null;
async function refreshLobby() {
  clearTimeout(lobbyTimer);
  const [games, saves] = await Promise.all([api("GET", "/api/games"), api("GET", "/api/saves")]);
  if (awaiting) {
    const g = games.find((x) => x.id === awaiting);
    if (g && g.preparing === undefined) { const id = awaiting; awaiting = null; location.hash = `#/game/${id}`; return; }
    if (!g || g.error) awaiting = null;
  }
  if (!$("lobby").hidden && games.some((g) => g.preparing !== undefined && !g.error)) lobbyTimer = setTimeout(() => refreshLobby().catch((e) => showError("lobby-err", e)), 1000);
  $("games").innerHTML = games.length ? games.map(gameCard).join("") : `<p class="hint">No games yet. Found one below.</p>`;
  $("saves").innerHTML = saves.length
    ? `<div class="table-scroll"><table><tr><th>slot</th><th>game</th><th>day</th><th>saved</th><th></th></tr>${saves.map((s) =>
      `<tr><td>${esc(s.slot)}</td><td>${esc(s.name)}</td><td>${s.tick}</td><td>${new Date(s.saved_at).toLocaleString()}</td>
       <td><button data-load="${esc(s.slot)}">Load</button></td></tr>`).join("")}</table></div>`
    : `<p class="hint">No saves yet. Save a game from its top bar.</p>`;
}

const paceName = (d) => ({ 0: "real time", 7: "weekly turns", 30: "monthly turns", 90: "seasonal turns", 365: "yearly turns" }[d] || `${d}-day turns`);

const mapLabel = (g) => (g.map === "classic" ? "classic map" : `${g.map} map, ${g.tiles} tiles`);
function gameCard(g) {
  if (g.preparing !== undefined) {
    const status = g.error ? `<span class="bad">Couldn't be prepared: ${esc(g.error)}</span>`
      : `<span class="sub">Preparing the world: markets finding their prices, people moving to the work… ${Math.round(100 * g.preparing)}%</span><div class="progress"><span style="width:${(100 * g.preparing).toFixed(1)}%"></span></div>`;
    return `<div class="game-card"><div class="title"><b>${esc(g.name)}</b><span class="sub">${mapLabel(g)} · ${g.nations.length} nations</span></div>${status}</div>`;
  }
  const seat = getSeat(g.id);
  const pills = g.nations.map((n) => {
    let action;
    if (seat && seat.nation === n.id && n.controller === "human") action = `<button class="primary" data-open="${g.id}">Continue</button>`;
    else if (n.eliminated) action = `<span class="sub">fallen</span>`;
    else if (n.controller === "ai") action = `<button data-join="${g.id}" data-nation="${n.id}">Play</button>`;
    else action = `<span class="sub">player</span>`;
    return `<span class="nation-pill"><span class="chip" style="background:${n.color}"></span>${esc(n.name)} · ${n.tiles} tiles ${action}</span>`;
  }).join("");
  return `<div class="game-card">
    <div class="title"><b>${esc(g.name)}</b><span class="sub">${esc(g.date)} · ${mapLabel(g)} · ${paceName(g.turn_days)}${g.running ? " · running" : ""}</span></div>
    <div class="nations-row">${pills}<button data-open="${g.id}">Watch</button></div></div>`;
}

$("lobby").addEventListener("click", async (ev) => {
  const b = ev.target.closest("button");
  if (!b) return;
  try {
    if (b.dataset.open) location.hash = `#/game/${b.dataset.open}`;
    if (b.dataset.join) {
      gid = b.dataset.join;
      const seat = await api("POST", `/api/games/${gid}/join?nation=${b.dataset.nation}`);
      setSeat(gid, seat);
      location.hash = `#/game/${gid}`;
    }
    if (b.dataset.load) {
      const g = await api("POST", `/api/saves/${encodeURIComponent(b.dataset.load)}/load`);
      location.hash = `#/game/${g.id}`;
    }
  } catch (e) { gid = null; showError("lobby-err", e); }
});

$("new-game").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  try {
    const q = new URLSearchParams({ name: $("new-name").value.trim(), cost: $("new-cost").value, turn: $("new-turn").value, map: $("new-map").value });
    if ($("new-nations").value) q.set("nations", $("new-nations").value);
    if ($("new-divided").checked) q.set("divided", "1");
    const g = await api("POST", `/api/games?${q}`);
    if (g.preparing === undefined) { location.hash = `#/game/${g.id}`; return; }
    awaiting = g.id; // a generated map settles first: open it when it's ready
    await refreshLobby();
  } catch (e) { showError("lobby-err", e); }
});
$("new-map").addEventListener("change", () => { $("new-nations").disabled = $("new-map").value === "classic"; });
$("new-nations").disabled = $("new-map").value === "classic";

$("import").addEventListener("change", async (ev) => {
  const file = ev.target.files[0];
  if (!file) return;
  try {
    const g = await api("POST", "/api/games/import", await file.text());
    location.hash = `#/game/${g.id}`;
  } catch (e) { showError("lobby-err", e); }
  ev.target.value = "";
});

// ---- game ----------------------------------------------------------------------
async function openGame(id) {
  stopPolling();
  clearTimeout(lobbyTimer);
  gid = id;
  selected = null; detail = null; selectedArmy = null; moving = false; hist = {}; seaRoutes.clear();
  world = await api("GET", `/api/games/${id}/world`);
  state = await api("GET", `/api/games/${id}/state`);
  const summary = (await api("GET", "/api/games")).find((g) => g.id === id);
  $("game-name").textContent = summary ? summary.name : "Mercury";
  document.title = `${summary ? summary.name : "Game"} · Mercury`;
  $("lobby").hidden = true;
  $("game").hidden = false;
  $("export").href = `/api/games/${id}/export`;
  const opts = world.goods.map((g, i) => `<option value="${i}">${esc(g.name)}${g.tier ? "" : " (input)"}</option>`).join("");
  $("good").innerHTML = opts;
  $("good").value = String(world.goods.findIndex((g) => g.name === "bread"));
  $("flow-good").innerHTML = `<option value="all">all goods (value)</option>${opts}`;
  $("speed").innerHTML = [1, 2, 3, 7, 15, 30].map((s) => `<option value="${s}">${s} day${s > 1 ? "s" : ""}/s</option>`).join("");
  if (state.you < 0 && getSeat(id)) setSeat(id, null); // the server no longer knows our seat
  viewNation = state.you >= 0 ? state.you : 0;
  lastEventTick = state.events.length ? state.events[state.events.length - 1].tick : -1;
  setTab(state.you >= 0 ? "nation" : "world");
  terrainCache = null;
  initialView();
  render(true);
  startPolling();
}

// a big world's shipments come only when something shows them: the map's
// trade overlay or the Trade tab
const wantFlows = () => $("show-flows").checked || tab === "trade";
const fetchState = () => api("GET", `/api/games/${gid}/state${wantFlows() ? "?flows=1" : ""}`);
function startPolling() {
  stopPolling();
  const tickOnce = async () => {
    try {
      state = await fetchState();
      if (selected !== null && tab === "tile") detail = await api("GET", `/api/games/${gid}/regions/${selected}`);
      autoPause();
      render();
    } catch (e) { showError("game-err", e); }
    if (gid) pollTimer = setTimeout(tickOnce, state && state.running ? 700 : 2500);
  };
  pollTimer = setTimeout(tickOnce, 700);
}
function stopPolling() { clearTimeout(pollTimer); pollTimer = null; }

// pause when something important happens to you
const ALARMS = ["war", "battle", "lost", "siege", "eliminated", "peace"];
function autoPause() {
  const fresh = state.events.filter((e) => e.tick > lastEventTick);
  if (state.events.length) lastEventTick = Math.max(lastEventTick, state.events[state.events.length - 1].tick);
  if (!fresh.length || !state.running || !canClock() || !pref("autopause", true)) return;
  const alarm = fresh.find((e) => ALARMS.includes(e.kind) && (e.nation < 0 || e.nation === state.you));
  if (alarm) {
    command("/clock?running=false");
    say("game-err", `Paused: ${alarm.text}`, false);
  }
}

const humans = () => state.nations.some((n) => n.controller === "human");
const canClock = () => !state.turn && (!humans() || state.you >= 0);
const me = () => (state.you >= 0 ? state.nations[state.you] : null);
const owner = (id) => state.regions[id].nation;
const armyById = (id) => state.armies.find((a) => a.id === id);
const nationName = (id) => (id >= 0 ? world.nations[id].name : "unowned");
const chip = (id) => `<span class="chip" style="background:${nationColor(id)}"></span>`;
// the game's calendar has 365-day years (29 February is skipped), so any
// common year maps its days to dates
const dateOf = (tick) => {
  const y = world.start_year + Math.floor(tick / 365);
  const dt = new Date(Date.UTC(2001, 0, 1 + (tick % 365)));
  return `${dt.getUTCDate()} ${dt.toLocaleString("en", { month: "short", timeZone: "UTC" })} ${y}`;
};
const yearOf = (tick) => world.start_year + tick / 365;

async function command(path, after) {
  try {
    state = await api("POST", `/api/games/${gid}${path}`);
    if (selected !== null) detail = await api("GET", `/api/games/${gid}/regions/${selected}`);
    if (after) after();
    render(true);
    return true;
  } catch (e) { showError("game-err", e); return false; }
}

// ---- history -------------------------------------------------------------------
// History is sampled monthly; fetch a series when first shown and again once
// a month has passed. The panel re-renders when it arrives.
const HISTORY_EVERY = 30;
function getHist(path) {
  let h = hist[path];
  if (!h || (!h.loading && state.tick - h.at >= HISTORY_EVERY)) {
    h = hist[path] = { ...(h || {}), loading: true, at: state.tick };
    api("GET", `/api/games/${gid}/history/${path}`)
      .then((d) => { h.data = d; h.loading = false; renderPanel(true); })
      .catch(() => { h.loading = false; });
  }
  return h.data || null;
}

// ---- top bar -------------------------------------------------------------------
function renderBar() {
  $("date").textContent = state.date;
  $("clock").hidden = !!state.turn;
  $("turn").hidden = !state.turn;
  if (state.turn) {
    const players = state.nations.filter((n) => n.controller === "human").map((n) => n.id);
    const waiting = players.filter((p) => !state.turn.ready.includes(p)).map(nationName);
    $("end-turn").textContent = `End turn (${state.turn.days} days)`;
    $("end-turn").disabled = state.you < 0 || state.turn.ready.includes(state.you);
    $("turn-status").textContent = waiting.length ? `waiting for ${waiting.join(", ")}` : "";
  } else {
    $("play").textContent = state.running ? "Pause" : "Play";
    $("speed").value = String(state.speed);
    for (const id of ["play", "speed", "step10"]) $(id).disabled = !canClock();
  }
  $("save").disabled = !(!humans() || state.you >= 0);
  $("leave").hidden = state.you < 0;
  const n = me();
  $("you").hidden = !n;
  if (!n) return;
  const wars = n.at_war.map((w, i) => (w ? world.nations[i].name : null)).filter(Boolean);
  $("you").innerHTML = `<span>${chip(n.id)} <b>${esc(world.nations[n.id].name)}</b>${n.eliminated ? " (fallen)" : ""}</span>
    <span>tiles <b>${n.tiles}</b></span><span>people <b>${short(n.population)}</b></span>
    <span>treasury <b>£${short(n.treasury)}</b></span><span>income <b>£${short(n.income)}</b>/day</span>
    <span>soldiers <b>${short(n.soldiers)}</b> / ${short(n.mobilize_cap)}</span>
    ${wars.length ? `<span class="war">⚔ at war with <b>${wars.map(esc).join(", ")}</b></span>` : ""}`;
}
$("to-lobby").onclick = () => { location.hash = "#/"; };
$("play").onclick = () => command(`/clock?running=${!state.running}`);
$("speed").onchange = () => command(`/clock?speed=${$("speed").value}`);
$("step10").onclick = () => command("/step?n=10");
$("end-turn").onclick = () => command("/end-turn");
$("leave").onclick = () => command("/leave", () => { setSeat(gid, null); setTab("world"); });
$("save").onclick = async () => {
  const suggestion = ($("game-name").textContent || "game").replace(/[^A-Za-z0-9_-]+/g, "-").slice(0, 40);
  const slot = prompt("Save as (letters, digits, - or _):", suggestion);
  if (!slot) return;
  try {
    const info = await api("POST", `/api/games/${gid}/save?slot=${encodeURIComponent(slot)}`);
    say("game-err", `Saved “${info.slot}” on ${state.date}.`, false);
  } catch (e) { showError("game-err", e); }
};

// ---- tabs ----------------------------------------------------------------------
const TABS = ["tile", "nation", "state", "research", "trade", "armies", "world"];
function setTab(t) {
  tab = t;
  for (const b of document.querySelectorAll(".tabs button")) b.classList.toggle("active", b.dataset.tab === t);
  for (const name of TABS) $(`tab-${name}`).hidden = name !== t;
}
document.querySelector(".tabs").addEventListener("click", async (ev) => {
  const b = ev.target.closest("button");
  if (!b) return;
  setTab(b.dataset.tab);
  if (tab === "tile" && selected !== null) {
    try { detail = await api("GET", `/api/games/${gid}/regions/${selected}`); } catch (e) { showError("game-err", e); }
  }
  if (tab === "trade" && !state.flows.length) { // a big world's shipments: fetch them now
    try { state = await fetchState(); } catch (e) { showError("game-err", e); }
  }
  render(true);
});

// is the user typing, dragging or reading a chart in this element? then don't redraw it
let chartHover = null;
const busy = (el) => (el.contains(document.activeElement) && /INPUT|SELECT/.test(document.activeElement.tagName)) || (chartHover && el.contains(chartHover));

function render(force) {
  if (!state || !world) return;
  readTheme();
  renderBar();
  researchPrompt();
  draw();
  const m = $("mode");
  const a = selectedArmy !== null && armyById(selectedArmy);
  m.hidden = !(moving && a);
  if (moving && a) m.innerHTML = `<span>Choose where army ${a.id} marches</span><button id="cancel-move">Cancel</button>`;
  renderPanel(force);
}
function renderPanel(force) {
  const el = $(`tab-${tab}`);
  if (!force && busy(el)) return;
  if (chartHover && el.contains(chartHover)) return; // never pull a chart from under the pointer
  const panels = { tile: tilePanel, nation: nationPanel, state: statePanel, research: researchPanel, trade: tradePanel, armies: armiesPanel, world: () => worldPanel() + newsPanel() };
  const scroll = el.scrollTop;
  charts.clear();
  el.innerHTML = panels[tab]();
  el.scrollTop = scroll;
}

// ---- charts --------------------------------------------------------------------
// Hand-drawn SVG line charts: one y-axis, recessive grid, 2px lines, a legend
// for two or more series plus direct labels at the line ends, and a crosshair
// with a tooltip on hover or touch.
const charts = new Map();
let chartSeq = 0;
const CW = 400, PAD = { l: 42, t: 10, b: 20 };

function niceStep(span, n) {
  const raw = span / n, p = Math.pow(10, Math.floor(Math.log10(raw))), f = raw / p;
  return (f < 1.5 ? 1 : f < 3 ? 2 : f < 7 ? 5 : 10) * p;
}

// lineChart(ticks, [{name, color, values}], {f: formatter, h, share})
function lineChart(ticks, list, o = {}) {
  const f = o.f || short;
  if (!ticks || ticks.length < 2) return `<div class="chart"><div class="empty">The ledger is written monthly; the first entries appear after two months.</div></div>`;
  const h = o.h || 160, labels = list.length >= 2 && list.length <= 4, padR = labels ? 78 : 12;
  const x0 = PAD.l, x1 = CW - padR, y0 = h - PAD.b, y1 = PAD.t;
  const all = list.flatMap((s) => s.values).filter((v) => v !== null && isFinite(v));
  let lo = Math.min(0, ...all), hi = Math.max(...all, 1e-9);
  if (o.share) { lo = 0; hi = Math.max(hi, 0.001); }
  const step = niceStep(hi - lo || 1, 3);
  lo = Math.floor(lo / step) * step; hi = Math.ceil(hi / step) * step;
  const tx = (i) => x0 + (x1 - x0) * (ticks[i] - ticks[0]) / Math.max(1, ticks[ticks.length - 1] - ticks[0]);
  const ty = (v) => y0 - (y0 - y1) * (v - lo) / (hi - lo || 1);
  let svg = "";
  for (let v = lo; v <= hi + step / 2; v += step) {
    svg += `<line class="grid" x1="${x0}" x2="${x1}" y1="${ty(v)}" y2="${ty(v)}"/>`;
    svg += `<text class="axis" x="${x0 - 5}" y="${ty(v) + 4}" text-anchor="end">${o.share ? pct(v) : f(v)}</text>`;
  }
  const ya = yearOf(ticks[0]), yb = yearOf(ticks[ticks.length - 1]);
  const ys = Math.max(1, niceStep(yb - ya || 1, 4));
  for (let y = Math.ceil(ya / ys) * ys; y <= yb; y += ys) {
    const x = x0 + (x1 - x0) * (y - ya) / (yb - ya || 1);
    svg += `<text class="axis" x="${x}" y="${h - 5}" text-anchor="middle">${Math.round(y)}</text>`;
  }
  const ends = [];
  for (const s of list) {
    let d = "", pen = false;
    s.values.forEach((v, i) => {
      if (v === null || !isFinite(v)) { pen = false; return; }
      d += `${pen ? "L" : "M"}${tx(i).toFixed(1)},${ty(v).toFixed(1)}`;
      pen = true;
    });
    svg += `<path d="${d}" fill="none" stroke="${s.color}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>`;
    const last = s.values[s.values.length - 1];
    if (labels && last !== null) ends.push({ y: ty(last), name: s.name, color: s.color });
  }
  // direct labels, nudged apart so they don't collide
  ends.sort((a, b) => a.y - b.y);
  for (let i = 1; i < ends.length; i++) ends[i].y = Math.max(ends[i].y, ends[i - 1].y + 13);
  for (const e of ends) {
    svg += `<rect x="${x1 + 4}" y="${e.y - 1}" width="8" height="2" fill="${e.color}"/><text class="lbl" x="${x1 + 15}" y="${e.y + 4}">${esc(e.name)}</text>`;
  }
  const id = ++chartSeq;
  charts.set(String(id), { ticks, list, tx, ty, x0, x1, h, f: o.share ? pct : f });
  svg += `<line class="cross" x1="0" x2="0" y1="${y1}" y2="${y0}" visibility="hidden"/>`;
  svg += list.map((s) => `<circle r="4" fill="${s.color}" stroke="var(--surface)" stroke-width="2" visibility="hidden"/>`).join("");
  const legend = list.length >= 2
    ? `<div class="legend-inline">${list.map((s) => `<span><span class="swatch" style="background:${s.color};height:3px;vertical-align:4px"></span>${esc(s.name)}</span>`).join("")}</div>` : "";
  return `${legend}<div class="chart" data-chart="${id}"><svg viewBox="0 0 ${CW} ${h}" role="img" aria-label="${esc(list.map((s) => s.name).join(", "))} over time">${svg}</svg></div>`;
}

function chartAt(ev) {
  const el = ev.target.closest && ev.target.closest(".chart[data-chart]");
  const c = el && charts.get(el.dataset.chart);
  if (!c) return;
  const svg = el.querySelector("svg"), rect = svg.getBoundingClientRect();
  const sx = (ev.clientX - rect.left) * CW / rect.width;
  let best = 0;
  c.ticks.forEach((t, i) => { if (Math.abs(c.tx(i) - sx) < Math.abs(c.tx(best) - sx)) best = i; });
  const x = c.tx(best);
  const cross = svg.querySelector(".cross");
  cross.setAttribute("x1", x); cross.setAttribute("x2", x); cross.setAttribute("visibility", "visible");
  svg.querySelectorAll("circle").forEach((dot, k) => {
    const v = c.list[k].values[best];
    if (v === null || !isFinite(v)) { dot.setAttribute("visibility", "hidden"); return; }
    dot.setAttribute("cx", x); dot.setAttribute("cy", c.ty(v)); dot.setAttribute("visibility", "visible");
  });
  let tip = el.querySelector(".chart-tip");
  if (!tip) { tip = document.createElement("div"); tip.className = "chart-tip"; el.appendChild(tip); }
  tip.innerHTML = `<b>${dateOf(c.ticks[best])}</b>` + c.list.map((s) => {
    const v = s.values[best];
    return `<br><span class="k" style="background:${s.color}"></span>${esc(s.name)}: ${v === null ? "–" : c.f(v)}`;
  }).join("");
  const px = x * rect.width / CW;
  tip.style.top = "0px";
  tip.style.left = px > rect.width / 2 ? "" : `${px + 12}px`;
  tip.style.right = px > rect.width / 2 ? `${rect.width - px + 12}px` : "";
}
function chartLeave(el) {
  const svg = el.querySelector("svg");
  if (svg) { svg.querySelector(".cross").setAttribute("visibility", "hidden"); svg.querySelectorAll("circle").forEach((d) => d.setAttribute("visibility", "hidden")); }
  const tip = el.querySelector(".chart-tip");
  if (tip) tip.remove();
}
const side = document.querySelector(".side");
side.addEventListener("pointermove", (ev) => {
  const el = ev.target.closest(".chart[data-chart]");
  if (chartHover && chartHover !== el) { chartLeave(chartHover); chartHover = null; }
  if (el) { chartHover = el; chartAt(ev); }
});
side.addEventListener("pointerleave", () => { if (chartHover) chartLeave(chartHover); chartHover = null; });
document.addEventListener("pointerdown", (ev) => { if (chartHover && !chartHover.contains(ev.target)) { chartLeave(chartHover); chartHover = null; } });

// a row of choices: which chart to show
function seg(key, options, fallback) {
  const cur = options.some(([k]) => k === chartPick[key]) ? chartPick[key] : fallback;
  return [cur, `<div class="seg">${options.map(([k, label]) => `<button data-pick="${key}" data-val="${esc(k)}" class="${k === cur ? "on" : ""}">${label}</button>`).join("")}</div>`];
}
// like seg, as a drop-down: for long lists (goods)
function pickSelect(key, options, fallback, label) {
  const cur = options.some(([k]) => k === chartPick[key]) ? chartPick[key] : fallback;
  return [cur, `<div class="row" style="margin:0 0 6px"><select data-pick-select="${key}" aria-label="${esc(label)}">${options.map(([k, l]) => `<option value="${esc(k)}"${k === cur ? " selected" : ""}>${esc(l)}</option>`).join("")}</select></div>`];
}
const stat = (v, k, d = "") => `<div class="stat"><div class="v">${v}</div><div class="k">${k}</div>${d ? `<div class="d">${d}</div>` : ""}</div>`;
// change over the last year of history, as a small coloured note
function trend(h, key, sub) {
  if (!h || !h.data) return "";
  let s = h.data[key];
  if (sub !== undefined) s = s && s[sub];
  if (!s || s.length < 13) return "";
  const a = s[s.length - 13], b = s[s.length - 1];
  if (!(Math.abs(a) > 1e-9)) return "";
  const r = b / a - 1;
  return `<span class="${r >= 0 ? "good" : "bad"}">${r >= 0 ? "▲" : "▼"} ${Math.abs(100 * r).toFixed(r > -0.1 && r < 0.1 ? 1 : 0)}%/yr</span>`;
}
function shield(color, px = 30) {
  return `<svg class="shield" viewBox="0 0 30 36" width="${px}" height="${px * 1.2}" aria-hidden="true"><path d="M2 2h26v14c0 10-7 15-13 18C9 31 2 26 2 16z" fill="${color}" stroke="${cssVar("--brass")}" stroke-width="2"/><path d="M15 6v24M6 14h18" stroke="rgba(255,255,255,.55)" stroke-width="2"/></svg>`;
}

// ---- map -----------------------------------------------------------------------
const canvas = $("map");
const ctx = canvas.getContext("2d");
const TERRAIN = {
  light: { plains: "#e3d59b", forest: "#b9c28c", hills: "#d6bd8c", mountains: "#c9bdab", tropical: "#bcd194" },
  dark: { plains: "#5c5434", forest: "#3e4a2f", hills: "#5b4b34", mountains: "#514b44", tropical: "#3d5234" },
};
const OCEAN = { light: "#9fc2c9", dark: "#22383d" };
const TIER_COLORS = () => [series(1), series(2), series(0)]; // basic, intermediate, luxury
// a single warm hue, light to dark, for magnitudes
const RAMP = ["#f6ecd0", "#ecd09a", "#d9a066", "#b5622f", "#7d2a17"];
const hexRGB = (c) => [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16));
const rgba = (c, a) => `rgba(${hexRGB(c).join(",")},${a})`;
function rampColor(t) {
  t = Math.max(0, Math.min(1, t));
  const x = t * (RAMP.length - 1), i = Math.min(Math.floor(x), RAMP.length - 2), f = x - i;
  const a = hexRGB(RAMP[i]), b = hexRGB(RAMP[i + 1]);
  return `rgb(${a.map((v, k) => Math.round(v + (b[k] - v) * f)).join(",")})`;
}

let size = 40;
const margin = 16;
const RING = 0.7; // how much of the surrounding ring of ocean tiles shows, in tiles
// The view: the map fits the pane at zoom 1 (fitSize); zoomed in, it pans.
// view.x and view.y are how far the map is scrolled, in CSS pixels.
let fitSize = 40;
const view = { zoom: 1, x: 0, y: 0 };
const MAX_SIZE = 64; // the largest hex, zoomed in
function center(r) {
  const w = Math.sqrt(3) * size;
  return [margin + RING * w + w / 2 + w * (r.col + 0.5 * (r.row & 1)) - view.x, margin + size + RING * 1.5 * size + 1.5 * size * r.row - view.y];
}
// the canvas's size in CSS pixels, and the whole map's at the current zoom
const canvasW = () => canvas.width / (window.devicePixelRatio || 1);
const canvasH = () => canvas.height / (window.devicePixelRatio || 1);
const mapW = () => 2 * margin + Math.sqrt(3) * size * (world.cols + 0.5 + 2 * RING);
const mapH = () => 2 * margin + size * (2 + 1.5 * (world.rows - 1) + 3 * RING);
// whether a point (a tile's centre) is near enough the canvas to draw
const onScreen = (x, y, pad = size * 1.2) => x > -pad && y > -pad && x < canvasW() + pad && y < canvasH() + pad;
function clampView() {
  view.x = Math.max(0, Math.min(view.x, mapW() - canvasW()));
  view.y = Math.max(0, Math.min(view.y, mapH() - canvasH()));
}
// zoom to z, keeping the map point under (sx, sy) where it is
function zoomTo(z, sx = canvasW() / 2, sy = canvasH() / 2) {
  z = Math.max(1, Math.min(z, MAX_SIZE / fitSize));
  const ux = (sx + view.x - margin) / size, uy = (sy + view.y - margin) / size;
  view.zoom = z; size = fitSize * z;
  view.x = margin + ux * size - sx; view.y = margin + uy * size - sy;
  clampView();
  requestDraw();
}
// centre the view on a tile
function lookAt(r) {
  const [x, y] = center(r);
  view.x += x - canvasW() / 2; view.y += y - canvasH() / 2;
  clampView();
}
let drawQueued = false;
function requestDraw() {
  if (drawQueued) return;
  drawQueued = true;
  requestAnimationFrame(() => { drawQueued = false; draw(); });
}
const corner = (x, y, s, i) => { const a = Math.PI / 180 * (60 * i - 30); return [x + s * Math.cos(a), y + s * Math.sin(a)]; };
function hexPath(x, y, s, c = ctx) {
  c.beginPath();
  for (let i = 0; i < 6; i++) { const [px, py] = corner(x, y, s, i); i ? c.lineTo(px, py) : c.moveTo(px, py); }
  c.closePath();
}
// the neighbour across each edge (edge i joins corners i and i+1): E, SE, SW, W, NW, NE
function across(r, i) {
  const odd = r.row & 1;
  const d = [[0, 1], [1, odd ? 1 : 0], [1, odd ? 0 : -1], [0, -1], [-1, odd ? 0 : -1], [-1, odd ? 1 : 0]][i];
  return grid[`${r.row + d[0]},${r.col + d[1]}`];
}
let grid = {};
function resize() {
  if (!world || $("game").hidden) return;
  grid = {};
  for (const r of world.regions) grid[`${r.row},${r.col}`] = r;
  const wrap = canvas.parentElement;
  const narrow = window.innerWidth <= 860;
  const availH = narrow ? Infinity : Math.max(320, window.innerHeight - wrap.getBoundingClientRect().top - 60);
  fitSize = Math.min((wrap.clientWidth - 2 * margin) / (Math.sqrt(3) * (world.cols + 0.5 + 2 * RING)), (availH - 2 * margin) / (2 + 1.5 * (world.rows - 1) + 3 * RING));
  size = fitSize;
  const w = mapW(), h = mapH();
  size = fitSize * view.zoom;
  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.round(w * dpr); canvas.height = Math.round(h * dpr);
  canvas.style.width = w + "px"; canvas.style.height = h + "px";
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  canvas.style.touchAction = zoomable() ? "none" : ""; // fingers pan and pinch a big map
  canvas.classList.toggle("pan", zoomable());
  $("zoom").hidden = !zoomable();
  clampView();
  terrainCache = null;
  draw();
}
window.addEventListener("resize", resize);
// a map too big to read whole can be zoomed (wheel, pinch, buttons) and panned (drag)
const zoomable = () => world && world.regions.length > 60;
// a new game's view: big maps open zoomed in on the player's capital (or the middle)
function initialView() {
  view.zoom = 1; view.x = 0; view.y = 0;
  resize();
  if (!zoomable()) return;
  zoomTo(Math.max(1, 36 / fitSize));
  const mine = state.you >= 0 && world.regions.find((r) => state.regions[r.id].nation === state.you);
  lookAt(mine || world.regions[Math.floor(world.regions.length / 2)]);
  terrainCache = null;
}

// deterministic scatter for terrain glyphs
function rng(seed) { let s = seed * 2654435761 % 4294967296 || 1; return () => ((s = (s * 1664525 + 1013904223) % 4294967296) / 4294967296); }

// The terrain layer (parchment, washes and glyphs) only changes with the size
// and theme, so it's drawn once onto an offscreen canvas.
let terrainCache = null;
function terrainLayer() {
  const key = `${size}|${dark}|${canvas.width}|${canvas.height}|${view.x}|${view.y}`;
  if (terrainCache && terrainCache.key === key) return terrainCache.canvas;
  const dpr = window.devicePixelRatio || 1, c = document.createElement("canvas");
  c.width = canvas.width; c.height = canvas.height;
  const g = c.getContext("2d");
  g.setTransform(dpr, 0, 0, dpr, 0, 0);
  const W = canvas.width / dpr, H = canvas.height / dpr;
  // paper with a faint vignette and a double frame
  g.fillStyle = cssVar("--paper"); g.fillRect(0, 0, W, H);
  const vg = g.createRadialGradient(W / 2, H / 2, Math.min(W, H) * 0.3, W / 2, H / 2, Math.max(W, H) * 0.75);
  vg.addColorStop(0, "rgba(0,0,0,0)"); vg.addColorStop(1, dark ? "rgba(0,0,0,0.35)" : "rgba(120,80,30,0.18)");
  g.fillStyle = vg; g.fillRect(0, 0, W, H);
  const pal = TERRAIN[dark ? "dark" : "light"], ink = dark ? "rgba(225,205,160,0.55)" : "rgba(80,55,25,0.6)";
  // the open sea inside the frame, with a ring of ocean tiles around the map
  g.save(); g.beginPath(); g.rect(8, 8, W - 16, H - 16); g.clip();
  g.fillStyle = OCEAN[dark ? "dark" : "light"]; g.fillRect(8, 8, W - 16, H - 16);
  const glyphs = size >= 12; // too small to read: plain colours
  for (let row = -2; row <= world.rows + 1; row++) {
    for (let col = -2; col <= world.cols + 1; col++) {
      if (grid[`${row},${col}`]) continue;
      const [x, y] = center({ row, col });
      if (!onScreen(x, y)) continue;
      if (glyphs) {
        g.save(); hexPath(x, y, size * 0.96, g); g.clip();
        terrainGlyphs(g, { id: 1000 + (row + 2) * 1000 + col, terrain: "ocean" }, x, y, ink);
        g.restore();
      }
      hexPath(x, y, size, g); g.lineWidth = 0.5; g.strokeStyle = cssVar("--hex-stroke"); g.stroke();
    }
  }
  for (const r of world.regions) {
    const [x, y] = center(r);
    if (!onScreen(x, y)) continue;
    hexPath(x, y, size, g);
    g.fillStyle = pal[r.terrain]; g.fill();
    if (glyphs) {
      g.save(); hexPath(x, y, size * 0.96, g); g.clip();
      terrainGlyphs(g, r, x, y, ink);
      g.restore();
    }
    hexPath(x, y, size, g); g.lineWidth = 0.75; g.strokeStyle = cssVar("--hex-stroke"); g.stroke();
  }
  g.restore();
  g.strokeStyle = cssVar("--rule"); g.lineWidth = 2; g.strokeRect(4, 4, W - 8, H - 8);
  g.lineWidth = 0.75; g.strokeRect(8, 8, W - 16, H - 16);
  terrainCache = { key, canvas: c };
  return c;
}

function terrainGlyphs(g, r, x, y, ink) {
  const rand = rng(r.id + 7), s = size;
  const spots = [];
  for (let k = 0; spots.length < 9 && k < 80; k++) {
    const a = rand() * 2 * Math.PI, d = (0.25 + 0.55 * Math.sqrt(rand())) * s;
    const px = x + d * Math.cos(a), py = y + d * Math.sin(a) * 0.9;
    if (Math.abs(py - (y + 0.62 * s)) < 0.14 * s && Math.abs(px - x) < 0.5 * s) continue; // keep the name clear
    if (spots.some(([qx, qy]) => Math.hypot(qx - px, qy - py) < 0.26 * s)) continue;
    spots.push([px, py]);
  }
  g.lineWidth = Math.max(0.8, s * 0.025); g.strokeStyle = ink; g.lineCap = "round"; g.lineJoin = "round";
  const u = s * 0.1;
  for (const [px, py] of spots) {
    g.beginPath();
    switch (r.terrain) {
      case "plains": // tufts of grass and wheat
        g.moveTo(px - u * 0.6, py); g.lineTo(px - u * 0.9, py - u * 0.9);
        g.moveTo(px, py); g.lineTo(px, py - u * 1.2);
        g.moveTo(px + u * 0.6, py); g.lineTo(px + u * 0.9, py - u * 0.9);
        g.stroke(); break;
      case "forest": { // a broadleaf: trunk and canopy
        g.moveTo(px, py + u * 0.9); g.lineTo(px, py); g.stroke();
        g.beginPath(); g.arc(px, py - u * 0.35, u * 0.75, 0, 2 * Math.PI);
        g.fillStyle = dark ? "#55683c" : "#7d9455"; g.fill(); g.stroke(); break;
      }
      case "hills": // rounded humps with a shading stroke
        g.arc(px, py + u * 0.4, u * 1.1, Math.PI * 1.1, Math.PI * 1.9); g.stroke();
        g.beginPath(); g.moveTo(px + u * 0.2, py - u * 0.2); g.lineTo(px + u * 0.6, py + u * 0.3); g.stroke(); break;
      case "mountains": { // a peak, shaded on one side, snow on top
        const hgt = u * (1.6 + rand() * 0.6), wd = u * 1.3;
        g.moveTo(px - wd, py + u * 0.6); g.lineTo(px, py + u * 0.6 - hgt); g.lineTo(px + wd, py + u * 0.6);
        g.fillStyle = dark ? "#6a645b" : "#b0a28c"; g.fill(); g.stroke();
        g.beginPath(); g.moveTo(px, py + u * 0.6 - hgt); g.lineTo(px + wd * 0.35, py + u * 0.6 - hgt * 0.65); g.lineTo(px - wd * 0.35, py + u * 0.6 - hgt * 0.65); g.closePath();
        g.fillStyle = dark ? "#cfc8bb" : "#fbf7ee"; g.fill(); break;
      }
      case "tropical": // a palm
        g.moveTo(px, py + u); g.quadraticCurveTo(px + u * 0.4, py, px + u * 0.2, py - u * 0.6); g.stroke();
        for (const dx of [-1, -0.4, 0.5, 1.1]) {
          g.beginPath(); g.moveTo(px + u * 0.2, py - u * 0.6);
          g.quadraticCurveTo(px + u * (0.2 + dx * 0.6), py - u * 1.3, px + u * (0.2 + dx), py - u * 0.4); g.stroke();
        }
        break;
      case "ocean": // waves
        g.strokeStyle = dark ? "rgba(150,200,210,0.7)" : "rgba(40,90,110,0.6)";
        g.moveTo(px - u, py); g.quadraticCurveTo(px - u * 0.5, py - u * 0.6, px, py); g.quadraticCurveTo(px + u * 0.5, py - u * 0.6, px + u, py);
        g.stroke(); g.strokeStyle = ink; break;
    }
  }
}

const CATEGORICAL = ["terrain", "nation", "infra"];
const isShare = () => ["basic", "luxury"].includes($("overlay").value);
function share(rs, tier) {
  const total = rs.tier_spend.reduce((a, b) => a + b, 0);
  return total > 0 ? rs.tier_spend[world.tiers.indexOf(tier)] / total : 0;
}
function overlayValue(id) {
  const rs = state.regions[id], g = +$("good").value;
  switch ($("overlay").value) {
    case "utility": return rs.utility;
    case "population": return rs.population;
    case "growth": return rs.growth;
    case "wage": return rs.wage;
    case "price": return rs.prices[g] / rs.wage;
    case "basic": return share(rs, "basic");
    case "luxury": return share(rs, "luxury");
  }
  return 0;
}
const overlayLabel = () => ($("overlay").value === "price" ? `${world.goods[+$("good").value].name} price ÷ wage` : $("overlay").selectedOptions[0].textContent);
const fmtOverlay = (v) => ($("overlay").value === "growth" ? `${(100 * v).toFixed(1)}%` : isShare() ? pct(v) : short(v));

// goods nobody can make yet (oil before anyone drills) have no market: leave
// them out of the goods pickers
function hideUnmadeGoods() {
  if (!state.made) return;
  for (const id of ["good", "flow-good"]) {
    for (const o of $(id).options) if (o.value !== "all") o.hidden = o.disabled = !state.made[+o.value];
  }
}

function draw() {
  if (!world || !state) return;
  hideUnmadeGoods();
  const dpr = window.devicePixelRatio || 1;
  ctx.save(); ctx.setTransform(1, 0, 0, 1, 0, 0);
  ctx.drawImage(terrainLayer(), 0, 0);
  ctx.restore();
  const kind = $("overlay").value;
  let sc = null;
  if (!CATEGORICAL.includes(kind)) {
    const f = kind === "price" ? Math.log : (x) => x;
    const vals = state.regions.map((r) => overlayValue(r.id)), xs = vals.map(f);
    let lo = Math.min(...xs), hi = Math.max(...xs);
    if (hi - lo < 1e-9) { lo -= 1; hi += 1; }
    sc = { t: (v) => (f(v) - lo) / (hi - lo), lo: vals[xs.indexOf(Math.min(...xs))], hi: vals[xs.indexOf(Math.max(...xs))] };
  }
  for (const r of world.regions) {
    const rs = state.regions[r.id], [x, y] = center(r);
    if (!onScreen(x, y)) continue;
    if (sc) {
      hexPath(x, y, size * 0.99); ctx.fillStyle = rampColor(sc.t(overlayValue(r.id))); ctx.globalAlpha = 0.88; ctx.fill(); ctx.globalAlpha = 1;
    } else if (kind === "infra") {
      hexPath(x, y, size); ctx.fillStyle = dark ? "rgba(0,0,0,0.35)" : "rgba(244,234,210,0.55)"; ctx.fill();
    }
    if (rs.occupation) hatch(x, y, nationColor(rs.occupation.nation), rs.occupation.progress);
  }
  drawBorders(kind !== "terrain");
  drawRoads(kind === "infra");
  const avg = state.regions.reduce((t, rs) => t + rs.population, 0) / state.regions.length;
  for (const r of world.regions) {
    const rs = state.regions[r.id], [x, y] = center(r);
    if (!onScreen(x, y)) continue;
    if (size >= 14) drawSettlement(r, x, y - size * 0.12, rs.population, avg);
    if (rs.port) drawAnchor(x - size * 0.55, y - size * 0.28);
    if (rs.project) progressRing(x + size * 0.52, y - size * 0.34, rs.project.delivered / rs.project.required);
  }
  if (size >= 30) drawNames();
  if (moving && selectedArmy !== null && armyById(selectedArmy)) shadeUnreachable(armyById(selectedArmy).nation);
  if ($("show-flows").checked) drawFlows();
  for (const r of world.regions) if (state.regions[r.id].battle > 0 && onScreen(...center(r))) drawBattle(...center(r));
  drawPaths();
  drawArmies();
  drawBoots();
  if (selected !== null) {
    const [x, y] = center(world.regions[selected]);
    hexPath(x, y, size * 0.93); ctx.lineWidth = 3; ctx.strokeStyle = cssVar("--select"); ctx.stroke();
    hexPath(x, y, size * 0.84); ctx.lineWidth = 1; ctx.stroke();
  }
  drawLegend(sc);
}

// tile names, small, along the bottom of each hex
function drawNames() {
  const px = Math.min(13, size * 0.24);
  ctx.font = `italic ${px}px ${cssVar("--serif") || "Georgia, serif"}`;
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.lineWidth = 3; ctx.lineJoin = "round";
  ctx.strokeStyle = dark ? "rgba(33,27,21,0.75)" : "rgba(251,245,228,0.8)";
  ctx.fillStyle = cssVar("--text");
  for (const r of world.regions) {
    const [x, y] = center(r);
    if (!onScreen(x, y)) continue;
    ctx.strokeText(r.name, x, y + size * 0.62);
    ctx.fillText(r.name, x, y + size * 0.62);
  }
}

// a label on a small paper plate, legible over any terrain
function plate(t, x, y, px, bold, color) {
  ctx.font = `${bold ? "600 " : "italic "}${px}px ${cssVar("--serif") || "Georgia, serif"}`;
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  const w = ctx.measureText(t).width + px * 0.7, h = px * 1.25;
  ctx.fillStyle = dark ? "rgba(33,27,21,0.85)" : "rgba(251,245,228,0.85)";
  roundRect(x - w / 2, y - h / 2, w, h, h * 0.25); ctx.fill();
  if (color) { ctx.lineWidth = 1.5; ctx.strokeStyle = color; ctx.stroke(); }
  ctx.fillStyle = cssVar("--text");
  ctx.fillText(t, x, y + 0.5);
}
function roundRect(x, y, w, h, r) {
  ctx.beginPath(); ctx.moveTo(x + r, y); ctx.arcTo(x + w, y, x + w, y + h, r); ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r); ctx.arcTo(x, y, x + w, y, r); ctx.closePath();
}

// occupation: diagonal hatching in the occupier's colour, denser as it nears 100%
function hatch(x, y, color, progress) {
  ctx.save(); hexPath(x, y, size * 0.97); ctx.clip();
  ctx.strokeStyle = rgba(color, 0.35 + 0.5 * progress); ctx.lineWidth = 2;
  const gap = size * (0.32 - 0.16 * progress);
  ctx.beginPath();
  for (let d = -2 * size; d < 2 * size; d += gap) { ctx.moveTo(x + d - size, y + size); ctx.lineTo(x + d + size, y - size); }
  ctx.stroke(); ctx.restore();
}

// a brass ring that fills as a construction project is supplied
function progressRing(x, y, f) {
  const r = size * 0.11;
  ctx.beginPath(); ctx.arc(x, y, r, 0, 2 * Math.PI); ctx.fillStyle = cssVar("--surface"); ctx.fill();
  ctx.lineWidth = 1; ctx.strokeStyle = cssVar("--rule"); ctx.stroke();
  ctx.beginPath(); ctx.arc(x, y, r, -Math.PI / 2, -Math.PI / 2 + 2 * Math.PI * Math.min(1, f));
  ctx.lineWidth = size * 0.06; ctx.strokeStyle = cssVar("--brass"); ctx.stroke();
  // a little hammer: the project's mark
  ctx.lineWidth = 1.2; ctx.strokeStyle = cssVar("--text");
  ctx.beginPath(); ctx.moveTo(x - r * 0.4, y + r * 0.45); ctx.lineTo(x + r * 0.3, y - r * 0.25); ctx.moveTo(x, y - r * 0.5); ctx.lineTo(x + r * 0.55, y + 0.05 * r); ctx.stroke();
}

// borders: a band of the owner's colour inside each frontier, and an inked line on it
function drawBorders(bands) {
  for (const r of world.regions) {
    const o = owner(r.id), [x, y] = center(r);
    if (!onScreen(x, y)) continue;
    for (let i = 0; i < 6; i++) {
      const nb = across(r, i), other = nb ? owner(nb.id) : -2;
      if (other === o) continue;
      const [ax, ay] = corner(x, y, size, i), [bx, by] = corner(x, y, size, i + 1);
      if (o >= 0 && bands) {
        const [cx, cy] = corner(x, y, size * 0.88, i), [dx, dy] = corner(x, y, size * 0.88, i + 1);
        ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(bx, by); ctx.lineTo(dx, dy); ctx.lineTo(cx, cy); ctx.closePath();
        ctx.fillStyle = rgba(nationColor(o), 0.9); ctx.fill();
      }
      if (nb && nb.id < r.id) continue; // ink each shared edge once
      if (!nb && o < 0) continue;
      ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(bx, by);
      ctx.setLineDash(nb ? [6, 3, 1.5, 3] : []); ctx.lineWidth = 1.5; ctx.strokeStyle = cssVar("--text"); ctx.stroke(); ctx.setLineDash([]);
    }
  }
}

// roads, paved roads and railways run between neighbouring tiles that both have them
function drawRoads(strong) {
  ctx.lineCap = "round";
  ctx.globalAlpha = strong ? 0.95 : 0.3; // faint unless the infrastructure view asks for them
  for (const l of world.links) {
    const lvl = Math.min(state.regions[l.a].infra, state.regions[l.b].infra);
    if (!lvl) continue;
    const [x1, y1] = center(world.regions[l.a]), [x2, y2] = center(world.regions[l.b]);
    if (!onScreen(x1, y1) && !onScreen(x2, y2)) continue;
    road(x1, y1, x2, y2, lvl, strong);
  }
  for (const r of world.regions) {
    const lvl = state.regions[r.id].infra;
    if (!lvl) continue;
    const [x, y] = center(r);
    ctx.beginPath(); ctx.arc(x, y, 1 + lvl, 0, 2 * Math.PI);
    ctx.fillStyle = lvl === 3 ? "#2b2420" : "#7a5630"; ctx.fill();
  }
  ctx.globalAlpha = 1;
}
function road(x1, y1, x2, y2, lvl, strong) {
  const k = strong ? 1.4 : 1;
  ctx.setLineDash([]);
  if (lvl === 1) {
    ctx.setLineDash([4, 4]); ctx.lineWidth = 1.3 * k; ctx.strokeStyle = dark ? "rgba(196,154,98,0.8)" : "rgba(138,98,52,0.75)";
    ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke(); ctx.setLineDash([]);
  } else if (lvl === 2) {
    ctx.lineWidth = 3 * k; ctx.strokeStyle = dark ? "#8a6a44" : "rgba(122,86,48,0.85)";
    ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke();
    ctx.lineWidth = 1.5 * k; ctx.strokeStyle = dark ? "#d8c08e" : "#e7d3a3"; ctx.stroke();
  } else {
    const len = Math.hypot(x2 - x1, y2 - y1), ux = (x2 - x1) / len, uy = (y2 - y1) / len, w = 3.2 * k;
    ctx.lineWidth = 1.2 * k; ctx.strokeStyle = dark ? "#e2d4b8" : "#2b2420";
    ctx.beginPath();
    for (let d = 3; d < len - 2; d += 5) { const px = x1 + ux * d, py = y1 + uy * d; ctx.moveTo(px - uy * w, py + ux * w); ctx.lineTo(px + uy * w, py - ux * w); }
    ctx.stroke();
    ctx.lineWidth = 1.6 * k;
    for (const s of [-1.6, 1.6]) {
      ctx.beginPath(); ctx.moveTo(x1 - uy * s * k, y1 + ux * s * k); ctx.lineTo(x2 - uy * s * k, y2 + ux * s * k); ctx.stroke();
    }
  }
}

// Settlements grow with population: a few farmsteads on a thin tile, a town
// with a church spire around the average, and a city of roofs and mill
// chimneys where people crowd in. Sizes are relative to the average tile.
function drawSettlement(r, x, y, pop, avg) {
  const rel = pop / Math.max(avg, 1e-9);
  const n = Math.max(1, Math.min(18, Math.round(3 * Math.pow(rel, 1.2))));
  const u = size * 0.08 * (n > 9 ? 0.85 : 1), ink = cssVar("--text");
  const wall = dark ? "#cdbb98" : "#f3e6c8", roof = dark ? "#b0533e" : "#9c3b26", slate = dark ? "#7d8790" : "#5d6670";
  const rand = rng(r.id * 31 + 5);
  // the houses, on a loose spiral out from the centre
  const spots = [];
  for (let k = 0; k < n; k++) {
    const a = k * 2.4 + rand() * 0.6, d = k === 0 ? 0 : u * (1.5 + 1.25 * Math.sqrt(k));
    spots.push([x + d * Math.cos(a), y + d * Math.sin(a) * 0.7]);
  }
  if (n >= 5) { // trodden ground under a town
    ctx.beginPath(); ctx.ellipse(x, y + u * 0.3, u * (2.2 + 1.3 * Math.sqrt(n)), u * (1.6 + 0.9 * Math.sqrt(n)), 0, 0, 2 * Math.PI);
    ctx.fillStyle = dark ? "rgba(0,0,0,0.25)" : "rgba(120,90,50,0.16)"; ctx.fill();
  }
  ctx.lineWidth = 1; ctx.strokeStyle = ink; ctx.lineJoin = "round";
  const chimneys = rel >= 2 ? Math.min(3, Math.floor(rel / 1.5)) : 0;
  for (let c = 0; c < chimneys; c++) { // mills: a tall chimney with smoke
    const [cx, cy] = spots[(c * 3 + 2) % n];
    const hx = cx + u * 1.1, h = u * (3 + rand());
    ctx.fillStyle = dark ? "#8a6a52" : "#7a4b35";
    ctx.fillRect(hx, cy - h, u * 0.55, h); ctx.strokeRect(hx, cy - h, u * 0.55, h);
    ctx.beginPath(); ctx.arc(hx + u * 0.5, cy - h - u * 0.5, u * 0.45, 0, 2 * Math.PI); ctx.arc(hx + u * 1.2, cy - h - u * 1.0, u * 0.65, 0, 2 * Math.PI);
    ctx.fillStyle = dark ? "rgba(200,190,170,0.45)" : "rgba(90,80,70,0.3)"; ctx.fill();
  }
  spots.sort((a, b) => a[1] - b[1]); // paint back to front
  spots.forEach(([hx, hy], k) => {
    const w = u * (1.3 + rand() * 0.5), h = u * (1 + rand() * 0.5);
    ctx.fillStyle = wall; ctx.fillRect(hx - w / 2, hy - h, w, h); ctx.strokeRect(hx - w / 2, hy - h, w, h);
    ctx.beginPath(); ctx.moveTo(hx - w / 2 - u * 0.15, hy - h); ctx.lineTo(hx, hy - h - u * 0.8); ctx.lineTo(hx + w / 2 + u * 0.15, hy - h); ctx.closePath();
    ctx.fillStyle = rel >= 2 && k % 2 ? slate : roof; ctx.fill(); ctx.stroke();
  });
  if (n >= 5) { // the church
    const [hx, hy] = [x + u * 0.2, y - u * 0.4];
    ctx.fillStyle = wall; ctx.fillRect(hx - u * 0.5, hy - u * 2.2, u, u * 2.2); ctx.strokeRect(hx - u * 0.5, hy - u * 2.2, u, u * 2.2);
    ctx.beginPath(); ctx.moveTo(hx - u * 0.6, hy - u * 2.2); ctx.lineTo(hx, hy - u * 4.2); ctx.lineTo(hx + u * 0.6, hy - u * 2.2); ctx.closePath();
    ctx.fillStyle = slate; ctx.fill(); ctx.stroke();
  }
}

function drawAnchor(x, y) {
  const u = size * 0.07;
  ctx.beginPath(); ctx.arc(x, y, u * 2.1, 0, 2 * Math.PI); ctx.fillStyle = dark ? "rgba(33,27,21,0.85)" : "rgba(251,245,228,0.9)"; ctx.fill();
  ctx.lineWidth = 1; ctx.strokeStyle = cssVar("--rule"); ctx.stroke();
  ctx.strokeStyle = dark ? "#9cc6d4" : "#1f4a6b"; ctx.lineWidth = Math.max(1.2, u * 0.35); ctx.lineCap = "round";
  ctx.beginPath(); ctx.arc(x, y - 1.2 * u, 0.35 * u, 0, 2 * Math.PI); ctx.stroke();
  ctx.beginPath(); ctx.moveTo(x, y - 0.85 * u); ctx.lineTo(x, y + 1.3 * u);
  ctx.moveTo(x - 0.8 * u, y - 0.4 * u); ctx.lineTo(x + 0.8 * u, y - 0.4 * u); ctx.stroke();
  ctx.beginPath(); ctx.arc(x, y + 0.2 * u, 1.1 * u, 0.15 * Math.PI, 0.85 * Math.PI); ctx.stroke();
}

// armies: a shield in the nation's colour with the number of men beneath
const armySpot = (x, y, i, n) => [x + (i - (n - 1) / 2) * size * 0.45, y + size * 0.05];
function drawArmies() {
  const byTile = {};
  for (const a of state.armies) (byTile[a.region] = byTile[a.region] || []).push(a);
  for (const [tile, armies] of Object.entries(byTile)) {
    const [x, y] = center(world.regions[+tile]);
    armies.forEach((a, i) => {
      const [bx, by] = armySpot(x, y, i, armies.length), w = size * 0.17, h = size * 0.21;
      ctx.beginPath();
      ctx.moveTo(bx - w, by - h); ctx.lineTo(bx + w, by - h); ctx.lineTo(bx + w, by);
      ctx.quadraticCurveTo(bx + w, by + h * 0.8, bx, by + h * 1.15); ctx.quadraticCurveTo(bx - w, by + h * 0.8, bx - w, by); ctx.closePath();
      ctx.shadowColor = "rgba(0,0,0,0.35)"; ctx.shadowBlur = 3; ctx.shadowOffsetY = 1;
      ctx.fillStyle = nationColor(a.nation); ctx.fill();
      ctx.shadowColor = "transparent";
      const sel = a.id === selectedArmy;
      ctx.lineWidth = sel ? 3 : 1.6; ctx.strokeStyle = sel ? cssVar("--select") : "#e7c776";
      if (a.retreating) ctx.setLineDash([3, 2]); // falling back
      ctx.stroke(); ctx.setLineDash([]);
      // crossed sabres
      ctx.strokeStyle = "rgba(255,255,255,0.9)"; ctx.lineWidth = Math.max(1, size * 0.025);
      ctx.beginPath(); ctx.moveTo(bx - w * 0.55, by - h * 0.6); ctx.lineTo(bx + w * 0.55, by + h * 0.55);
      ctx.moveTo(bx + w * 0.55, by - h * 0.6); ctx.lineTo(bx - w * 0.55, by + h * 0.55); ctx.stroke();
      // strength, and a health bar: strength against the army at full size and supply
      const health = Math.max(0, Math.min(1, a.strength / Math.max(a.full_strength, 1e-9)));
      const bw = w * 2.3, bh = Math.max(3.5, size * 0.065), bxl = bx - bw / 2, byt = by + h * 1.3;
      ctx.fillStyle = "rgba(20,14,8,0.75)"; roundRect(bxl - 1, byt - 1, bw + 2, bh + 2, 2); ctx.fill();
      ctx.fillStyle = health > 0.6 ? (dark ? "#8fbf6a" : "#4f8a2e") : health > 0.3 ? "#d9a520" : "#c8402e";
      ctx.fillRect(bxl, byt, bw * health, bh);
      plate(short(a.strength), bx, byt + bh + Math.max(9, size * 0.15) * 0.75, Math.max(9, size * 0.15), true);
    });
  }
}

// While choosing where to march: grey out the tiles the army can't enter.
// An army keeps to its own tiles and those next to them (its supply lines),
// and enters foreign land only at war.
function canEnter(nation, r) {
  const o = state.regions[r.id].nation;
  if (o >= 0 && o !== nation && !state.nations[nation].at_war[o]) return false;
  if (o === nation) return true;
  for (let i = 0; i < 6; i++) { const nb = across(r, i); if (nb && state.regions[nb.id].nation === nation) return true; }
  return false;
}
function shadeUnreachable(nation) {
  ctx.fillStyle = dark ? "rgba(0,0,0,0.45)" : "rgba(60,40,20,0.45)";
  for (const r of world.regions) {
    if (canEnter(nation, r)) continue;
    const [x, y] = center(r);
    hexPath(x, y, size); ctx.fill();
  }
}

// a battle: crossed sabres in a red burst above the armies
function drawBattle(x, y) {
  const cx = x, cy = y - size * 0.45, r = size * 0.22;
  const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, r * 1.6);
  g.addColorStop(0, "rgba(210,45,25,0.95)"); g.addColorStop(0.55, "rgba(220,60,30,0.7)"); g.addColorStop(1, "rgba(220,60,30,0)");
  ctx.fillStyle = g; ctx.beginPath(); ctx.arc(cx, cy, r * 1.6, 0, 2 * Math.PI); ctx.fill();
  ctx.strokeStyle = "#fff6e0"; ctx.lineWidth = Math.max(1.5, size * 0.035); ctx.lineCap = "round";
  ctx.beginPath(); ctx.moveTo(cx - r, cy - r); ctx.lineTo(cx + r, cy + r); ctx.moveTo(cx + r, cy - r); ctx.lineTo(cx - r, cy + r); ctx.stroke();
  ctx.lineWidth = Math.max(1, size * 0.02);
  ctx.beginPath(); ctx.moveTo(cx - r * 0.75, cy + r * 0.45); ctx.lineTo(cx - r * 0.45, cy + r * 0.75);
  ctx.moveTo(cx + r * 0.75, cy + r * 0.45); ctx.lineTo(cx + r * 0.45, cy + r * 0.75); ctx.stroke();
}

// The march button: a pair of boots on your army in the selected tile. Tap
// it, then tap where the army should go.
let boots = null;
const myArmyOn = (tile) => state.armies.find((a) => a.region === tile && a.nation === state.you && state.you >= 0);
const canvasXY = (ev) => { const rect = canvas.getBoundingClientRect(); return [ev.clientX - rect.left, ev.clientY - rect.top]; };
function drawBoots() {
  boots = null;
  const a = selected !== null && myArmyOn(selected);
  if (!a || a.retreating) return;
  const here = state.armies.filter((x) => x.region === selected);
  const [x, y] = center(world.regions[selected]);
  const [bx, by] = armySpot(x, y, here.indexOf(a), here.length);
  const r = Math.max(11, size * 0.15), cx = bx + size * 0.3, cy = by - size * 0.24;
  boots = { x: cx, y: cy, r, army: a.id };
  ctx.beginPath(); ctx.arc(cx, cy, r, 0, 2 * Math.PI);
  ctx.shadowColor = "rgba(0,0,0,0.35)"; ctx.shadowBlur = 4; ctx.shadowOffsetY = 1;
  ctx.fillStyle = moving && selectedArmy === a.id ? cssVar("--select") : cssVar("--accent"); ctx.fill();
  ctx.shadowColor = "transparent";
  ctx.lineWidth = 1.5; ctx.strokeStyle = "#e7c776"; ctx.stroke();
  // a boot: shaft, heel and toe
  const u = r * 0.11, ox = cx - u * 1.5, oy = cy + u * 0.5;
  ctx.beginPath();
  ctx.moveTo(ox - 2 * u, oy - 5 * u); ctx.lineTo(ox + 1.5 * u, oy - 5 * u); ctx.lineTo(ox + 1.5 * u, oy - 0.5 * u);
  ctx.quadraticCurveTo(ox + 5.5 * u, oy - 0.2 * u, ox + 5.5 * u, oy + 2 * u); ctx.lineTo(ox - 2 * u, oy + 2 * u); ctx.closePath();
  ctx.fillStyle = cssVar("--accent-text"); ctx.fill();
  ctx.fillStyle = cssVar("--accent"); ctx.fillRect(ox - 2 * u, oy + 0.9 * u, 7.5 * u, 0.5 * u); // the sole's seam
}

// the marching route of the selected army (and the routes of your others, faintly)
function drawPaths() {
  for (const a of state.armies) {
    if (!a.path.length || (a.id !== selectedArmy && a.nation !== state.you)) continue;
    const pts = [a.region, ...a.path].map((id) => center(world.regions[id]));
    ctx.setLineDash([2, 5]); ctx.lineCap = "round"; ctx.lineWidth = a.id === selectedArmy ? 3.5 : 2;
    ctx.strokeStyle = nationColor(a.nation);
    ctx.beginPath(); pts.forEach(([x, y], i) => (i ? ctx.lineTo(x, y + size * 0.12) : ctx.moveTo(x, y + size * 0.12))); ctx.stroke();
    ctx.setLineDash([]);
    const [ex, ey] = pts[pts.length - 1];
    ctx.font = `${Math.max(12, size * 0.3)}px serif`; ctx.fillStyle = nationColor(a.nation); ctx.textAlign = "center";
    ctx.fillText("✕", ex, ey + size * 0.2);
  }
}

// Sea routes sail through the ocean: from a port out to the nearest ocean
// hex, along the coast, and in to the other port (a shortest path over the
// ring of ocean tiles around the map).
const cellNeighbours = (row, col) => {
  const odd = row & 1;
  return [[0, 1], [1, odd ? 1 : 0], [1, odd ? 0 : -1], [0, -1], [-1, odd ? 0 : -1], [-1, odd ? 1 : 0]].map(([dr, dc]) => [row + dr, col + dc]);
};
const isOcean = (row, col) => row >= -1 && row <= world.rows && col >= -1 && col <= world.cols && !grid[`${row},${col}`];
const seaRoutes = new Map(); // "a-b" -> cells [[row, col], ...] between the two ports, or null
function seaCells(a, b) {
  const key = `${a}-${b}`;
  if (seaRoutes.has(key)) return seaRoutes.get(key);
  const ra = world.regions[a], rb = world.regions[b];
  const goal = new Set(cellNeighbours(rb.row, rb.col).filter(([r, c]) => isOcean(r, c)).map(([r, c]) => `${r},${c}`));
  const prev = new Map(), queue = [];
  for (const [r, c] of cellNeighbours(ra.row, ra.col)) {
    if (isOcean(r, c)) { prev.set(`${r},${c}`, null); queue.push([r, c]); }
  }
  let found = null;
  while (queue.length && !found) {
    const [r, c] = queue.shift();
    if (goal.has(`${r},${c}`)) { found = `${r},${c}`; break; }
    for (const [nr, nc] of cellNeighbours(r, c)) {
      const k = `${nr},${nc}`;
      if (isOcean(nr, nc) && !prev.has(k)) { prev.set(k, `${r},${c}`); queue.push([nr, nc]); }
    }
  }
  let cells = null;
  if (found) {
    cells = [];
    for (let k = found; k !== null; k = prev.get(k)) cells.unshift(k.split(",").map(Number));
  }
  seaRoutes.set(key, cells);
  return cells;
}

// a polyline shifted sideways by off (to keep the two directions apart)
function offsetLine(pts, off) {
  return pts.map(([x, y], i) => {
    const [px, py] = pts[Math.max(0, i - 1)], [nx, ny] = pts[Math.min(pts.length - 1, i + 1)];
    const len = Math.hypot(nx - px, ny - py) || 1;
    return [x - (ny - py) / len * off, y + (nx - px) / len * off];
  });
}

function drawFlows() {
  const fg = $("flow-good").value, agg = new Map();
  for (const f of state.flows) {
    if (fg !== "all" && f.good !== +fg) continue;
    const v = fg === "all" ? f.qty * state.regions[f.from].prices[f.good] : f.qty;
    const k = f.from * 1000 + f.to + (f.sea ? 1e6 : 0);
    agg.set(k, (agg.get(k) || 0) + v);
  }
  const max = Math.max(1e-9, ...agg.values());
  ctx.strokeStyle = ctx.fillStyle = cssVar("--flow");
  for (const [key, v] of agg) {
    // width grows with the square root of the value, relative to the biggest
    // route, so area tracks value; tiny routes fade out
    const rel = Math.sqrt(v / max);
    if (rel < 0.12) continue;
    const sea = key >= 1e6, k = key % 1e6, from = Math.floor(k / 1000), to = k % 1000;
    const w = Math.max(0.8, size * 0.2 * rel);
    ctx.globalAlpha = 0.35 + 0.65 * rel;
    let pts = [center(world.regions[from]), center(world.regions[to])];
    if (sea) {
      const cells = seaCells(from, to);
      if (cells) pts = [pts[0], ...cells.map(([row, col]) => center({ row, col })), pts[1]];
    }
    pts = offsetLine(pts, size * 0.12);
    // trim the ends off the tiles' middles
    const trim = (p, q, d) => { const len = Math.hypot(q[0] - p[0], q[1] - p[1]) || 1; return [p[0] + (q[0] - p[0]) / len * d, p[1] + (q[1] - p[1]) / len * d]; };
    pts[0] = trim(pts[0], pts[1], size * 0.35);
    pts[pts.length - 1] = trim(pts[pts.length - 1], pts[pts.length - 2], size * 0.45);
    ctx.setLineDash(sea ? [8, 6] : []);
    ctx.lineWidth = w; ctx.lineCap = "round"; ctx.lineJoin = "round";
    ctx.beginPath(); ctx.moveTo(...pts[0]);
    for (let i = 1; i < pts.length - 1; i++) { // smooth through the hexes' middles
      const mx = (pts[i][0] + pts[i + 1][0]) / 2, my = (pts[i][1] + pts[i + 1][1]) / 2;
      ctx.quadraticCurveTo(pts[i][0], pts[i][1], mx, my);
    }
    ctx.lineTo(...pts[pts.length - 1]); ctx.stroke();
    ctx.setLineDash([]);
    const [bx, by] = pts[pts.length - 1], [px, py] = pts[pts.length - 2];
    const len = Math.hypot(bx - px, by - py) || 1, ux = (bx - px) / len, uy = (by - py) / len, h = 3 + w * 1.1;
    ctx.beginPath(); ctx.moveTo(bx + ux * h, by + uy * h); ctx.lineTo(bx - uy * h * 0.6, by + ux * h * 0.6); ctx.lineTo(bx + uy * h * 0.6, by - ux * h * 0.6);
    ctx.closePath(); ctx.fill();
  }
  ctx.globalAlpha = 1;
}

function drawLegend(sc) {
  const kind = $("overlay").value, el = $("legend");
  const sw = (c) => `<span class="swatch" style="background:${c}"></span>`;
  const marks = `<span>⌂ towns grow with their people</span><span>✕ battle</span><span>⚓ port</span><span>⛨ army</span><span>▨ occupied</span><span>┄ road · ═ paved · ╫ railway</span>`;
  const pal = TERRAIN[dark ? "dark" : "light"];
  if (kind === "nation") {
    el.innerHTML = world.nations.map((n) => `<span>${sw(nationColor(n.id))}${esc(n.name)}</span>`).join("") + marks;
  } else if (kind === "infra") {
    el.innerHTML = world.infra_levels.map((l) => `<span>${esc(l.name)} ×${l.move_mult}</span>`).join(" · ") + `<span>roads and railways shown along the links</span>`;
  } else if (kind === "terrain") {
    el.innerHTML = world.terrains.map((t) => `<span>${sw(pal[t.name])}${t.name}</span>`).join("") + `<span>${sw(OCEAN[dark ? "dark" : "light"])}ocean: tiles beside it are coastal (fishing, cheap shipping, ports)</span>` + marks;
  } else {
    el.innerHTML = `<span>${esc(overlayLabel())}</span><span>${fmtOverlay(sc.lo)}</span><span class="ramp" style="background:linear-gradient(90deg,${RAMP.join(",")})"></span><span>${fmtOverlay(sc.hi)}</span>`;
  }
}

function regionAt(ev) {
  const rect = canvas.getBoundingClientRect(), x = ev.clientX - rect.left, y = ev.clientY - rect.top;
  let best = null, bd = Infinity;
  for (const r of world.regions) {
    const [cx, cy] = center(r), d = Math.hypot(cx - x, cy - y);
    if (d < bd) { bd = d; best = r; }
  }
  return bd < size ? best : null;
}
function armyAt(ev, region) {
  const rect = canvas.getBoundingClientRect(), x = ev.clientX - rect.left, y = ev.clientY - rect.top;
  const here = state.armies.filter((a) => a.region === region.id);
  const [cx, cy] = center(region);
  for (let i = 0; i < here.length; i++) {
    const [bx, by] = armySpot(cx, cy, i, here.length);
    if (Math.hypot(bx - x, by - y) < size * 0.24) return here[i];
  }
  return null;
}
// panning and zooming a big map: drag (a mouse or a finger), pinch, the
// wheel, and the + / − buttons. A drag isn't a click.
const pointers = new Map(); // pointer id -> [x, y]
let dragged = false, pinch = null;
canvas.addEventListener("pointerdown", (ev) => {
  if (!zoomable()) return;
  pointers.set(ev.pointerId, canvasXY(ev));
  dragged = false;
  if (pointers.size === 2) {
    const [a, b] = [...pointers.values()];
    pinch = { d: Math.hypot(a[0] - b[0], a[1] - b[1]), zoom: view.zoom };
  }
});
canvas.addEventListener("pointermove", (ev) => {
  const last = pointers.get(ev.pointerId);
  if (!last) return;
  const p = canvasXY(ev);
  pointers.set(ev.pointerId, p);
  if (pointers.size === 2 && pinch) {
    const [a, b] = [...pointers.values()];
    dragged = true;
    zoomTo(pinch.zoom * Math.hypot(a[0] - b[0], a[1] - b[1]) / pinch.d, (a[0] + b[0]) / 2, (a[1] + b[1]) / 2);
    return;
  }
  const dx = p[0] - last[0], dy = p[1] - last[1];
  if (!dragged && Math.hypot(dx, dy) < 4) { pointers.set(ev.pointerId, last); return; } // a click's jitter
  if (!dragged) canvas.setPointerCapture(ev.pointerId);
  dragged = true;
  view.x -= dx; view.y -= dy;
  clampView();
  $("tooltip").hidden = true;
  requestDraw();
});
const lift = (ev) => { pointers.delete(ev.pointerId); if (pointers.size < 2) pinch = null; };
canvas.addEventListener("pointerup", lift);
canvas.addEventListener("pointercancel", lift);
canvas.addEventListener("wheel", (ev) => {
  if (!zoomable()) return;
  ev.preventDefault();
  zoomTo(view.zoom * Math.exp(-ev.deltaY * (ev.deltaMode ? 0.05 : 0.0015)), ...canvasXY(ev));
}, { passive: false });
$("zoom").addEventListener("click", (ev) => {
  const b = ev.target.closest("button");
  if (b) zoomTo(view.zoom * (b.dataset.zoom === "in" ? 1.5 : 1 / 1.5));
});

canvas.addEventListener("click", async (ev) => {
  if (dragged) { dragged = false; return; } // the end of a drag
  if (boots && Math.hypot(...canvasXY(ev).map((v, i) => v - [boots.x, boots.y][i])) < boots.r * 1.4) {
    selectedArmy = boots.army; moving = true; // march: now pick the destination
    render(true);
    return;
  }
  const r = regionAt(ev);
  if (!r) return;
  if (moving && selectedArmy !== null) {
    moving = false;
    await command(`/armies/${selectedArmy}/move?to=${r.id}`);
    return;
  }
  const a = armyAt(ev, r);
  if (a) selectedArmy = a.id;
  const changed = selected !== r.id;
  selected = r.id;
  if (changed || !a) setTab("tile");
  try { detail = await api("GET", `/api/games/${gid}/regions/${r.id}`); } catch (e) { showError("game-err", e); }
  render(true);
  // on a phone, show the tile's panel, unless the tap was to command an army here
  if (window.innerWidth <= 860 && !myArmyOn(r.id)) document.querySelector(".tabs").scrollIntoView({ behavior: "smooth", block: "start" });
});
$("mode").addEventListener("click", (ev) => { if (ev.target.closest("#cancel-move")) { moving = false; render(true); } });
document.addEventListener("keydown", (ev) => { if (ev.key === "Escape" && moving) { moving = false; render(true); } });
canvas.addEventListener("mousemove", (ev) => {
  const r = regionAt(ev), tip = $("tooltip");
  if (!r || !state) { tip.hidden = true; return; }
  const rs = state.regions[r.id];
  const v = CATEGORICAL.includes($("overlay").value) ? "" : ` · ${overlayLabel()}: ${fmtOverlay(overlayValue(r.id))}`;
  tip.textContent = `${r.name}, ${r.coastal ? "coastal " : ""}${r.terrain}${rs.city ? " city" : ""} · ${nationName(rs.nation)} · ${short(rs.population)} people${v}`;
  const rect = canvas.getBoundingClientRect();
  tip.style.left = ev.clientX - rect.left + 12 + "px"; tip.style.top = ev.clientY - rect.top + 12 + "px";
  tip.hidden = false;
});
canvas.addEventListener("mouseleave", () => { $("tooltip").hidden = true; });
for (const id of ["overlay", "good", "show-flows", "flow-good"]) {
  $(id).addEventListener("change", async () => {
    $("good").hidden = $("overlay").value !== "price";
    $("flow-good").hidden = !$("show-flows").checked;
    if (id === "show-flows" && $("show-flows").checked && !state.flows.length) {
      try { state = await fetchState(); } catch (e) { showError("game-err", e); }
    }
    draw();
  });
}

// ---- tile panel ----------------------------------------------------------------
const flowValue = (f) => f.qty * state.regions[f.from].prices[f.good];
function tilePanel() {
  if (selected === null || !detail || detail.info.id !== selected) return `<p class="hint">Choose a tile on the map to open its ledger.</p>`;
  const d = detail, s = state.regions[selected], G = world.goods;
  const mine = state.you >= 0 && s.nation === state.you;
  const h = getHist(`regions/${selected}`);
  const exp = d.exports.reduce((t, f) => t + flowValue(f), 0), imp = d.imports.reduce((t, f) => t + flowValue(f), 0);
  let html = `<div class="crest">${shield(nationColor(s.nation))}<div><h2>${esc(d.info.name)}${s.city ? " <span class=\"sub\">· city</span>" : ""}</h2>
    <p class="sub">${cap(d.info.terrain)}${d.info.coastal ? " by the sea" : ""} · ${esc(nationName(s.nation))} · its size makes industry and services ×${s.city_bonus.toFixed(2)} as productive</p></div></div>
    <div class="stats">
      ${stat(short(s.population), "people", `<span class="${s.growth >= 0 ? "good" : "bad"}">${s.growth >= 0 ? "+" : ""}${(100 * s.growth).toFixed(1)}%/yr</span>`)}
      ${stat(fmt(s.wage), "daily wage", trend(h, "wage"))}
      ${stat(fmt(s.utility), "living standard", trend(h, "utility"))}
      ${stat("£" + short(d.gdp), "real GDP / day, 1820 £", trend(h, "gdp"))}
      ${stat("£" + short(exp), "exports / day")}
      ${stat("£" + short(imp), "imports / day")}
    </div>`;
  if (s.nation >= 0 && s.allegiance < 0.995) {
    const split = (s.loyalties || []).map((v, i) => [i, v]).filter(([, v]) => v > 0.005).sort((a, b) => b[1] - a[1])
      .map(([i, v]) => `${i < world.nations.length ? esc(world.nations[i].name) : "nobody"} ${pct(v)}`).join(", ");
    html += `<p class="sub" style="margin-top:8px"><b>Allegiance ${pct(s.allegiance)}</b>: its people stand by ${split} · production ×${s.work_mult.toFixed(2)}, taxes ×${(0.5 + 0.5 * s.allegiance).toFixed(2)}, recruits ${pct(s.allegiance)}
      ${info(`A tile's people hold a share of loyalty for each nation, and for nobody. Day by day it drifts to whoever owns the tile (about 70% after ten years). The owner's share, plus 40% of nobody's, is its allegiance: until it is whole the tile pays taxes at half to all of the usual and gives only that share of recruits; people still loyal to another nation also work half-heartedly (production falls by half their share), while people who answered to nobody work as before (disloyal people don't count towards your mobilisation cap either). A conquered tile starts loyal to its old owner; free land answers to nobody; a tile lost and soon retaken still mostly stands by you.`)}</p>`;
  }
  if (s.services) html += `<p class="sub" style="margin-top:8px">Public services: ${s.nation >= 0 ? `taxes collected ${pct(s.collect)} · ` : ""}educated ${pct(s.educated)} ·
    public health ${healthGrowth(s)} a year</p>`;
  html += `<h3>Over time</h3>` + tileChart(h);
  html += classesBlock(d);
  html += armiesHere(s, mine);
  html += `<h3>Roads and harbour ${info(`Roads, paved roads and railways make it cheaper to move goods and armies through a tile, and cost construction to build and some upkeep each day (neglected, they decay). Tiles by the sea can be crossed by ship cheaply and have fishing; a port sails straight to every other port.`)}</h3>${infraBlock(s, mine)}`;
  const total = s.tier_spend.reduce((a, b) => a + b, 0) || 1, tc = TIER_COLORS();
  html += `<h3>What people buy</h3><div class="bar-stack">${world.tiers.map((t, i) => `<span style="width:${(100 * s.tier_spend[i] / total).toFixed(1)}%;background:${tc[i]}"></span>`).join("")}</div>
    <div class="legend-inline">${world.tiers.map((t, i) => `<span><span class="swatch" style="background:${tc[i]}"></span>${t} ${pct(s.tier_spend[i] / total)}</span>`).join("")}</div>`;
  const cons = world.goods.findIndex((g) => g.name === "construction");
  const levelPrice = world.level_cost * ((d.markets.find((x) => x.good === cons) || { price: 0 }).price);
  // idle firms (nobody wants what they make here yet, or it doesn't pay)
  // are listed last, so they can still be expanded
  const busy = (f) => f.labor > 0.05 || f.produced > 0.05 || f.levels > 1 || f.expansion;
  const firms = d.firms.map((f, i) => ({ ...f, i }))
    .sort((a, b) => (b.enabled - a.enabled) || (b.state - a.state) || (!!busy(b) - !!busy(a)) || (b.labor - a.labor)).map((f) => {
      if (!f.enabled) return `<tr class="dim"><td>${esc(f.name)}</td><td>${G[f.output].name}</td><td colspan="4">locked: needs ${esc(f.requires)}</td></tr>`;
      let grow = "";
      if (f.expansion) grow = `${pct(f.expansion.delivered / world.level_cost)}${f.expansion.public ? " ★" : ""}`;
      else if (mine && !f.state && !(f.max_levels > 0 && f.levels >= f.max_levels)) grow = `<button data-cmd="/regions/${selected}/firms/${f.i}/expand" title="Your nation pays ${world.level_cost} construction for a level">+1</button>`;
      // productivity bonuses: everything but the recipe's base, as ×factors
      const bon = (f.bonuses || []).slice(1).map((m) => `${esc(m.name.toLowerCase())} ×${m.value.toFixed(2)}`).join(" · ");
      const total = (f.bonuses || []).reduce((t, m) => t * m.value, 1);
      const tip = (f.bonuses || []).map((m) => `${m.name}: ×${m.value.toFixed(2)} (${m.note})`).join("\n");
      if (f.state) return `<tr class="dim"><td>${esc(f.name)}<br><span class="sub">public · ${f.levels} levels, run from the ${f.name === "University" ? "Research" : "State"} tab</span></td><td>${f.name === "University" ? "research" : "service"}</td><td>${fmt(f.labor, 1)}</td><td colspan="3"></td></tr>`;
      const m = d.markets.find((x) => x.good === f.output) || { supply: 0, demand: 0 };
      // profit per worker: the value each worker adds a day (output less the goods used up) less the wage
      const per = f.labor > 0.01 ? `<span class="${f.per_worker >= 0 ? "good" : "bad"}">${f.per_worker < 0 ? "−" : ""}£${fmt(Math.abs(f.per_worker))}</span>` : "–";
      return `<tr${busy(f) ? "" : ` class="idle"`} title="${esc(tip)}${busy(f) ? "" : "\nidle: it doesn't pay to make this here at today's prices"}\nproductivity ×${total.toFixed(2)}\nprofit £${fmt(f.profit, 2)} a day"><td>${esc(f.name)}<br><span class="sub">${f.sector}${bon ? ` · ${bon}` : ""}</span></td>
        <td>${G[f.output].name}<br><span class="sub">${fmt(f.produced, 1)} a day</span></td>
        <td class="nowrap">${fmt(f.labor, 1)} / ${fmt(f.wanted, 1)}</td><td class="nowrap">${fmt(m.supply, 1)} / ${fmt(m.demand, 1)}</td><td>${per}</td><td>${f.max_levels > 0 ? `${f.levels}/${f.max_levels}` : f.levels}${grow ? `<br>${grow}` : ""}${f.level_gain > 0.005 && !f.expansion ? `<br><span class="sub" title="Extra output a day one more level would bring, from the same workers and inputs: a level (£${fmt(levelPrice, 0)}) pays for itself in about ${Math.round(levelPrice / f.level_gain)} days">+£${fmt(f.level_gain, 1)}</span>` : ""}</td></tr>`;
    }).join("");
  const employed = d.firms.reduce((t, f) => t + f.labor, 0), workers = (d.classes.find((c) => c.class === "workers") || { size: 0 }).size;
  html += `<h3>Production ${info(`Every firm on a tile pays the same wage, set by the tile's labour market; a firm only hires the workers it can pay.<br><br><b>Profit / worker</b> is what each worker earns the owners a day: the value they add (output less the goods used up) less their wage. Red means the firm is losing money on its workers.<br><br><b>Supply / demand</b> are the tile's market for what the firm makes.<br><br>Under each firm, what multiplies its productivity: its site, technology, the tile's size and schooling, allegiance, and crowding (how much more productive it would be with one more building level). Hover a firm for the details and its profit.<br><br>Under the levels, the £ a day one more level would add from the same workers and inputs (hover for how soon it pays for itself). Owners build a level when it would pay for itself within ten years, and spend 2% of their levels' worth a year on upkeep; ★ an expansion paid by the state.<br><br>Greyed-out firms stand idle: at today's prices it doesn't pay to make their goods here (war materiel in peacetime, say). You can still expand them; they start up once buyers come.`)}</h3>
    <div class="stats">${stat(fmt(s.wage), "the tile's daily wage")}${stat(`${short(employed)} / ${short(workers)}`, "workers employed")}${stat(pct(workers > 0 ? employed / workers : 0), "employment")}</div>
    <div class="table-scroll" style="margin-top:8px"><table><tr><th>firm</th><th>makes</th><th class="nowrap">workers<br>/ wanted</th><th class="nowrap">supply<br>/ demand</th><th class="nowrap">profit /<br>worker</th><th>levels</th></tr>${firms}</table></div>`;
  // the market: local supply and demand, and what ships in and out
  const ex = {}, im = {};
  for (const f of d.exports) ex[f.good] = (ex[f.good] || 0) + f.qty;
  for (const f of d.imports) im[f.good] = (im[f.good] || 0) + f.qty;
  const row = (m) => `<tr><td>${G[m.good].name}</td><td>${fmt(m.price)}</td><td>${fmt(m.supply)}</td><td>${fmt(m.demand)}</td><td>${im[m.good] ? fmt(im[m.good]) : "–"}</td><td>${ex[m.good] ? fmt(ex[m.good]) : "–"}</td></tr>`;
  const live = d.markets.filter((m) => m.supply > 1e-3 || m.demand > 1e-3 || ex[m.good] || im[m.good]);
  const head = (label) => `<tr class="branch"><th colspan="6">${label}</th></tr><tr><th>good</th><th>price</th><th>supply</th><th>demand</th><th>imports</th><th>exports</th></tr>`;
  html += `<h3>Market ${info(`Each good trades once a day at that morning's price. <b>Supply</b> is what was offered for sale today (local output and imports arriving), <b>demand</b> what buyers wanted (people, firms, the state and merchants shipping out). The short side is rationed in proportion, and the price moves by up to about 10% a day towards balance. Imports and exports are today's shipments.`)}</h3><div class="table-scroll"><table>${head("What people buy")}${live.filter((m) => G[m.good].tier).map(row).join("")}
    ${head("Materials and services")}${live.filter((m) => !G[m.good].tier).map(row).join("")}</table></div>`;
  const trade = (list, out) => list.sort((a, b) => flowValue(b) - flowValue(a)).slice(0, 8).map((f) =>
    `<tr><td>${G[f.good].name}</td><td>${out ? "→" : "←"} ${world.regions[out ? f.to : f.from].name}${f.sea ? " ⚓" : ""}</td><td>${fmt(f.qty)}</td><td>${fmt(flowValue(f))}</td></tr>`).join("") || `<tr class="dim"><td colspan="4">none</td></tr>`;
  html += `<h3>The trading house ${info(`The tile's merchants share one purse across its routes (one per good and destination, by land and sea). They buy where a good is cheap and ship it where it sells for more than the price plus transport; when money is short it goes to the best margins first. Profits go to the tile's owners.`)}</h3>
    <div class="stats">${stat(short(d.house.cash), "purse")}${stat(short(d.house.spent), "spent today")}${stat(`${d.house.active}/${d.house.routes}`, "routes in use")}</div>`;
  html += `<h3>Shipments today</h3><table><tr><th>exports</th><th>to</th><th>qty</th><th>value</th></tr>${trade(d.exports, true)}
    <tr><th>imports</th><th>from</th><th>qty</th><th>value</th></tr>${trade(d.imports, false)}</table>`;
  return html;
}

function tileChart(h) {
  const [k, row] = seg("tile", [["population", "People"], ["wage", "Wage"], ["utility", "Living"], ["gdp", "GDP"], ["trade", "Trade"], ["classes", "Classes"], ["prices", "Prices"]], "population");
  if (!h || !h.data) return row + lineChart(null, []);
  const d = h.data, T = h.ticks;
  const one = (name, values, f) => lineChart(T, [{ name, color: series(0), values }], { f });
  let chart;
  if (k === "trade") chart = lineChart(T, [{ name: "exports", color: series(1), values: d.exports }, { name: "imports", color: series(2), values: d.imports }], { f: money });
  else if (k === "classes") chart = lineChart(T, world.classes.map((c, i) => ({ name: c, color: series(i), values: d.class_income[i] })), { f: (v) => fmt(v) }) + `<p class="sub">Income per head per day.</p>`;
  else if (k === "prices") {
    const [g, prow] = pickSelect("tile-price", world.consumer_goods.map((g) => [String(g), world.goods[g].name]), String(world.consumer_goods[0]), "Good to chart");
    const idx = world.consumer_goods.indexOf(+g);
    chart = prow + lineChart(T, [{ name: `${world.goods[+g].name} price`, color: series(0), values: d.prices[idx] }], { f: (v) => fmt(v) });
  } else chart = one({ population: "people", wage: "wage", utility: "living standard", gdp: "real GDP per day (1820 £)" }[k], d[k], k === "population" ? short : k === "gdp" ? money : (v) => fmt(v));
  return row + chart;
}

function classesBlock(d) {
  if (!d.classes || !d.classes.length) return "";
  const total = d.classes.reduce((t, c) => t + c.size, 0) || 1;
  const rows = d.classes.map((c, i) => `<tr><td><span class="swatch" style="background:${series(i)}"></span>${cap(c.class)}</td>
    <td>${short(c.size)}</td><td>${pct(c.size / total)}</td><td>${fmt(c.income)}</td><td>${fmt(c.utility)}</td><td>${short(c.cash)}</td></tr>`).join("");
  return `<h3>Society ${info(`Workers earn wages; owners hold the land and the means of production (farms, mines, works and trading houses) and live off their profits.`)}</h3><div class="table-scroll"><table><tr><th>class</th><th>people</th><th>share</th><th>income/head</th><th>living</th><th>savings</th></tr>${rows}</table></div>`;
}

function armiesHere(s, mine) {
  const here = state.armies.filter((a) => a.region === selected);
  let html = "";
  if (s.occupation) {
    html += `<h3>Occupation ${info(`Occupation grows each day an army with a strength of at least ${world.occupy_strength} stands here unopposed (no army of the owner on the tile, no battle), and falls back otherwise. At 100% the tile changes hands. Free land takes about half a year; a nation's tile twice as long, and its fortifications slow it further. Occupying costs the army men: about 15% over a full occupation of free land, 40% of a nation's tile.`)}</h3><div class="progress"><span style="width:${pct(s.occupation.progress)};background:${nationColor(s.occupation.nation)}"></span></div>
      <p class="sub">${esc(nationName(s.occupation.nation))} holds ${pct(s.occupation.progress)}.</p>`;
  }
  if (s.battle > 0) {
    html += `<h3>Battle ${info(`Each day of battle each side loses men in proportion to the other's strength. An army fights until its strength is down to a third of what it began with, then falls back to its nearest own tile, losing half its men and kit on the way (surrounded, it fights on to the end). Occupation stands still while it lasts.`)}</h3><p class="bad">Battle raging, day ${s.battle}.</p>`;
  }
  if (here.length) {
    html += `<h3>Armies here</h3>` + here.map((a) =>
      `<div class="row">${chip(a.nation)} <span>${esc(nationName(a.nation))}: strength ${short(a.strength)} (${short(a.men)} men, ${world.doctrines[a.doctrine].name})</span>
       <button data-army="${a.id}">Inspect</button></div>`).join("");
  }
  const n = me();
  if (mine && n && here.some((a) => a.nation === n.id)) {
    html += `<p class="sub">One of your armies holds this tile ${info(`A nation keeps at most one army per tile.`)}</p>`;
  } else if (mine && n) {
    const room = Math.max(0, n.mobilize_cap - state.armies.filter((a) => a.nation === n.id).reduce((t, a) => t + a.target, 0));
    html += `<h3>Raise an army here ${info(`Recruits are drafted from all your tiles, in proportion to their people and allegiance (each gives up to 0.2% of its people a day). An army fills by at most a ninetieth of its full strength a day, so a full mobilisation takes about 90 days; recruits join at once at home, and an army in the field gets them after the march. One army can field up to ${short(n.max_army_size)} men (military techs raise this); your mobilisation cap is ${pct(n.mobilization)} of your people, so field several armies if you need more.`)}</h3>
      <div class="row"><label>Soldiers <input id="raise-men" type="number" min="0" max="${Math.floor(n.max_army_size)}" step="1" value="${Math.floor(Math.min(room, n.max_army_size))}" class="num"></label>
      <button class="primary" id="raise">Raise</button></div>
      <p class="sub">Up to ${short(n.max_army_size)} men per army · room for ${short(room)} more within your mobilisation cap.</p>`;
  }
  return html;
}

const knows = (nation, techName) => !techName || state.nations[nation].known[world.techs.findIndex((t) => t.name === techName)];
const canPort = (nation) => world.techs.some((t, i) => t.ports && state.nations[nation].known[i]);
const portTech = () => (world.techs.find((t) => t.ports) || {}).name;

function infraBlock(s, mine) {
  const lvl = world.infra_levels[s.infra], next = world.infra_levels[s.infra + 1];
  const coast = world.regions[selected].coastal;
  let html = `<p class="sub">${cap(lvl.name)} · moving through costs ×${s.move_cost.toFixed(2)}${lvl.upkeep ? ` · upkeep ${lvl.upkeep}/day` : ""}</p>`;
  const nat = s.nation >= 0 ? state.nations[s.nation] : null;
  const allowance = nat ? ` · ${esc(nationName(s.nation))} has ${nat.ports} of the ${nat.port_allowance} port${nat.port_allowance === 1 ? "" : "s"} it may have ${info(`A nation may have one port for every ${world.tiles_per_port} tiles it holds, and at least one. Ports being built count. Every port trades with every other. A nation that conquers ports keeps them, but builds no more until it is back under its allowance.`)}` : "";
  if (coast) html += `<p class="sub">Coastal · ${s.port ? "⚓ it has a port" : "no port yet"}${allowance}</p>`;
  if (mine && s.port && !s.project) {
    html += `<div class="row"><button data-cmd="/regions/${selected}/unport" data-confirm="Pull down this port? Its sea routes close, and the goods they have bought or have at sea are lost.">Pull down port</button><span class="sub">frees it for a port elsewhere</span></div>`;
  }
  if (s.project) {
    const what = s.project.port ? "port" : world.infra_levels[s.project.target].name;
    html += `<div class="progress"><span style="width:${pct(s.project.delivered / s.project.required)}"></span></div>
      <div class="row"><span class="sub">Building ${what}: ${fmt(s.project.delivered, 0)} / ${s.project.required}</span>
      ${mine ? `<button data-cmd="/regions/${selected}/cancel">Cancel</button>` : ""}</div>`;
    return html;
  }
  if (mine && coast && !s.port) {
    const mats = Object.entries(world.port.materials).map(([g, q]) => ` + ${q} ${g}`).join("");
    html += !canPort(state.you) ? `<p class="sub">A port needs ${portTech()}.</p>`
      : nat && nat.ports >= nat.port_allowance ? `<p class="sub">No more ports: pull one down to build here, or hold more land.</p>`
      : `<div class="row"><button class="primary" data-cmd="/regions/${selected}/port">Build port</button><span class="sub">${world.port.cost} construction${mats}</span></div>`;
  }
  if (next && mine) {
    const mats = Object.entries(next.materials || {}).map(([g, q]) => ` + ${q} ${g}`).join("");
    html += knows(state.you, next.tech)
      ? `<div class="row"><button class="primary" data-cmd="/regions/${selected}/build">Build ${next.name}</button><span class="sub">${next.cost} construction${mats}</span></div>`
      : `<p class="sub">${cap(next.name)} needs ${next.tech}.</p>`;
  }
  return html;
}

// ---- nation panel --------------------------------------------------------------
function nationPicker() {
  return `<div class="seg">${world.nations.map((o) => `<button data-view-nation="${o.id}" class="${o.id === viewNation ? "on" : ""}">${chip(o.id)} ${esc(o.name)}${o.id === state.you ? " (you)" : ""}</button>`).join("")}</div>`;
}

function nationPanel() {
  const n = state.nations[viewNation], info = world.nations[n.id];
  const mine = n.id === state.you;
  const h = getHist(`nations/${n.id}`);
  const who = mine ? "your nation" : n.controller === "ai" ? "a computer nation" : "another player's nation";
  const wars = n.at_war.map((w, i) => (w ? world.nations[i].name : null)).filter(Boolean);
  let html = nationPicker() + `<div class="crest">${shield(nationColor(n.id), 34)}<div><h2>${esc(info.name)}</h2>
    <p class="sub">${who}${n.eliminated ? " · fallen" : ""} · ${wars.length ? `<span class="bad">at war with ${wars.map(esc).join(", ")}</span>` : "at peace"}</p></div></div>`;
  if (n.eliminated) return html + `<p class="hint">All its tiles were taken.</p>`;
  const known = n.known.filter(Boolean).length;
  html += `<div class="stats four">
    ${stat(short(n.population), "people", trend(h, "population"))}
    ${stat("£" + short(n.gdp), "real GDP / day, 1820 £", trend(h, "gdp"))}
    ${stat("£" + short(n.income), "state income / day", trend(h, "income"))}
    ${stat("£" + short(n.treasury), "treasury")}
    ${stat(fmt(n.utility), "living standard", trend(h, "utility"))}
    ${stat(short(n.soldiers), "soldiers", `cap ${short(n.mobilize_cap)}`)}
    ${stat(`${known}/${world.techs.length}`, "techs known")}
    ${stat(n.tiles, "tiles")}
  </div>`;
  html += `<h3>The national ledger</h3>` + nationChart(h, n);
  html += societyBlock(n);
  html += pricesBlock(h, n);
  return html;
}

// the nation's price index: each good's price across its tiles, weighted by
// what changed hands, with today's volume and the change over a year
function pricesBlock(h, n) {
  if (!n.prices) return "";
  const goods = world.goods.map((g, i) => i).filter((g) => (!state.made || state.made[g]) && n.traded[g] > 1e-3);
  const opts = goods.map((g) => [String(g), world.goods[g].name]);
  let html = `<h3>Prices ${info(`The nation's price for each good: the average across its tiles, weighted by how much changed hands at each, so a price nobody pays doesn't count. Traded is the amount bought and sold a day across the nation; the change is over the last year.`)}</h3>`;
  if (!goods.length) return html + `<p class="hint">Nothing traded today.</p>`;
  const series_ = h && h.data && h.data.prices;
  if (series_) {
    const [g, prow] = pickSelect("nation-price", opts, opts.find(([k]) => world.goods[+k].name === "bread") ? String(world.goods.findIndex((x) => x.name === "bread")) : opts[0][0], "Good to chart");
    html += prow + lineChart(h.ticks, [{ name: `${world.goods[+g].name} price`, color: nationColor(n.id), values: series_[+g].map((v) => (v > 0 ? v : null)) }], { f: (v) => fmt(v) });
  }
  const change = (g) => {
    const s = series_ && series_[g];
    if (!s || s.length < 13 || !(s[s.length - 13] > 0)) return "–";
    const r = n.prices[g] / s[s.length - 13] - 1;
    return `<span class="${r > 0.005 ? "bad" : r < -0.005 ? "good" : ""}">${r > 0 ? "+" : r < 0 ? "−" : ""}${pct(Math.abs(r))}</span>`;
  };
  const row = (g) => `<tr><td>${world.goods[g].name}</td><td>${fmt(n.prices[g])}</td><td>${fmt(n.traded[g], 1)}</td><td>${change(g)}</td></tr>`;
  const head = (label) => `<tr class="branch"><th colspan="4">${label}</th></tr><tr><th>good</th><th>price</th><th>traded / day</th><th>a year</th></tr>`;
  html += `<div class="table-scroll"><table>${head("What people buy")}${goods.filter((g) => world.goods[g].tier).map(row).join("")}
    ${head("Materials and services")}${goods.filter((g) => !world.goods[g].tier).map(row).join("")}</table></div>`;
  return html;
}

// what real GDP means, under its charts
const GDP_NOTE = "Real GDP: what the firms add (their output less the goods they use up), valued at constant prices in pounds of January 1820, linked month by month. The money in the world is fixed, so prices fall as output grows; real GDP counts the goods, not the pounds.";

function nationChart(h, n) {
  const [k, row] = seg("nation", [["population", "People"], ["gdp", "GDP"], ["income", "Revenue"], ["treasury", "Treasury"], ["utility", "Living"],
    ["classes", "Classes"], ["labor", "Labour"], ["soldiers", "Soldiers"], ["strength", "Might"], ["techs", "Techs"], ["tiles", "Tiles"]], "population");
  if (!h || !h.data) return row + lineChart(null, []);
  const d = h.data, T = h.ticks, c = nationColor(n.id);
  const names = { population: "people", gdp: "real GDP per day (1820 £)", income: "state income per day", treasury: "treasury", utility: "living standard",
    soldiers: "soldiers", strength: "military strength", techs: "techs known", tiles: "tiles" };
  if (k === "classes") return row + lineChart(T, world.classes.map((cl, i) => ({ name: cl, color: series(i), values: d.class_income[i] })), { f: (v) => fmt(v) }) + `<p class="sub">Income per head per day.</p>`;
  if (k === "labor") return row + lineChart(T, world.sectors.map((s, i) => ({ name: s, color: series(i), values: d.labor[i] })), { share: true }) + `<p class="sub">Share of workers by sector: the march from the fields into the mills.</p>`;
  if (k === "gdp") return row + lineChart(T, [{ name: names[k], color: c, values: d[k] }], { f: money }) + `<p class="sub">Real GDP, in pounds of January 1820 ${info(GDP_NOTE)}</p>`;
  return row + lineChart(T, [{ name: names[k], color: c, values: d[k] }], { f: k === "utility" ? (v) => fmt(v) : ["gdp", "income", "treasury"].includes(k) ? money : short });
}

function societyBlock(n) {
  const sc = n.labor.map((v, i) => [world.sectors[i], v, series(i)]);
  const base = n.class_income[0] || 1e-9;
  return `<h3>Society</h3>
    <table><tr><th>class</th><th>income / head</th><th>× workers</th></tr>
    ${world.classes.map((c, i) => `<tr><td><span class="swatch" style="background:${series(i)}"></span>${cap(c)}</td><td>${fmt(n.class_income[i])}</td><td>${(n.class_income[i] / base).toFixed(1)}</td></tr>`).join("")}</table>
    <p class="sub" style="margin-top:10px">Where people work</p>
    <div class="bar-stack">${sc.map(([, v, col]) => `<span style="width:${(100 * v).toFixed(1)}%;background:${col}"></span>`).join("")}</div>
    <div class="legend-inline">${sc.map(([s, v, col]) => `<span><span class="swatch" style="background:${col}"></span>${s} ${pct(v)}</span>`).join("")}</div>`;
}

// the state's accounts today: what came in, what it paid for, and the treasury
function accountsBlock(n, mine) {
  const spent = n.spent_on.reduce((a, b) => a + b, 0);
  const row = (label, v, note = "") => `<tr><td>${label}${note ? `<br><span class="sub">${note}</span>` : ""}</td><td>${v < 0 ? "−" : ""}£${fmt(Math.abs(v), 1)}</td></tr>`;
  const names = { economy: "Roads, upkeep and construction", science: "Universities' staff", military: "Armies: supplies and soldiers' pay",
    administration: "Officials", education: "Teachers", health: "Public health", pensions: "Pensions" };
  let html = `<h3>The state's accounts, today ${info(`The state pays for everything it needs: roads, armies, officials, teachers, doctors, scholars and pensions. When the treasury can't cover a day's bills, everything is cut alike, and unpaid soldiers desert.`)}</h3>
    <table>${row("Income tax", n.revenue)}${row("Tariffs", n.tariff_revenue)}
    ${world.spending.map((k, i) => row(names[k] || cap(k), -n.spent_on[i], k === "military" && n.pay_owed > 0 ? `of which pay £${fmt(n.pay_owed * n.paid, 1)}` : "")).join("")}
    <tr class="total"><td>Treasury</td><td>£${fmt(n.treasury, 0)}</td></tr>
    <tr><td>Building budget today ${info(`What the state may spend on roads, ports and new building levels today: up to 60% of its income while the treasury holds 15 days of spending, less as it falls below that and nothing at half of it, plus a thirtieth of anything beyond it.`)}</td><td>£${fmt(n.build_budget, 1)}</td></tr></table>
    ${n.paid < 0.999 ? `<p class="sub bad" style="margin-top:6px">Today only ${pct(n.paid)} of the bills could be paid: everything was cut alike.</p>` : ""}`;
  if (mine) {
    const steps = ["none", "¼ of the wage", "½ of the wage", "¾ of the wage", "the full wage"];
    html += `<div class="row"><label>Tax <input id="tax" type="number" min="0" max="90" step="1" value="${Math.round(100 * n.tax_rate)}" class="num" ${n.auto_tax ? "disabled" : ""}>%</label><button data-input="tax" ${n.auto_tax ? "disabled" : ""}>Set</button>
        <label class="check"><input type="checkbox" id="auto-tax" ${n.auto_tax ? "checked" : ""}> Automate</label>
        ${info(`Tax is taken from people's wages and dividends (where officials collect it). Automated, the rate moves a little each week (at most half a point) to keep about 15 days of spending in the treasury.`)}</div>
      <div class="budget"><label>Pensions ${info(`The elderly (8% of your people) get ${steps[n.pension]} a day, paid straight to them: no buildings, no staff. It's money back in your people's pockets.`)}<input type="range" min="0" max="${world.max_pension}" step="1" value="${n.pension}" id="pension" aria-label="Pensions"><span class="pct" id="pension-label">${n.pension}/${world.max_pension}</span></label></div>
      <h3>Construction ${info(`Automated, each month the state builds roads, railways and ports where traffic is heaviest, expands the most profitable firms, and builds public service levels where they fall short, as long as taxes are at most 15%. Funding them, and universities, stays yours (State and Research tabs).<br><br>All building, automated or not, is paid from a budget: up to 60% of the state's income a day, less as the treasury falls below its 15-day reserve and nothing at half of it, plus anything held beyond the reserve, spread over a month. Projects go slower when it is short.`)}</h3>
      <label class="check"><input type="checkbox" id="auto-build" ${n.auto_build ? "checked" : ""}> Automate construction</label>`;
  }
  return html;
}

// levels and funding of a public service in a tile, with − and + buttons
function levelCells(tile, svc, u, mine) {
  const btn = (label, path, title, off) => mine ? `<button data-cmd="/regions/${tile}/services/${svc}/${path}" title="${title}" ${off ? "disabled" : ""}>${label}</button>` : "";
  const building = u.building !== null ? `<br><span class="sub">building ${pct(u.building)}</span>` : "";
  return `<td>${btn("−", "close", "Close a level", u.levels <= 0)} ${u.levels} ${btn("+", "expand", `Build a level (${world.level_cost} construction)`, u.building !== null || u.levels >= u.max_levels)}${building}</td>
    <td>${btn("−", `fund?levels=${u.funded - 1}`, "Fund one level fewer", u.funded <= 0)} ${u.funded} ${btn("+", `fund?levels=${u.funded + 1}`, "Fund one more level", u.funded >= u.levels)}</td>`;
}

// ---- state panel: administration, schools and public health ---------------------
const SERVICE_TEXT = {
  administration: ["Officials collect the taxes. Without them only 30% of a tile's taxes come in (tax farmers and the gentry); with every official it needs, all of it. A tile needs more of them the more people it has and the further it is from the capital (your most populous tile).", "taxes collected", (rs) => pct(rs.collect)],
  schools: ["Teachers school the young. A tile's educated share follows the share of its children in school as they grow up and replace the unschooled, over about forty years: schooling every child from 1820 makes a tile over half educated by 1850. Educated people make its industry and services up to 15% more productive, and its universities up to two and a half times as productive.", "educated", (rs) => pct(rs.educated)],
  "public health": ["Doctors, clean water and sewers. A covered tile grows up to 0.6% a year faster. Crowded tiles (from 150 people) need many more of them, and crowding they don't cover slows growth by up to 2% a year and brings cholera. The growth column adds both up: when it is negative, the tile needs more public health.", "growth a year", (rs) => healthGrowth(rs)],
};
// what public health does to a tile's growth, less uncovered crowding: red
// when the tile needs more of it
const healthGrowth = (rs) => {
  const g = 100 * (rs.health_growth || 0);
  return `<span class="${g < -0.005 ? "bad" : g > 0.005 ? "good" : ""}">${g < -0.005 ? "−" : "+"}${Math.abs(g).toFixed(2)}%</span>`;
};
function statePanel() {
  const n = state.nations[viewNation], mine = n.id === state.you;
  const kinds = world.services.map((name, i) => [name, i]).filter(([name]) => name !== "universities");
  const [pick, row] = seg("service", kinds.map(([name]) => [name, cap(name)]), kinds[0][0]);
  const svc = world.services.indexOf(pick), [about, effectName, effect] = SERVICE_TEXT[pick] || ["", "", () => ""];
  const tiles = world.regions.filter((r) => state.regions[r.id].nation === n.id && state.regions[r.id].services);
  let lv = 0, fu = 0, staff = 0, need = 0, cost = 0;
  const rows = tiles.map((r) => {
    const rs = state.regions[r.id], u = rs.services[svc];
    lv += u.levels; fu += u.funded; staff += u.staff; need += u.need; cost += u.funded * u.cost;
    const short_ = u.need > 0 && u.coverage < 0.999;
    return `<tr><td>${esc(r.name)}${rs.city ? " · city" : ""}</td>${levelCells(r.id, svc, u, mine)}
      <td>${fmt(u.staff, 1)} / ${fmt(u.need, 1)}</td><td class="${short_ ? "bad" : ""}">${u.need > 0 ? pct(u.coverage) : "–"}</td><td>${effect(rs)}</td><td>£${fmt(u.funded * u.cost)}</td></tr>`;
  }).join("");
  const spent = n.spent_on.reduce((a, b) => a + b, 0);
  return nationPicker() + `<h2>The state</h2>
    <div class="stats">${stat("£" + fmt(n.income, 0), "income / day")}${stat("£" + fmt(spent, 0), "spent today")}${stat(`${Math.round(n.treasury / Math.max(n.spend_avg, 1e-9))} days`, "in the treasury")}</div>
    <h3>Public services ${info(`${about}<br><br>Each funded level employs ${world.staff} people at the tile's wage, paid from the treasury. Building a level costs ${world.level_cost} construction; unfunded levels stand idle and cost nothing.${mine && n.auto_build ? " Automated construction builds levels where they fall short; funding stays yours." : ""}`)}</h3>${row}
    <div class="table-scroll"><table class="unis"><tr><th>tile</th><th>levels</th><th>funded</th><th>staff / needed</th><th>covered</th><th>${effectName}</th><th>cost</th></tr>${rows}
    <tr class="total"><td>Total</td><td>${lv}</td><td>${fu}</td><td>${fmt(staff, 1)} / ${fmt(need, 1)}</td><td>${need > 0 ? pct(Math.min(1, staff / need)) : "–"}</td><td></td><td>£${fmt(cost, 1)}</td></tr></table></div>
    ${accountsBlock(n, mine)}`;
}

// ---- research panel ------------------------------------------------------------
function researchPanel() {
  const n = state.nations[viewNation], mine = n.id === state.you;
  const tiles = world.regions.filter((r) => state.regions[r.id].nation === n.id && state.regions[r.id].services);
  let lv = 0, fu = 0, staff = 0, made = 0, cost = 0;
  const rows = tiles.map((r) => {
    const u = state.regions[r.id].services[0];
    lv += u.levels; fu += u.funded; staff += u.staff; made += u.research; cost += u.funded * u.cost;
    return `<tr><td>${esc(r.name)}${state.regions[r.id].city ? " · city" : ""}</td>${levelCells(r.id, 0, u, mine)}
      <td>${fmt(u.per_level)}</td><td>£${fmt(u.cost)}</td><td>${fmt(u.research)}</td><td>£${fmt(u.funded * u.cost)}</td></tr>`;
  }).join("");
  const days = n.researching >= 0 && made > 0 ? Math.max(0, (n.tech_costs[n.researching] - n.research_points) / made) : null;
  let html = nationPicker() + `<h2>Research</h2>
    <div class="stats">${stat(fmt(made, 1), "research / day")}${stat("£" + fmt(cost, 0), "staff wages / day")}${stat(days === null ? "–" : `${short(days / 365 * 12)} mo`, "to the next tech")}</div>
    <h3>Universities ${info(`Universities are the state's. Each funded level employs ${world.staff} scholars at the tile's wage, paid from the treasury; what they find is the nation's research. A level yields more in a big city (its size bonus), with schooling, and with better science. Building a level costs ${world.level_cost} construction; unfunded levels stand idle and cost nothing.`)}</h3>
    <div class="table-scroll"><table class="unis"><tr><th>tile</th><th>levels</th><th>funded</th><th>per level</th><th>wages per level</th><th>research</th><th>cost</th></tr>${rows}
    <tr class="total"><td>Total</td><td>${lv}</td><td>${fu}</td><td></td><td></td><td>${fmt(made, 1)}</td><td>£${fmt(cost, 1)}</td></tr></table></div>`;
  if (mine) html += researchBlock(n);
  else html += `<h3>Researching</h3><p class="sub">${n.researching >= 0 ? `${world.techs[n.researching].name} (${pct(Math.min(1, n.research_points / n.tech_costs[n.researching]))})` : "nothing"}.</p>`;
  return html;
}

function researchBlock(n) {
  const current = n.researching >= 0
    ? `${world.techs[n.researching].name}: ${fmt(n.research_points, 0)} / ${fmt(n.tech_costs[n.researching], 0)}` : "nothing";
  const queue = n.queue.length
    ? n.queue.map((i, k) => `<span class="nation-pill">${k + 1}. ${world.techs[i].name} <button data-unqueue="${k}" title="Remove">×</button></span>`).join(" ")
    : `<span class="sub">empty: the cheapest available tech is picked automatically</span>`;
  const available = (t) => t.requires.every((r) => n.known[world.techs.findIndex((x) => x.name === r)]);
  const rows = (branch) => world.techs.map((t, i) => [t, i]).filter(([t]) => t.branch === branch).map(([t, i]) => {
    let action;
    if (n.known[i]) action = n.adoption && n.adoption[i] < 0.999 ? `✓<br><span class="sub" title="Its productivity and growth bonuses come in over five years">adopted ${pct(n.adoption[i])}</span>` : "✓";
    else if (n.researching === i) action = pct(Math.min(1, n.research_points / n.tech_costs[i]));
    else {
      const now = available(t) ? `<button data-cmd="/nations/${n.id}/research?tech=${i}">Now</button>` : "";
      const q = n.queue.includes(i) ? "" : `<button data-queue="${i}">Queue</button>`;
      action = `${now}${q}`;
    }
    const cost = n.tech_costs[i] < t.cost - 1 ? `${short(n.tech_costs[i])} <span class="sub">(−${pct(1 - n.tech_costs[i] / t.cost)})</span>` : short(t.cost);
    const needs = !n.known[i] && !available(t) ? `<br><span class="sub">needs ${t.requires.join(", ")}</span>` : "";
    return `<tr class="${n.known[i] ? "dim" : ""}"><td>${t.name}<br><span class="sub">${world.eras[t.era].split(" (")[0]}</span></td><td>${t.effect}${needs}</td><td>${cost}</td><td>${action}</td></tr>`;
  }).join("");
  const branches = world.branches.map((b) => {
    const all = world.techs.filter((t) => t.branch === b).length, known = world.techs.filter((t, i) => t.branch === b && n.known[i]).length;
    return `<tr class="branch"><th colspan="4">${b} · ${known}/${all}</th></tr>${rows(b)}`;
  }).join("");
  const prog = n.researching >= 0 ? Math.min(1, n.research_points / n.tech_costs[n.researching]) : 0;
  return `<h3>Research ${info(`Your universities' research goes to one tech at a time; points carry over. Techs your trade partners know are cheaper: ideas travel with trade. A new tech's productivity and growth bonuses come in over five years as it spreads through the works; army size and mobilisation techs over two; unlocks and other military effects are immediate.`)}</h3>
    <p class="sub">Researching ${current}.</p>
    <div class="progress"><span style="width:${pct(prog)}"></span></div>
    <div class="row">${queue}</div>
    <div class="table-scroll"><table>${branches}</table></div>`;
}

// ---- trade panel ---------------------------------------------------------------
function tradePanel() {
  const n = state.nations[viewNation], mine = n.id === state.you;
  const h = getHist(`nations/${n.id}`);
  const exp = n.exports.reduce((a, b) => a + b, 0), imp = n.imports.reduce((a, b) => a + b, 0);
  let html = nationPicker() + `<h2>Trade of ${esc(world.nations[n.id].name)} ${info(`Goods crossing its frontiers, valued at the prices where they were loaded. Nations at war, or allied against each other, don't trade; an embargo stops it too.`)}</h2>
    <div class="stats">
      ${stat("£" + short(exp), "exports / day", trend(h, "exports"))}
      ${stat("£" + short(imp), "imports / day", trend(h, "imports"))}
      ${stat(`<span class="${exp >= imp ? "good" : "bad"}">${exp >= imp ? "+" : "−"}£${short(Math.abs(exp - imp))}</span>`, "balance / day")}
    </div>`;
  // partners today, from the shipments on the map
  const partners = {};
  for (const f of state.flows) {
    const a = owner(f.from), b = owner(f.to);
    if (a === b || (a !== n.id && b !== n.id)) continue;
    const other = a === n.id ? b : a, p = partners[other] = partners[other] || { out: 0, in: 0 };
    p[a === n.id ? "out" : "in"] += flowValue(f);
  }
  const plist = Object.entries(partners).sort((x, y) => (y[1].out + y[1].in) - (x[1].out + x[1].in));
  html += `<h3>Partners today</h3>` + (plist.length
    ? `<table><tr><th>with</th><th>exports</th><th>imports</th></tr>${plist.map(([o, p]) => `<tr><td>${chip(+o)} ${esc(nationName(+o))}</td><td>£${fmt(p.out, 1)}</td><td>£${fmt(p.in, 1)}</td></tr>`).join("")}</table>`
    : `<p class="hint">No goods cross its frontiers today.</p>`);
  // by good
  const goods = world.goods.map((g, i) => i).filter((i) => n.exports[i] > 1e-6 || n.imports[i] > 1e-6)
    .sort((a, b) => (n.exports[b] + n.imports[b]) - (n.exports[a] + n.imports[a]));
  const max = Math.max(1e-9, ...goods.map((i) => Math.max(n.exports[i], n.imports[i])));
  const pick = goods.includes(+chartPick.tradeGood) ? +chartPick.tradeGood : null;
  html += `<h3>Over time</h3>`;
  if (!h || !h.data) html += lineChart(null, []);
  else if (pick === null) html += lineChart(h.ticks, [{ name: "exports", color: series(1), values: h.data.exports }, { name: "imports", color: series(2), values: h.data.imports }], { f: money });
  else html += `<div class="row" style="margin:0"><span class="sub">${esc(world.goods[pick].name)} only</span><button data-pick="tradeGood" data-val="">All goods</button></div>` +
    lineChart(h.ticks, [{ name: "exports", color: series(1), values: h.data.exports_by_good[pick] }, { name: "imports", color: series(2), values: h.data.imports_by_good[pick] }], { f: money });
  html += `<h3>By good, today</h3>` + (goods.length ? `<table><tr><th>good</th><th>exports</th><th></th><th>imports</th><th></th></tr>${goods.map((i) =>
    `<tr><td><button data-pick="tradeGood" data-val="${i}" title="Chart ${esc(world.goods[i].name)}" class="${pick === i ? "primary" : ""}">${esc(world.goods[i].name)}</button></td>
     <td>£${fmt(n.exports[i], 1)}</td><td><div class="hbar"><span style="width:${pct(n.exports[i] / max)};background:${series(1)}"></span></div></td>
     <td>£${fmt(n.imports[i], 1)}</td><td><div class="hbar"><span style="width:${pct(n.imports[i] / max)};background:${series(2)}"></span></div></td></tr>`).join("")}</table>
     <p class="sub">Tap a good to chart it.</p>` : `<p class="hint">Nothing today.</p>`);
  if (mine) {
    const tradable = world.goods.map((g, i) => i).filter((i) => !["transport", "construction", "research"].includes(world.goods[i].name));
    const tariff = Math.max(...tradable.map((i) => n.tariff[i]));
    const others = world.nations.filter((o) => o.id !== n.id).map((o) =>
      `<label class="check"><input type="checkbox" data-embargo="${o.id}" ${n.embargo[o.id] ? "checked" : ""}> ${esc(o.name)}</label>`).join("");
    html += `<h3>Customs ${info(`An import tariff is paid on goods entering your nation, at the price they sell for, into the treasury. An embargo stops all trade with that nation, both ways. Trade also spreads knowledge: techs your partners know cost you less.`)}</h3>
      <div class="row"><label>Import tariff <input id="tariff" type="number" min="0" max="500" step="5" value="${Math.round(100 * tariff)}" class="num">%</label><button data-input="tariff">Set</button></div>
      <div class="row"><span class="sub">Embargo:</span> ${others}</div>
      <p class="sub">Tariffs earned £${fmt(n.tariff_revenue, 1)} today.</p>`;
  }
  return html;
}

// ---- armies panel --------------------------------------------------------------
function armiesPanel() {
  const n = me();
  const mine = n ? state.armies.filter((a) => a.nation === n.id) : [];
  const others = state.armies.filter((a) => !n || a.nation !== n.id);
  let html = "";
  if (n) {
    html += `<h2>Your armies</h2>
      ${(() => {
        let goods = 0, transport = 0, pay = 0;
        for (const a of mine) { for (const [g, v] of Object.entries(a.cost || {})) g === "transport" ? (transport += v) : (goods += v); pay += a.pay; }
        return `<div class="stats">${stat("£" + fmt(goods, 0), "supplies / day")}${stat("£" + fmt(transport, 0), "transport / day")}${stat("£" + fmt(pay, 0), "pay / day")}</div>`;
      })()}
      <p class="sub">${short(n.soldiers)} soldiers of ${short(n.mobilize_cap)} allowed (${pct(n.mobilization)} of your people), up to ${short(n.max_army_size)} per army.
      Armies cost £${fmt(n.spent_on[world.spending.indexOf("military")], 0)} today of £${fmt(n.income, 0)} income.
      Best doctrine: ${world.doctrines[n.best_doctrine].name}. Raise armies from a tile of yours.</p>
      <div class="budget"><label>Mobilization ${info(`In peace each army keeps this share of its full strength under arms; the rest are reservists, at home working as ordinary people. At war they are called up from across your tiles: an army fills by a ninetieth of its full strength a day, so mobilising from 10% takes about 80 days, plus the march to the army. High mobilisation is ready but costs pay and supplies; low is cheap but leaves you weak for months.`)}<input type="range" min="10" max="100" step="5" value="${Math.round(100 * n.standing)}" id="standing" aria-label="Mobilization in peace"><span class="pct" id="standing-pct">${pct(n.standing)}</span></label></div>
      ${n.at_war.some(Boolean) ? `<p class="sub"><b>You are at war now: every reservist is called up.</b></p>` : ""}`;
    html += mine.length ? mine.map((a) => armyCard(a, true)).join("") : `<p class="hint">You have no armies.</p>`;
  }
  html += `<h3>Other armies</h3>` + (others.length ? others.map((a) => armyCard(a, false)).join("") : `<p class="hint">None.</p>`);
  return html;
}

function armyCard(a, mine) {
  const d = world.doctrines[a.doctrine];
  const where = world.regions[a.region];
  const dest = a.path.length ? ` → marching to ${world.regions[a.path[a.path.length - 1]].name}` : "";
  const open = a.id === selectedArmy;
  let html = `<div class="army-card ${open ? "selected" : ""}" data-select-army="${a.id}">
    <div class="title"><b>${chip(a.nation)} ${esc(nationName(a.nation))} army ${a.id}</b><span><b>${short(a.strength)}</b> <span class="sub">strength</span></span></div>
    <div class="progress" title="strength against full size and supply"><span style="width:${pct(Math.min(1, a.strength / Math.max(a.full_strength, 1e-9)))};background:${a.strength > 0.6 * a.full_strength ? cssVar("--good") : a.strength > 0.3 * a.full_strength ? cssVar("--brass") : cssVar("--danger")}"></span></div>
    <p class="sub">${pct(Math.min(1, a.strength / Math.max(a.full_strength, 1e-9)))} of full strength ${short(a.full_strength)} · ${short(a.men)} / ${short(a.under_arms)} men${a.under_arms < a.target - 0.05 ? ` (${short(a.target)} at war; the rest are reservists)` : ""} · ${d.name} · at ${where.name}${dest}
      ${a.coming > 0.05 ? ` · ${short(a.coming)} recruits marching out, the first in ${a.coming_days} days` : ""}
      ${a.retreating ? ` · <span class="bad">falling back after a lost battle: no orders until home</span>` : ""}
      ${a.retreating || a.can_occupy ? "" : ` · <span class="bad">too weak to take ground: it needs a strength of ${world.occupy_strength}</span>`}</p>`;
  if (!open) return html + `</div>`;
  const supply = Object.keys(d.needs).map((g) => {
    const v = a.supply[g] ?? 0;
    const color = v > 0.8 ? cssVar("--good") : v > 0.4 ? cssVar("--brass") : cssVar("--danger");
    const days = a.stock_days[g] ?? 0, from = (a.source_shares || {})[g] || [], none = from.length > 0 && !(a.offered[g] > 0);
    const where = from.length === 1 ? `bought at ${world.regions[from[0].tile].name}`
      : `bought across ${from.length} tiles: ${from.slice(0, 3).map((s) => `${world.regions[s.tile].name} ${pct(s.share)}`).join(", ")}${from.length > 3 ? `, +${from.length - 3} more` : ""}`;
    return `<span>${g}</span><span class="meter" title="${Math.round(days)} days in stock"><span style="width:${pct(v)};background:${color}"></span></span><span>${pct(v)}</span>
      <span></span><span class="sub" style="grid-column:2/4">${days >= 1 ? `${Math.round(days)} days in stock` : `<span class="bad">no stock left</span>`}${none ? ` · <span class="bad">none of your tiles sells any yet: ordered from ${from.length} that could make it</span>` : from.length ? ` · ${where}` : ""}</span>`;
  }).join("");
  // strength: the doctrine's base power times each factor
  const mult = (v) => `<span class="${v > 1.0001 ? "good" : v < 0.9999 ? "bad" : ""}">×${v.toFixed(2)}</span>`;
  let per = 1;
  const att = (a.attack || []).map((m, i) => {
    per *= m.value;
    return `<tr><td>${esc(m.name)}<br><span class="sub">${esc(m.note)}</span></td><td>${i === 0 ? m.value.toFixed(2) : mult(m.value)}</td></tr>`;
  }).join("");
  let home = 1;
  const def = (a.defense || []).map((m) => { home *= m.value; return `<tr><td>${esc(m.name)}<br><span class="sub">${esc(m.note)}</span></td><td>${mult(m.value)}</td></tr>`; }).join("");
  const attr = (a.attrition || []).map((m) => `<tr><td>${esc(m.name)}<br><span class="sub">${esc(m.note)}</span></td>
    <td>${m.add ? `${(100 * m.value).toFixed(2)}%` : mult(m.value)}</td></tr>`).join("");
  const depot = a.depot >= 0 ? `${fmt(a.transport, 1)} transport today` : `<span class="bad">Cut off: no tile of yours can reach it</span>`;
  // what the army costs a day: each good, the transport to bring it, and pay
  const goods = Object.entries(a.cost || {}).filter(([g]) => g !== "transport");
  const goodsTotal = goods.reduce((t, [, v]) => t + v, 0), transport = (a.cost || {}).transport || 0;
  html += `<h3>Cost a day ${info(`Goods (with the transport to bring them) and soldiers' pay, at the nation's average wage. Goods include restocking towards 30 days of needs, so a depleted army costs more while it refills. Unpaid soldiers desert.`)}</h3><table class="mods">
    ${goods.map(([g, v]) => `<tr><td>${esc(g)}</td><td>£${fmt(v, 2)}</td></tr>`).join("")}
    <tr><td>Transport<br><span class="sub">carrying the goods to the army</span></td><td>£${fmt(transport, 2)}</td></tr>
    <tr><td>Soldiers' pay<br><span class="sub">${short(a.men)} men at the nation's average wage</span></td><td>£${fmt(a.pay, 2)}</td></tr>
    <tr class="total"><td>Total</td><td>£${fmt(goodsTotal + transport + a.pay, 2)}</td></tr></table>`;
  html += `<h3>Supplies ${info(`Each good is ordered from every tile of yours that sells it, in proportion to what each offers (among those where it lands no more than half again dearer than the cheapest), so all your works see the demand; it is shipped here, needing transport. The army keeps up to 30 days of needs in stock and fights from its stock: it is short only once the stock runs out. Only the men under arms eat and use kit; reservists live at home.<br><br>The state pays whatever a good costs, so a shortage drives its price up and calls up supply, until it costs ten times its usual price; beyond that the state spends no more on it.`)}</h3><div class="supply">${supply}</div><p class="sub" style="margin-top:4px">${depot}</p>
    <h3>Strength in battle ${info(`Doctrines form a ladder. The levies' power, cut by short food, clothing and arms, is the first rung; each doctrine above multiplies it by its power over the one below, as far as the kit it adds is supplied, and never further than the rung beneath it (tanks are worth nothing without rifles). An army that loses its newest kit falls back down the ladder.<br><br>In battle each side loses men in proportion to the other's strength, and three times that share of its stock (ammunition spent, guns abandoned), so long fights drain supplies fast. An army fights on until its strength is down to a third of what it began the battle with, then falls back to its nearest own tile, losing half its men and half its stock on the way; surrounded, it fights on to the end. Defenders on their own soil fight harder, and each day fortune adds or takes up to 10% from each side.`)}</h3>
    <table class="mods"><tr><th>factor</th><th>effect</th></tr>${att}
      <tr class="total"><td>Per soldier</td><td>${per.toFixed(2)}</td></tr>
      <tr><td>× ${short(a.men)} men</td><td>${short(a.strength)}</td></tr></table>
    <p class="sub" style="margin-top:8px">When defending its own nation's soil:</p>
    <table class="mods">${def}<tr class="total"><td>Defending</td><td>×${home.toFixed(2)}</td></tr></table>
    <p class="sub" style="margin-top:8px">Daily losses outside battle:</p>
    <table class="mods">${attr}<tr class="total"><td>Men lost a day</td><td>${(100 * a.daily_loss).toFixed(2)}% ≈ ${fmt(a.men * a.daily_loss, 2)}</td></tr></table>`;
  const h = getHist(`armies/${a.id}`);
  const [k, row] = seg("army", [["strength", "Strength"], ["men", "Men"], ["effectiveness", "Per soldier"]], "strength");
  html += `<h3>Its campaign</h3>${row}` + (h && h.data ? lineChart(h.ticks, [{ name: { men: "men", strength: "strength", effectiveness: "strength per soldier" }[k], color: nationColor(a.nation), values: h.data[k] }], { f: k === "effectiveness" ? (v) => fmt(v) : short }) : lineChart(null, []));
  if (mine) {
    const n = me();
    html += `<div class="row">
      ${a.retreating ? "" : `<button class="primary" data-move="${a.id}">March…</button>`}
      <label>Size <input type="number" min="0" max="${Math.floor(n.max_army_size)}" step="1" value="${Math.round(a.target)}" class="num" data-target-for="${a.id}"></label><button data-set-target="${a.id}">Set</button>
      ${state.regions[a.region].nation === a.nation ? `<button class="danger" data-cmd="/armies/${a.id}/disband">Disband</button>` : ""}
    </div>`;
  }
  return html + `</div>`;
}

// ---- world panel ---------------------------------------------------------------
function worldPanel() {
  const n = me();
  const rows = state.nations.map((o) => {
    const info = world.nations[o.id];
    const who = o.id === state.you ? "you" : o.controller === "ai" ? `computer · ${esc(o.temperament)}` : "player";
    let diplo = "";
    if (n && o.id !== n.id && !o.eliminated && !n.eliminated) {
      if (n.at_war[o.id]) {
        diplo = n.peace_offer[o.id] ? `<span class="sub">peace offered</span>`
          : `<button data-cmd="/nations/${n.id}/peace?target=${o.id}">${o.peace_offer[n.id] ? "Accept peace" : "Offer peace"}</button>`;
      } else if (n.allied_against >= 0 && n.allied_against === o.allied_against) {
        diplo = `<span class="sub">your ally</span>`;
      } else {
        diplo = `<button class="danger" data-confirm="Declare war on ${esc(info.name)}?" data-cmd="/nations/${n.id}/war?target=${o.id}">Declare war</button>`;
      }
    }
    const war = o.at_war.map((w, i) => (w ? world.nations[i].name : null)).filter(Boolean);
    const allies = o.allied_against >= 0 ? state.nations.filter((x) => x.id !== o.id && x.allied_against === o.allied_against).map((x) => world.nations[x.id].name) : [];
    const alliance = o.allied_against >= 0 ? `<br><span class="sub">allied with ${allies.map(esc).join(", ")} against ${esc(world.nations[o.allied_against].name)}</span>` : "";
    return `<tr><td>${chip(o.id)} ${esc(info.name)}<br><span class="sub">${who}</span></td><td>${o.eliminated ? "fallen" : o.tiles}</td>
      <td>${short(o.population)}</td><td>£${short(o.gdp)}</td><td class="nowrap">out £${short(o.exports.reduce((t, v) => t + v, 0))}<br><span class="sub">in £${short(o.imports.reduce((t, v) => t + v, 0))}</span></td><td>${short(o.soldiers)}</td><td>${o.known.filter(Boolean).length}</td>
      <td>${war.length ? `<span class="bad">⚔ ${war.map(esc).join(", ")}</span>` : "peace"}${alliance}${diplo ? `<br>${diplo}` : ""}</td></tr>`;
  }).join("");
  const hs = world.nations.map((o) => getHist(`nations/${o.id}`));
  const [k, row] = seg("world", [["population", "People"], ["gdp", "GDP"], ["utility", "Living"], ["soldiers", "Soldiers"], ["strength", "Might"], ["techs", "Techs"], ["tiles", "Tiles"]], "gdp");
  const ready = hs.every((h) => h && h.data);
  const chart = ready ? lineChart(hs[0].ticks, world.nations.map((o, i) => ({ name: o.name, color: nationColor(o.id), values: hs[i].data[k] })), { f: k === "utility" ? (v) => fmt(v) : k === "gdp" ? money : short }) : lineChart(null, []);
  return `<h2>The powers ${info(`<b>War</b>: you can only declare war on a neighbour. Peace needs both sides: a computer nation accepts after a year if it isn't clearly winning, or after three years regardless; after peace a truce holds for some years.<br><br><b>Balance of power</b>: when one nation's might (over a year) is half again every other's, the rest ally against it, for at least three years and until it falls below 1.2 times the next. Allies embargo it, can't fight each other, join each other's wars against it the same day, and make peace with it together.`)}</h2><div class="table-scroll"><table>
    <tr><th>nation</th><th>tiles</th><th>people</th><th>real GDP<br>/ day</th><th>trade / day</th><th>soldiers</th><th>techs</th><th>relations</th></tr>${rows}</table></div>
    <h3>The race</h3>${row}${chart}
    ${k === "gdp" ? `<p class="sub">Real GDP, in pounds of January 1820 ${info(GDP_NOTE)}</p>` : ""}
    <p class="sub" style="margin-top:8px">Money in the world: ${short(state.money)} ${info(`The world's money is fixed: none is made or destroyed, it only changes hands. As the economy grows, prices fall instead.`)}</p>`;
}

// ---- news panel ----------------------------------------------------------------
function newsPanel() {
  const items = [...state.events].reverse().map((e) =>
    `<li class="${e.kind}"><span class="when">${dateOf(e.tick)}</span>${esc(e.text)}</li>`).join("");
  return `<h3>The Gazette</h3>
    <label class="check"><input type="checkbox" id="autopause" ${pref("autopause", true) ? "checked" : ""}> Pause when war, battle or occupation news concerns me</label>
    <div class="news"><ul>${items || `<li class="sub">Nothing yet.</li>`}</ul></div>`;
}

// ---- research prompt -----------------------------------------------------------
// When one of your techs is finished and nothing is queued, the game pauses
// (if you hold the clock) and asks what to study next. The engine has picked
// one provisionally, so no research is lost while you decide.
let prompted = false;      // the dialog is open
let resumeAfter = false;   // we paused the game to ask, so play on after
function researchPrompt() {
  const n = me(), dlg = $("dialog");
  if (!n || n.eliminated || !n.choose_research) {
    if (prompted) { dlg.hidden = true; prompted = false; }
    return;
  }
  if (prompted) return;
  prompted = true;
  if (state.running && canClock()) { resumeAfter = true; command("/clock?running=false"); }
  const done = [...state.events].reverse().find((e) => e.kind === "tech" && e.nation === n.id);
  const available = world.techs.map((t, i) => [t, i]).filter(([t, i]) => !n.known[i] && t.requires.every((r) => n.known[world.techs.findIndex((x) => x.name === r)]));
  const rows = world.branches.map((b) => {
    const list = available.filter(([t]) => t.branch === b).sort((x, y) => n.tech_costs[x[1]] - n.tech_costs[y[1]]);
    if (!list.length) return "";
    return `<h3>${cap(b)}</h3>` + list.map(([t, i]) => {
      const cut = n.tech_costs[i] < t.cost - 1 ? ` <span class="good">(−${pct(1 - n.tech_costs[i] / t.cost)} from trade)</span>` : "";
      return `<div class="tech-choice ${i === n.researching ? "now" : ""}"><b>${esc(t.name)}${i === n.researching ? ` <span class="sub">· picked for now</span>` : ""}</b>
        <button class="${i === n.researching ? "" : "primary"}" data-study="${i}">Study</button>
        <span class="sub">${esc(t.effect)} · ${short(n.tech_costs[i])} points${cut} · ${world.eras[t.era].split(" (")[0]}</span></div>`;
    }).join("");
  }).join("");
  dlg.querySelector(".dialog-card").innerHTML = `<header><p class="sub">${done ? esc(dateOf(done.tick)) : ""}</p>
      <h2 id="dialog-title">${done ? esc(done.text.replace(world.nations[n.id].name + " discovers ", "Your scholars discover ")) : "A discovery"}</h2>
      <p class="sub">What should they study next? Points carry over, so nothing is lost while you decide.</p></header>
    <div class="body">${rows || `<p class="hint">Every tech is known.</p>`}</div>
    <footer><button data-plan>Plan a queue…</button><button class="primary" data-study="${n.researching}">Keep ${esc(world.techs[n.researching].name)}</button></footer>`;
  dlg.hidden = false;
  const first = dlg.querySelector("button.primary");
  if (first) first.focus();
}
async function answerPrompt(tech, plan) {
  const ok = await command(`/nations/${state.you}/research?tech=${tech}`);
  if (!ok) return;
  $("dialog").hidden = true; prompted = false;
  if (plan) { setTab("nation"); render(true); }
  else if (resumeAfter && canClock() && !state.running) command("/clock?running=true");
  resumeAfter = false;
}
$("dialog").addEventListener("click", (ev) => {
  const b = ev.target.closest("button");
  if (b && b.dataset.study !== undefined) answerPrompt(+b.dataset.study, false);
  if (b && b.dataset.plan !== undefined) answerPrompt(me().researching, true);
});
document.addEventListener("keydown", (ev) => {
  if (ev.key === "Escape" && prompted && me()) answerPrompt(me().researching, false);
});

// ---- panel actions -------------------------------------------------------------
side.addEventListener("click", (ev) => {
  const b = ev.target.closest("button");
  if (!b) {
    const card = ev.target.closest("[data-select-army]");
    if (card && +card.dataset.selectArmy !== selectedArmy) { selectedArmy = +card.dataset.selectArmy; render(true); }
    return;
  }
  if (b.dataset.pick) { chartPick[b.dataset.pick] = b.dataset.val; renderPanel(true); return; }
  if (b.dataset.viewNation !== undefined) { viewNation = +b.dataset.viewNation; renderPanel(true); return; }
  if (b.dataset.confirm && !confirm(b.dataset.confirm)) return;
  if (b.dataset.cmd) command(b.dataset.cmd);
  if (b.dataset.army) { selectedArmy = +b.dataset.army; setTab("armies"); render(true); }
  if (b.dataset.move) { selectedArmy = +b.dataset.move; moving = true; render(true); if (window.innerWidth <= 860) canvas.scrollIntoView({ behavior: "smooth" }); }
  if (b.dataset.setTarget) command(`/armies/${b.dataset.setTarget}/target?men=${+document.querySelector(`[data-target-for="${b.dataset.setTarget}"]`).value}`);
  if (b.id === "raise") command(`/nations/${state.you}/armies?region=${selected}&target=${+$("raise-men").value}`);
  if (b.dataset.queue !== undefined) command(`/nations/${state.you}/queue?techs=${[...me().queue, +b.dataset.queue].join(",")}`);
  if (b.dataset.unqueue !== undefined) {
    const q = [...me().queue]; q.splice(+b.dataset.unqueue, 1);
    command(`/nations/${state.you}/queue?techs=${q.join(",")}`);
  }

  if (b.dataset.input === "tax") command(`/nations/${state.you}/tax?rate=${(+$("tax").value / 100).toFixed(3)}`);
  if (b.dataset.input === "tariff") command(`/nations/${state.you}/tariff?rate=${(+$("tariff").value / 100).toFixed(3)}`);
});
side.addEventListener("input", (ev) => {
  if (ev.target.id === "standing") $("standing-pct").textContent = `${ev.target.value}%`;
  if (ev.target.id === "pension") $("pension-label").textContent = `${ev.target.value}/${world.max_pension}`;
});
side.addEventListener("change", (ev) => {
  if (ev.target.dataset.pickSelect) { chartPick[ev.target.dataset.pickSelect] = ev.target.value; ev.target.blur(); renderPanel(true); return; }
  if (ev.target.id === "standing") command(`/nations/${state.you}/standing?share=${+ev.target.value / 100}`);
  if (ev.target.id === "auto-build") command(`/nations/${state.you}/auto?on=${ev.target.checked}`);
  if (ev.target.id === "auto-tax") command(`/nations/${state.you}/auto-tax?on=${ev.target.checked}`);
  if (ev.target.id === "pension") command(`/nations/${state.you}/pension?level=${ev.target.value}`);
  const t = ev.target.dataset.embargo;
  if (t !== undefined) command(`/nations/${state.you}/embargo?target=${t}&on=${ev.target.checked}`);
  if (ev.target.id === "autopause") setPref("autopause", ev.target.checked);
});

applyTheme();
if (document.fonts) document.fonts.ready.then(() => { terrainCache = null; if (state) draw(); });
route();

// ---- info popovers: one note at a time, opened by an "i" button ---------------
const infoPop = document.createElement("div");
infoPop.className = "info-pop";
infoPop.setAttribute("role", "note");
infoPop.hidden = true;
document.body.appendChild(infoPop);
let infoFor = null; // the tip shown (its text: buttons are re-rendered under it)
function closeInfo() { infoPop.hidden = true; infoFor = null; }
document.addEventListener("click", (ev) => {
  const b = ev.target.closest && ev.target.closest(".info-btn");
  if (!b) {
    if (!infoPop.hidden && !infoPop.contains(ev.target)) closeInfo();
    return;
  }
  ev.preventDefault();
  ev.stopPropagation(); // not a click on the card or row it sits in
  if (infoFor === b.dataset.tip && !infoPop.hidden) { closeInfo(); return; }
  infoFor = b.dataset.tip;
  infoPop.innerHTML = b.dataset.tip;
  infoPop.hidden = false;
  const r = b.getBoundingClientRect(), w = infoPop.offsetWidth, h = infoPop.offsetHeight;
  const left = Math.max(8, Math.min(window.innerWidth - w - 8, r.left + r.width / 2 - w / 2));
  const below = r.bottom + 6 + h <= window.innerHeight - 8;
  infoPop.style.left = `${left}px`;
  infoPop.style.top = `${below ? r.bottom + 6 : Math.max(8, r.top - h - 6)}px`;
}, true);
document.addEventListener("keydown", (ev) => { if (ev.key === "Escape") closeInfo(); });
window.addEventListener("scroll", closeInfo, true);
window.addEventListener("resize", closeInfo);
