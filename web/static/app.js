"use strict";

// ---- basics --------------------------------------------------------------------
const $ = (id) => document.getElementById(id);
const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const fmt = (x, d = 2) => (Math.abs(x) >= 1000 ? Math.round(x).toLocaleString() : x.toFixed(d));
const pct = (x) => `${Math.round(100 * x)}%`;

let gid = null;          // current game
let world = null;        // its static description
let state = null;        // its latest state
let detail = null;       // selected tile's details
let selected = null;     // selected tile id
let selectedArmy = null; // selected army id
let moving = false;      // next map tap orders the selected army to march there
let tab = "tile";
let pollTimer = null;
let lastEventTick = -1;  // newest event seen, for auto-pause

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
  $("game").hidden = true;
  $("lobby").hidden = false;
  document.title = "Mercury";
  try { await refreshLobby(); } catch (e) { showError("lobby-err", e); }
}

async function refreshLobby() {
  const [games, saves] = await Promise.all([api("GET", "/api/games"), api("GET", "/api/saves")]);
  $("games").innerHTML = games.length ? games.map(gameCard).join("") : `<p class="hint">No games yet. Create one below.</p>`;
  $("saves").innerHTML = saves.length
    ? `<div class="table-scroll"><table><tr><th>slot</th><th>game</th><th>date</th><th>saved</th><th></th></tr>${saves.map((s) =>
      `<tr><td>${esc(s.slot)}</td><td>${esc(s.name)}</td><td>${s.tick}</td><td>${new Date(s.saved_at).toLocaleString()}</td>
       <td><button data-load="${esc(s.slot)}">Load</button></td></tr>`).join("")}</table></div>`
    : `<p class="hint">No saves yet. Save a game from its top bar.</p>`;
}

const paceName = (d) => ({ 0: "real time", 7: "weekly turns", 30: "monthly turns", 90: "seasonal turns", 365: "yearly turns" }[d] || `${d}-day turns`);

function gameCard(g) {
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
    <div class="title"><b>${esc(g.name)}</b><span class="sub">${esc(g.date)} · ${paceName(g.turn_days)}${g.running ? " · running" : ""}</span></div>
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
    const q = new URLSearchParams({ name: $("new-name").value.trim(), cost: $("new-cost").value, turn: $("new-turn").value });
    const g = await api("POST", `/api/games?${q}`);
    location.hash = `#/game/${g.id}`;
  } catch (e) { showError("lobby-err", e); }
});

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
  gid = id;
  selected = null; detail = null; selectedArmy = null; moving = false;
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
  lastEventTick = state.events.length ? state.events[state.events.length - 1].tick : -1;
  setTab(state.you >= 0 ? "nation" : "world");
  resize();
  render(true);
  startPolling();
}

function startPolling() {
  stopPolling();
  const tickOnce = async () => {
    try {
      state = await api("GET", `/api/games/${gid}/state`);
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
const neighbours = (id) => world.links.filter((l) => l.a === id || l.b === id).map((l) => (l.a === id ? l.b : l.a));
const armyById = (id) => state.armies.find((a) => a.id === id);
const nationName = (id) => (id >= 0 ? world.nations[id].name : "unowned");
const chip = (id) => `<span class="chip" style="background:${id >= 0 ? world.nations[id].color : UNOWNED}"></span>`;
// the game's calendar has 365-day years (29 February is skipped), so any
// common year maps its days to dates
const dateOf = (tick) => {
  const y = world.start_year + Math.floor(tick / 365);
  const dt = new Date(Date.UTC(2001, 0, 1 + (tick % 365)));
  return `${dt.getUTCDate()} ${dt.toLocaleString("en", { month: "short", timeZone: "UTC" })} ${y}`;
};

async function command(path, after) {
  try {
    state = await api("POST", `/api/games/${gid}${path}`);
    if (selected !== null) detail = await api("GET", `/api/games/${gid}/regions/${selected}`);
    if (after) after();
    render(true);
    return true;
  } catch (e) { showError("game-err", e); return false; }
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
    <span>tiles <b>${n.tiles}</b></span><span>people <b>${fmt(n.population, 0)}</b></span>
    <span>treasury <b>${fmt(n.treasury, 0)}</b></span><span>income <b>${fmt(n.income, 0)}</b>/day</span>
    <span>soldiers <b>${fmt(n.soldiers, 0)}</b> / ${fmt(n.mobilize_cap, 0)}</span>
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
const TABS = ["tile", "nation", "armies", "world", "news"];
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
  render(true);
});

// is the user typing or dragging in this element? then don't redraw it
const busy = (el) => el.contains(document.activeElement) && /INPUT|SELECT/.test(document.activeElement.tagName);

function render(force) {
  if (!state || !world) return;
  renderBar();
  draw();
  const m = $("mode");
  const a = selectedArmy !== null && armyById(selectedArmy);
  m.hidden = !(moving && a);
  if (moving && a) m.innerHTML = `<span>Tap a tile to march army ${a.id} there</span><button id="cancel-move">Cancel</button>`;
  const el = $(`tab-${tab}`);
  if (!force && busy(el)) return;
  const panels = { tile: tilePanel, nation: nationPanel, armies: armiesPanel, world: worldPanel, news: newsPanel };
  el.innerHTML = panels[tab]();
}

// ---- map -----------------------------------------------------------------------
const canvas = $("map");
const ctx = canvas.getContext("2d");
const TERRAIN_COLORS = { plains: "#c8d67a", forest: "#4f8a4a", hills: "#b89c69", mountains: "#8e8e93", tropical: "#2fa37b", coast: "#6db3dc" };
const TIER_COLORS = { basic: "#7a9e4f", intermediate: "#d4a23c", luxury: "#9b5fc0" };
const INFRA_COLORS = ["#d9d4c7", "#b07d48", "#5b3a1e", "#2b2b2b"];
const UNOWNED = "#a3a3a3";
const RAMP = ["#2a1b5c", "#26508f", "#1f8a8c", "#49b670", "#b5d84a", "#fde725"];
const hexRGB = (c) => [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16));
function rampColor(t) {
  t = Math.max(0, Math.min(1, t));
  const x = t * (RAMP.length - 1), i = Math.min(Math.floor(x), RAMP.length - 2), f = x - i;
  const a = hexRGB(RAMP[i]), b = hexRGB(RAMP[i + 1]);
  return `rgb(${a.map((v, k) => Math.round(v + (b[k] - v) * f)).join(",")})`;
}

let size = 40;
const margin = 8;
function center(r) {
  const w = Math.sqrt(3) * size;
  return [margin + w / 2 + w * (r.col + 0.5 * (r.row & 1)), margin + size + 1.5 * size * r.row];
}
function hexPath(x, y, s) {
  ctx.beginPath();
  for (let i = 0; i < 6; i++) {
    const a = Math.PI / 180 * (60 * i - 30);
    i ? ctx.lineTo(x + s * Math.cos(a), y + s * Math.sin(a)) : ctx.moveTo(x + s * Math.cos(a), y + s * Math.sin(a));
  }
  ctx.closePath();
}
function resize() {
  if (!world || $("game").hidden) return;
  const wrap = canvas.parentElement;
  const narrow = window.innerWidth <= 760;
  const availH = narrow ? Infinity : Math.max(320, window.innerHeight - wrap.getBoundingClientRect().top - 60);
  size = Math.min((wrap.clientWidth - 2 * margin) / (Math.sqrt(3) * (world.cols + 0.5)), (availH - 2 * margin) / (2 + 1.5 * (world.rows - 1)));
  const w = 2 * margin + Math.sqrt(3) * size * (world.cols + 0.5), h = 2 * margin + size * (2 + 1.5 * (world.rows - 1));
  const dpr = window.devicePixelRatio || 1;
  canvas.width = w * dpr; canvas.height = h * dpr;
  canvas.style.width = w + "px"; canvas.style.height = h + "px";
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  draw();
}
window.addEventListener("resize", resize);

const CATEGORICAL = ["terrain", "nation", "infra"];
const isShare = () => ["basic", "luxury"].includes($("overlay").value);
const isPct = () => isShare() || $("overlay").value === "growth";
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
const fmtOverlay = (v) => ($("overlay").value === "growth" ? `${(100 * v).toFixed(1)}%` : isShare() ? pct(v) : fmt(v, 1));

function draw() {
  if (!world || !state) return;
  const css = getComputedStyle(document.documentElement);
  ctx.clearRect(0, 0, canvas.width, canvas.height);
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
    hexPath(x, y, size * 0.97);
    if (sc) ctx.fillStyle = rampColor(sc.t(overlayValue(r.id)));
    else if (kind === "nation") ctx.fillStyle = rs.nation >= 0 ? world.nations[rs.nation].color : UNOWNED;
    else if (kind === "infra") ctx.fillStyle = INFRA_COLORS[rs.infra];
    else ctx.fillStyle = TERRAIN_COLORS[r.terrain];
    ctx.fill();
    ctx.lineWidth = 1; ctx.strokeStyle = css.getPropertyValue("--hex-stroke"); ctx.stroke();
    if (kind !== "terrain") { // terrain pip
      ctx.beginPath(); ctx.arc(x, y - size * 0.62, size * 0.09, 0, 2 * Math.PI);
      ctx.fillStyle = TERRAIN_COLORS[r.terrain]; ctx.fill(); ctx.strokeStyle = "rgba(0,0,0,0.4)"; ctx.stroke();
    }
    let label, dark;
    if (sc) { const v = overlayValue(r.id); label = fmtOverlay(v); dark = sc.t(v) <= 0.6; }
    else if (kind === "nation") { label = rs.nation >= 0 ? world.nations[rs.nation].name.slice(0, 3) : "–"; dark = rs.nation >= 0; }
    else if (kind === "infra") { label = "×" + rs.move_cost.toFixed(2); dark = rs.infra >= 2; }
    else { label = r.terrain[0].toUpperCase(); dark = false; }
    ctx.fillStyle = dark ? "#fff" : "#1b1b1b";
    ctx.font = `600 ${Math.max(10, size * 0.26)}px system-ui`;
    ctx.textAlign = "center"; ctx.textBaseline = "middle";
    ctx.fillText(rs.city ? `★ ${label}` : label, x, y - size * 0.22);
    for (let i = 0; i < rs.infra; i++) { // one bar per road level
      ctx.fillStyle = INFRA_COLORS[2]; ctx.strokeStyle = "#fff"; ctx.lineWidth = 1;
      const bx = x - size * 0.37 + i * size * 0.27, by = y + size * 0.62;
      ctx.fillRect(bx, by, size * 0.22, size * 0.07); ctx.strokeRect(bx, by, size * 0.22, size * 0.07);
    }
    if (rs.project) progressRing(x + size * 0.5, y - size * 0.38, rs.project.delivered / rs.project.required, "#e8590c");
    if (rs.port) outlinedText("⚓", x - size * 0.52, y - size * 0.38, Math.max(11, size * 0.26));
    if (rs.occupation) {
      const c = world.nations[rs.occupation.nation].color;
      ctx.setLineDash([6, 4]); hexPath(x, y, size * 0.82);
      ctx.lineWidth = 3; ctx.strokeStyle = c; ctx.stroke(); ctx.setLineDash([]);
      outlinedText(`⚑ ${pct(rs.occupation.progress)}`, x, y + size * 0.5, Math.max(10, size * 0.2));
    }
  }
  if ($("show-flows").checked) drawFlows(css);
  drawBorders();
  drawPaths();
  drawArmies();
  if (selected !== null) {
    const [x, y] = center(world.regions[selected]);
    hexPath(x, y, size * 0.92);
    ctx.lineWidth = 3; ctx.strokeStyle = css.getPropertyValue("--select"); ctx.stroke();
  }
  drawLegend(sc);
}

function outlinedText(t, x, y, px) {
  ctx.font = `700 ${px}px system-ui`;
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.lineWidth = 3; ctx.strokeStyle = "rgba(0,0,0,0.75)"; ctx.fillStyle = "#fff";
  ctx.strokeText(t, x, y); ctx.fillText(t, x, y);
}

function progressRing(x, y, f, color) {
  ctx.beginPath(); ctx.arc(x, y, size * 0.11, -Math.PI / 2, -Math.PI / 2 + 2 * Math.PI * Math.min(1, f));
  ctx.lineWidth = size * 0.06; ctx.strokeStyle = color; ctx.stroke();
  ctx.beginPath(); ctx.arc(x, y, size * 0.11, 0, 2 * Math.PI);
  ctx.lineWidth = 1; ctx.strokeStyle = "rgba(0,0,0,0.4)"; ctx.stroke();
}

// borders: the shared hex edge between tiles of different owners
function drawBorders() {
  for (const l of world.links) {
    if (owner(l.a) === owner(l.b)) continue;
    const [x1, y1] = center(world.regions[l.a]), [x2, y2] = center(world.regions[l.b]);
    const mx = (x1 + x2) / 2, my = (y1 + y2) / 2, len = Math.hypot(x2 - x1, y2 - y1);
    const px = -(y2 - y1) / len * size / 2, py = (x2 - x1) / len * size / 2;
    ctx.lineCap = "round";
    for (const [w, c] of [[5, "rgba(255,255,255,0.85)"], [2.5, "#222"]]) {
      ctx.lineWidth = w; ctx.strokeStyle = c;
      ctx.beginPath(); ctx.moveTo(mx + px, my + py); ctx.lineTo(mx - px, my - py); ctx.stroke();
    }
  }
}

// armies: a badge per army in its nation's colour with its size
function drawArmies() {
  const byTile = {};
  for (const a of state.armies) (byTile[a.region] = byTile[a.region] || []).push(a);
  for (const [tile, armies] of Object.entries(byTile)) {
    const [x, y] = center(world.regions[+tile]);
    armies.forEach((a, i) => {
      const bx = x + (i - (armies.length - 1) / 2) * size * 0.5, by = y + size * 0.12;
      const r = size * 0.2;
      ctx.beginPath(); ctx.arc(bx, by, r, 0, 2 * Math.PI);
      ctx.fillStyle = world.nations[a.nation].color; ctx.fill();
      ctx.lineWidth = a.id === selectedArmy ? 3 : 1.5;
      ctx.strokeStyle = a.id === selectedArmy ? getComputedStyle(document.documentElement).getPropertyValue("--select") : "#fff";
      ctx.stroke();
      if (a.retreating) { ctx.setLineDash([3, 3]); ctx.strokeStyle = "#000"; ctx.stroke(); ctx.setLineDash([]); }
      ctx.fillStyle = "#fff"; ctx.font = `700 ${Math.max(9, size * 0.17)}px system-ui`;
      ctx.textAlign = "center"; ctx.textBaseline = "middle";
      ctx.fillText(fmt(a.men, 0), bx, by);
    });
  }
}

// the marching route of the selected army (and the routes of your others, faintly)
function drawPaths() {
  for (const a of state.armies) {
    if (!a.path.length || (a.id !== selectedArmy && a.nation !== state.you)) continue;
    const pts = [a.region, ...a.path].map((id) => center(world.regions[id]));
    ctx.setLineDash([5, 5]); ctx.lineWidth = a.id === selectedArmy ? 3 : 1.5;
    ctx.strokeStyle = world.nations[a.nation].color;
    ctx.beginPath(); pts.forEach(([x, y], i) => (i ? ctx.lineTo(x, y + size * 0.12) : ctx.moveTo(x, y + size * 0.12))); ctx.stroke();
    ctx.setLineDash([]);
  }
}

function drawFlows(css) {
  const fg = $("flow-good").value, agg = new Map();
  for (const f of state.flows) {
    if (fg !== "all" && f.good !== +fg) continue;
    const v = fg === "all" ? f.qty * state.regions[f.from].prices[f.good] : f.qty;
    const k = f.from * 1000 + f.to + (f.sea ? 1e6 : 0);
    agg.set(k, (agg.get(k) || 0) + v);
  }
  const max = Math.max(1e-9, ...agg.values());
  ctx.strokeStyle = ctx.fillStyle = css.getPropertyValue("--flow");
  for (const [key, v] of agg) {
    if (v < max * 0.03) continue;
    const sea = key >= 1e6, k = key % 1e6;
    ctx.setLineDash(sea ? [8, 6] : []);
    const [x1, y1] = center(world.regions[Math.floor(k / 1000)]), [x2, y2] = center(world.regions[k % 1000]);
    const len = Math.hypot(x2 - x1, y2 - y1), ux = (x2 - x1) / len, uy = (y2 - y1) / len;
    const sx = -uy * size * 0.12, sy = ux * size * 0.12, w = 1 + 5 * Math.sqrt(v / max);
    const ax = x1 + ux * size * 0.35 + sx, ay = y1 + uy * size * 0.35 + sy, bx = x2 - ux * size * 0.45 + sx, by = y2 - uy * size * 0.45 + sy;
    ctx.lineWidth = w; ctx.lineCap = "round";
    ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(bx, by); ctx.stroke();
    ctx.setLineDash([]);
    const h = 4 + w;
    ctx.beginPath(); ctx.moveTo(bx + ux * h, by + uy * h); ctx.lineTo(bx - uy * h * 0.6, by + ux * h * 0.6); ctx.lineTo(bx + uy * h * 0.6, by - ux * h * 0.6);
    ctx.closePath(); ctx.fill();
  }
}

function drawLegend(sc) {
  const kind = $("overlay").value, el = $("legend");
  const sw = (c) => `<span class="swatch" style="background:${c}"></span>`;
  const marks = `<span>★ city</span><span>⚓ port</span><span>● army</span><span>⚑ occupation</span>`;
  if (kind === "nation") {
    el.innerHTML = world.nations.map((n) => `<span>${sw(n.color)}${esc(n.name)}</span>`).join("") + `<span>${sw(UNOWNED)}unowned</span>${marks}`;
  } else if (kind === "infra") {
    el.innerHTML = world.infra_levels.map((l, i) => `<span>${sw(INFRA_COLORS[i])}${l.name} (×${l.move_mult})</span>`).join("") + `<span>labels: movement cost</span>`;
  } else if (kind === "terrain") {
    el.innerHTML = world.terrains.map((t) => `<span>${sw(TERRAIN_COLORS[t.name])}${t.name}${t.move_cost < 1 ? ` (move ×${t.move_cost})` : ""}</span>`).join("") + marks;
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
    const bx = cx + (i - (here.length - 1) / 2) * size * 0.5, by = cy + size * 0.12;
    if (Math.hypot(bx - x, by - y) < size * 0.24) return here[i];
  }
  return null;
}
canvas.addEventListener("click", async (ev) => {
  const r = regionAt(ev);
  if (!r) return;
  if (moving && selectedArmy !== null) {
    moving = false;
    await command(`/armies/${selectedArmy}/move?to=${r.id}`);
    return;
  }
  const a = armyAt(ev, r);
  if (a) {
    selectedArmy = a.id;
    setTab("armies");
    render(true);
    return;
  }
  selected = r.id;
  setTab("tile");
  try { detail = await api("GET", `/api/games/${gid}/regions/${r.id}`); } catch (e) { showError("game-err", e); }
  render(true);
  if (window.innerWidth <= 760) document.querySelector(".tabs").scrollIntoView({ behavior: "smooth", block: "start" });
});
canvas.addEventListener("mousemove", (ev) => {
  const r = regionAt(ev), tip = $("tooltip");
  if (!r || !state) { tip.hidden = true; return; }
  const rs = state.regions[r.id];
  const v = CATEGORICAL.includes($("overlay").value) ? "" : ` · ${overlayLabel()}: ${fmtOverlay(overlayValue(r.id))}`;
  tip.textContent = `${r.name} ${r.terrain}${rs.city ? " city" : ""} · ${nationName(rs.nation)}${v}`;
  const rect = canvas.getBoundingClientRect();
  tip.style.left = ev.clientX - rect.left + 12 + "px"; tip.style.top = ev.clientY - rect.top + 12 + "px";
  tip.hidden = false;
});
canvas.addEventListener("mouseleave", () => { $("tooltip").hidden = true; });
for (const id of ["overlay", "good", "show-flows", "flow-good"]) {
  $(id).addEventListener("change", () => {
    $("good").hidden = $("overlay").value !== "price";
    $("flow-good").hidden = !$("show-flows").checked;
    draw();
  });
}
window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", draw);

// ---- tile panel ----------------------------------------------------------------
function tilePanel() {
  if (selected === null || !detail || detail.info.id !== selected) return `<p class="hint">Tap a tile on the map.</p>`;
  const d = detail, s = state.regions[selected], G = world.goods;
  const mine = state.you >= 0 && s.nation === state.you;
  const total = s.tier_spend.reduce((a, b) => a + b, 0) || 1;
  const bar = world.tiers.map((t, i) => `<span style="width:${(100 * s.tier_spend[i] / total).toFixed(1)}%;background:${TIER_COLORS[t]}"></span>`).join("");
  let html = `<h2>${s.city ? "★ City" : "Tile"} ${esc(d.info.name)}</h2>
    <p class="sub">${d.info.terrain} · ${chip(s.nation)} ${esc(nationName(s.nation))}${s.city ? ` · industry and services ×${s.city_bonus.toFixed(2)} from its size` : ""}</p>
    <div class="stats">
      <div class="stat"><div class="v">${fmt(s.population, 0)}</div><div class="k">people (${s.growth >= 0 ? "+" : ""}${(100 * s.growth).toFixed(1)}%/yr)</div></div>
      <div class="stat"><div class="v">${fmt(s.utility)}</div><div class="k">utility / person</div></div>
      <div class="stat"><div class="v">${fmt(s.wage)}</div><div class="k">wage</div></div>
    </div>`;
  html += armiesHere(s, mine);
  html += `<h3>Infrastructure</h3>${infraBlock(s, mine)}`;
  html += `<h3>Spending by need tier</h3><div class="bar-stack">${bar}</div>
    <p class="sub">${world.tiers.map((t, i) => `${t} ${pct(s.tier_spend[i] / total)}`).join(" · ")}</p>`;
  const firms = d.firms.map((f, i) => ({ ...f, i })).filter((f) => !f.enabled || f.labor > 0.05 || f.produced > 0.05 || f.levels > 1)
    .sort((a, b) => (b.enabled - a.enabled) || (b.labor - a.labor)).map((f) => {
      if (!f.enabled) return `<tr class="dim"><td>🔒 ${esc(f.name)}</td><td>${G[f.output].name}</td><td colspan="4">needs ${esc(f.requires)}</td></tr>`;
      let grow = "";
      if (f.expansion) grow = `${pct(f.expansion.delivered / world.level_cost)}${f.expansion.public ? " ★" : ""}`;
      else if (mine && f.levels < f.max_levels) grow = `<button data-cmd="/regions/${selected}/firms/${f.i}/expand" title="Your nation pays ${world.level_cost} construction for a level">+1</button>`;
      return `<tr><td>${esc(f.name)}</td><td>${G[f.output].name}</td><td>${f.levels}/${f.max_levels}</td><td>${fmt(f.labor, 1)}</td><td>${fmt(f.profit, 1)}</td><td>${grow}</td></tr>`;
    }).join("");
  html += `<h3>Firms</h3><div class="table-scroll"><table><tr><th>firm</th><th>makes</th><th>levels</th><th>workers</th><th>profit</th><th></th></tr>${firms}</table></div>
    <p class="sub">★ state-funded expansion; the rest are private.</p>`;
  const markets = d.markets.filter((m) => m.supply > 1e-3 || m.demand > 1e-3).map((m) =>
    `<tr class="${G[m.good].tier ? "" : "dim"}"><td>${G[m.good].name}</td><td>${fmt(m.price)}</td><td>${fmt(m.price / s.wage)}</td><td>${fmt(m.supply)}</td><td>${fmt(m.demand)}</td></tr>`).join("");
  html += `<h3>Markets</h3><div class="table-scroll"><table><tr><th>good</th><th>price</th><th>÷ wage</th><th>supply</th><th>demand</th></tr>${markets}</table></div>`;
  const trade = (list, out) => list.sort((a, b) => b.qty - a.qty).slice(0, 8).map((f) =>
    `<tr><td>${G[f.good].name}</td><td>${out ? "→" : "←"} ${world.regions[out ? f.to : f.from].name}${f.sea ? " ⚓" : ""}</td><td>${fmt(f.qty)}</td></tr>`).join("") || `<tr class="dim"><td colspan="3">none</td></tr>`;
  html += `<h3>Exports</h3><table>${trade(d.exports, true)}</table><h3>Imports</h3><table>${trade(d.imports, false)}</table>`;
  return html;
}

function armiesHere(s, mine) {
  const here = state.armies.filter((a) => a.region === selected);
  let html = "";
  if (s.occupation) {
    html += `<h3>Occupation</h3><div class="progress"><span style="width:${pct(s.occupation.progress)};background:${world.nations[s.occupation.nation].color}"></span></div>
      <p class="sub">${esc(nationName(s.occupation.nation))} holds ${pct(s.occupation.progress)}: it grows each day their army stands here unopposed, and falls back otherwise. At 100% the tile changes hands.</p>`;
  }
  if (here.length) {
    html += `<h3>Armies here</h3>` + here.map((a) =>
      `<div class="row">${chip(a.nation)} <span>${esc(nationName(a.nation))}: ${fmt(a.men, 0)} men, ${world.doctrines[a.doctrine].name}${a.retreating ? " (retreating)" : ""}</span>
       <button data-army="${a.id}">Select</button></div>`).join("");
  }
  const n = me();
  if (mine && n) {
    const room = Math.max(0, n.mobilize_cap - state.armies.filter((a) => a.nation === n.id).reduce((t, a) => t + a.target, 0));
    html += `<h3>Raise an army here</h3>
      <div class="row"><label>Soldiers <input id="raise-men" type="number" min="0" step="1" value="${Math.floor(room)}" class="num"></label>
      <button class="primary" id="raise">Raise</button></div>
      <p class="sub">They are drafted from this tile's people over the coming weeks. Room for ${fmt(room, 0)} more within your mobilization cap (${pct(n.mobilization)} of your people).</p>`;
  }
  return html;
}

const knows = (nation, techName) => !techName || state.nations[nation].known[world.techs.findIndex((t) => t.name === techName)];
const canPort = (nation) => world.techs.some((t, i) => t.ports && state.nations[nation].known[i]);
const portTech = () => (world.techs.find((t) => t.ports) || {}).name;

function infraBlock(s, mine) {
  const lvl = world.infra_levels[s.infra], next = world.infra_levels[s.infra + 1];
  const coast = world.regions[selected].terrain === "coast";
  let html = `<p class="sub">${lvl.name} · moving through costs ×${s.move_cost.toFixed(2)}${lvl.upkeep ? ` · upkeep ${lvl.upkeep}/day` : ""}</p>`;
  if (coast) html += `<p class="sub">${s.port ? "⚓ Port: ships sail straight to every other port." : "No port."}</p>`;
  if (s.project) {
    const what = s.project.port ? "port" : world.infra_levels[s.project.target].name;
    html += `<div class="progress"><span style="width:${pct(s.project.delivered / s.project.required)}"></span></div>
      <div class="row"><span class="sub">Building ${what}: ${fmt(s.project.delivered, 0)} / ${s.project.required}</span>
      ${mine ? `<button data-cmd="/regions/${selected}/cancel">Cancel</button>` : ""}</div>`;
    return html;
  }
  if (mine && coast && !s.port) {
    const mats = Object.entries(world.port.materials).map(([g, q]) => ` + ${q} ${g}`).join("");
    html += canPort(state.you)
      ? `<div class="row"><button class="primary" data-cmd="/regions/${selected}/port">Build port</button><span class="sub">${world.port.cost} construction${mats}</span></div>`
      : `<p class="sub">A port needs ${portTech()}.</p>`;
  }
  if (next && mine) {
    const mats = Object.entries(next.materials || {}).map(([g, q]) => ` + ${q} ${g}`).join("");
    html += knows(state.you, next.tech)
      ? `<div class="row"><button class="primary" data-cmd="/regions/${selected}/build">Build ${next.name}</button><span class="sub">${next.cost} construction${mats}</span></div>`
      : `<p class="sub">${next.name} needs ${next.tech}.</p>`;
  }
  return html;
}

// ---- nation panel --------------------------------------------------------------
function nationPanel() {
  const n = me();
  if (!n) return `<p class="hint">You are watching. To play, go back to the games list and pick a nation marked “Play”.</p>`;
  const info = world.nations[n.id];
  if (n.eliminated) return `<h2>${esc(info.name)} has fallen</h2><p class="hint">All your tiles were taken.</p>`;
  const kept = Math.max(0, 1 - n.budget.reduce((a, b) => a + b, 0));
  const sliders = world.budgets.map((name, i) =>
    `<label>${name}<input type="range" min="0" max="100" step="5" value="${Math.round(100 * n.budget[i])}" data-budget="${i}" aria-label="${name} budget"><span class="pct" id="pct-${i}">${pct(n.budget[i])}</span></label>`).join("");
  const tradable = world.goods.map((g, i) => i).filter((i) => !["transport", "construction", "research"].includes(world.goods[i].name));
  const tariff = Math.max(...tradable.map((i) => n.tariff[i]));
  const others = world.nations.filter((o) => o.id !== n.id).map((o) =>
    `<label class="check"><input type="checkbox" data-embargo="${o.id}" ${n.embargo[o.id] ? "checked" : ""}> ${esc(o.name)}</label>`).join("");
  return `<h2>${chip(n.id)} ${esc(info.name)}</h2>
    <div class="stats">
      <div class="stat"><div class="v">${fmt(n.income, 0)}</div><div class="k">income / day</div></div>
      <div class="stat"><div class="v">${fmt(n.treasury, 0)}</div><div class="k">treasury</div></div>
      <div class="stat"><div class="v">${fmt(n.soldiers, 0)}</div><div class="k">soldiers (cap ${fmt(n.mobilize_cap, 0)})</div></div>
    </div>
    <h3>Budget (share of income)</h3>
    <div class="budget">${sliders}</div>
    <div class="row"><span class="sub" id="kept">${pct(kept)} kept in the treasury</span><button class="primary" id="apply-budget">Apply</button></div>
    <p class="sub">Economy pays for roads, upkeep and firm expansions; science buys research; military buys your armies' food, clothing and equipment. Armies short of supplies fight worse and lose men.</p>
    <h3>Taxes and trade</h3>
    <div class="row"><label>Tax <input id="tax" type="number" min="0" max="90" step="1" value="${Math.round(100 * n.tax_rate)}" class="num">%</label><button data-input="tax">Set</button></div>
    <div class="row"><label>Import tariff <input id="tariff" type="number" min="0" max="500" step="5" value="${Math.round(100 * tariff)}" class="num">%</label><button data-input="tariff">Set</button></div>
    <div class="row"><span class="sub">Embargo:</span> ${others}</div>
    <p class="sub">Tax ${fmt(n.revenue, 1)} + tariffs ${fmt(n.tariff_revenue, 1)} a day · spending ${fmt(n.spending, 1)} · returned to your people ${fmt(n.rebate, 1)}</p>
    ${researchBlock(n)}`;
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
    if (n.known[i]) action = "✓";
    else if (n.researching === i) action = pct(Math.min(1, n.research_points / n.tech_costs[i]));
    else {
      const now = available(t) ? `<button data-cmd="/nations/${n.id}/research?tech=${i}">Now</button>` : "";
      const q = n.queue.includes(i) ? "" : `<button data-queue="${i}">Queue</button>`;
      action = `${now}${q}`;
    }
    const cost = n.tech_costs[i] < t.cost - 1 ? `${fmt(n.tech_costs[i], 0)} <span class="sub">(−${pct(1 - n.tech_costs[i] / t.cost)})</span>` : fmt(t.cost, 0);
    const needs = !n.known[i] && !available(t) ? `<br><span class="sub">needs ${t.requires.join(", ")}</span>` : "";
    return `<tr class="${n.known[i] ? "dim" : ""}"><td>${t.name}<br><span class="sub">${world.eras[t.era].split(" (")[0]}</span></td><td>${t.effect}${needs}</td><td>${cost}</td><td>${action}</td></tr>`;
  }).join("");
  const branches = world.branches.map((b) => {
    const all = world.techs.filter((t) => t.branch === b).length, known = world.techs.filter((t, i) => t.branch === b && n.known[i]).length;
    return `<tr class="branch"><th colspan="4">${b} · ${known}/${all}</th></tr>${rows(b)}`;
  }).join("");
  return `<h3>Research</h3>
    <p class="sub">Researching ${current}. Techs your trade partners know are cheaper: ideas travel with trade.</p>
    <div class="row">${queue}</div>
    <div class="table-scroll"><table>${branches}</table></div>`;
}

// keep the three budget shares within 100% while dragging
$("tab-nation").addEventListener("input", (ev) => {
  if (ev.target.dataset.budget === undefined) return;
  const ranges = [...document.querySelectorAll("[data-budget]")];
  let over = ranges.reduce((a, r) => a + +r.value, 0) - 100;
  for (const r of ranges) {
    if (over <= 0) break;
    if (r === ev.target) continue;
    const cut = Math.min(+r.value, over);
    r.value = +r.value - cut;
    over -= cut;
  }
  ranges.forEach((r, i) => { $(`pct-${i}`).textContent = `${r.value}%`; });
  $("kept").textContent = `${100 - ranges.reduce((a, r) => a + +r.value, 0)}% kept in the treasury`;
});

// ---- armies panel --------------------------------------------------------------
function armiesPanel() {
  const n = me();
  const mine = n ? state.armies.filter((a) => a.nation === n.id) : [];
  const others = state.armies.filter((a) => !n || a.nation !== n.id);
  let html = "";
  if (n) {
    html += `<h2>Your armies</h2>
      <p class="sub">${fmt(n.soldiers, 0)} soldiers of ${fmt(n.mobilize_cap, 0)} allowed (${pct(n.mobilization)} of your people). Best doctrine: ${world.doctrines[n.best_doctrine].name}.
      Raise armies from a tile of yours (Tile tab).</p>`;
    html += mine.length ? mine.map((a) => armyCard(a, true)).join("") : `<p class="hint">You have no armies.</p>`;
  }
  html += `<h3>Other armies</h3>` + (others.length ? others.map((a) => armyCard(a, false)).join("") : `<p class="hint">None.</p>`);
  return html;
}

function armyCard(a, mine) {
  const d = world.doctrines[a.doctrine];
  const where = world.regions[a.region];
  const dest = a.path.length ? ` → marching to ${world.regions[a.path[a.path.length - 1]].name}` : "";
  const supply = Object.entries(d.needs).map(([g]) => {
    const v = a.supply[g] ?? 0;
    const color = v > 0.8 ? "#4caf50" : v > 0.4 ? "#e0a020" : "#d9480f";
    return `<span>${g}</span><span class="meter"><span style="width:${pct(v)};background:${color}"></span></span><span>${pct(v)}</span>`;
  }).join("");
  let actions = "";
  if (mine) {
    const n = me();
    actions = `<div class="row">
      <button class="primary" data-move="${a.id}">Move…</button>
      <label>Size <input type="number" min="0" step="1" value="${Math.round(a.target)}" class="num" data-target-for="${a.id}"></label><button data-set-target="${a.id}">Set</button>
      ${n.best_doctrine > a.doctrine ? `<button data-cmd="/armies/${a.id}/upgrade">Upgrade to ${world.doctrines[n.best_doctrine].name}</button>` : ""}
      ${state.regions[a.region].nation === a.nation ? `<button class="danger" data-cmd="/armies/${a.id}/disband">Disband</button>` : ""}
    </div>`;
  }
  return `<div class="army-card ${a.id === selectedArmy ? "selected" : ""}" data-select-army="${a.id}">
    <div class="title"><b>${chip(a.nation)} ${esc(nationName(a.nation))} army ${a.id}</b><span class="sub">${fmt(a.men, 0)} / ${fmt(a.target, 0)} men</span></div>
    <p class="sub">${d.name} · strength ${fmt(a.strength, 1)} (effectiveness ${fmt(a.effectiveness)} of ${d.power}) · at ${where.name}${dest}${a.retreating ? " · retreating" : ""}</p>
    <div class="supply">${supply}</div>${actions}</div>`;
}

// ---- world panel ---------------------------------------------------------------
function worldPanel() {
  const n = me();
  const rows = state.nations.map((o) => {
    const info = world.nations[o.id];
    const who = o.id === state.you ? "you" : o.controller === "ai" ? "computer" : "player";
    let diplo = "";
    if (n && o.id !== n.id && !o.eliminated && !n.eliminated) {
      if (n.at_war[o.id]) {
        diplo = n.peace_offer[o.id] ? `<span class="sub">peace offered</span>`
          : `<button data-cmd="/nations/${n.id}/peace?target=${o.id}">${o.peace_offer[n.id] ? "Accept peace" : "Offer peace"}</button>`;
      } else {
        diplo = `<button class="danger" data-confirm="Declare war on ${esc(info.name)}?" data-cmd="/nations/${n.id}/war?target=${o.id}">Declare war</button>`;
      }
    }
    const war = o.at_war.map((w, i) => (w ? world.nations[i].name : null)).filter(Boolean);
    return `<tr><td>${chip(o.id)} ${esc(info.name)}</td><td>${who}</td><td>${o.eliminated ? "fallen" : o.tiles}</td>
      <td>${fmt(o.population, 0)}</td><td>${fmt(o.income, 0)}</td><td>${fmt(o.soldiers, 0)}</td><td>${o.known.filter(Boolean).length}/${world.techs.length}</td>
      <td>${war.length ? `⚔ ${war.map(esc).join(", ")}` : "peace"}</td><td>${diplo}</td></tr>`;
  }).join("");
  return `<h2>Nations</h2><div class="table-scroll"><table>
    <tr><th>nation</th><th>who</th><th>tiles</th><th>people</th><th>income</th><th>soldiers</th><th>techs</th><th>wars</th><th></th></tr>${rows}</table></div>
    <p class="sub">You can only go to war with a neighbour. Peace needs both sides: a computer nation accepts after a year if it isn't clearly winning, or after three years regardless.</p>
    <p class="sub">Money in the world: ${fmt(state.money, 0)} (conserved).</p>`;
}

// ---- news panel ----------------------------------------------------------------
function newsPanel() {
  const items = [...state.events].reverse().map((e) =>
    `<li class="${e.kind}"><span class="when">${dateOf(e.tick)}</span>${esc(e.text)}</li>`).join("");
  return `<h2>News</h2>
    <label class="check"><input type="checkbox" id="autopause" ${pref("autopause", true) ? "checked" : ""}> Pause when war, battle or occupation news concerns me</label>
    <div class="news"><ul>${items || `<li class="sub">Nothing yet.</li>`}</ul></div>`;
}

// ---- panel actions -------------------------------------------------------------
document.querySelector(".side").addEventListener("click", (ev) => {
  const b = ev.target.closest("button");
  if (b && b.id === "cancel-move") { moving = false; render(true); return; }
  if (!b) {
    const card = ev.target.closest("[data-select-army]");
    if (card) { selectedArmy = +card.dataset.selectArmy; render(true); }
    return;
  }
  if (b.dataset.confirm && !confirm(b.dataset.confirm)) return;
  if (b.dataset.cmd) command(b.dataset.cmd);
  if (b.dataset.army) { selectedArmy = +b.dataset.army; setTab("armies"); render(true); }
  if (b.dataset.move) { selectedArmy = +b.dataset.move; moving = true; render(true); if (window.innerWidth <= 760) canvas.scrollIntoView({ behavior: "smooth" }); }
  if (b.dataset.setTarget) command(`/armies/${b.dataset.setTarget}/target?men=${+document.querySelector(`[data-target-for="${b.dataset.setTarget}"]`).value}`);
  if (b.id === "raise") command(`/nations/${state.you}/armies?region=${selected}&target=${+$("raise-men").value}`);
  if (b.dataset.queue !== undefined) command(`/nations/${state.you}/queue?techs=${[...me().queue, +b.dataset.queue].join(",")}`);
  if (b.dataset.unqueue !== undefined) {
    const q = [...me().queue]; q.splice(+b.dataset.unqueue, 1);
    command(`/nations/${state.you}/queue?techs=${q.join(",")}`);
  }
  if (b.id === "apply-budget") {
    const q = world.budgets.map((name, i) => `${name}=${+document.querySelector(`[data-budget="${i}"]`).value / 100}`).join("&");
    command(`/nations/${state.you}/budget?${q}`);
  }
  if (b.dataset.input === "tax") command(`/nations/${state.you}/tax?rate=${(+$("tax").value / 100).toFixed(3)}`);
  if (b.dataset.input === "tariff") command(`/nations/${state.you}/tariff?rate=${(+$("tariff").value / 100).toFixed(3)}`);
});
document.querySelector(".side").addEventListener("change", (ev) => {
  const t = ev.target.dataset.embargo;
  if (t !== undefined) command(`/nations/${state.you}/embargo?target=${t}&on=${ev.target.checked}`);
  if (ev.target.id === "autopause") setPref("autopause", ev.target.checked);
});

route();
