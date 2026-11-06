"use strict";

// ---- basics --------------------------------------------------------------------
const $ = (id) => document.getElementById(id);
const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const fmt = (x, d = 2) => (Math.abs(x) >= 1000 ? Math.round(x).toLocaleString() : x.toFixed(d));
const pct = (x) => `${Math.round(100 * x)}%`;

let gid = null;       // current game
let world = null;     // its static description
let state = null;     // its latest state
let detail = null;    // selected tile's details
let selected = null;  // selected tile id
let tab = "tile";
let pollTimer = null;

// seats (player tokens) are kept per game in this browser
const seatKey = (g) => `mercury:seat:${g}`;
function getSeat(g) {
  try { return JSON.parse(localStorage.getItem(seatKey(g))) || null; } catch { return null; }
}
function setSeat(g, seat) {
  try { seat ? localStorage.setItem(seatKey(g), JSON.stringify(seat)) : localStorage.removeItem(seatKey(g)); } catch { /* storage blocked */ }
}

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
    ? `<div class="table-scroll"><table><tr><th>slot</th><th>game</th><th>tick</th><th>saved</th><th></th></tr>${saves.map((s) =>
      `<tr><td>${esc(s.slot)}</td><td>${esc(s.name)}</td><td>${s.tick}</td><td>${new Date(s.saved_at).toLocaleString()}</td>
       <td><button data-load="${esc(s.slot)}">Load</button></td></tr>`).join("")}</table></div>`
    : `<p class="hint">No saves yet. Save a game from its top bar.</p>`;
}

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
    <div class="title"><b>${esc(g.name)}</b><span class="sub">tick ${g.tick}${g.running ? " · running" : ""}</span></div>
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
    const q = new URLSearchParams({ name: $("new-name").value.trim(), cost: $("new-cost").value });
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
  selected = null; detail = null;
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
  $("speed").innerHTML = [1, 2, 5, 10, 20, 50].map((s) => `<option value="${s}">${s}/s</option>`).join("");
  if (state.you < 0 && getSeat(id)) setSeat(id, null); // the server no longer knows our seat
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
      render();
    } catch (e) { showError("game-err", e); }
    if (gid) pollTimer = setTimeout(tickOnce, state && state.running ? 700 : 2500);
  };
  pollTimer = setTimeout(tickOnce, 700);
}
function stopPolling() { clearTimeout(pollTimer); pollTimer = null; }

const humans = () => state.nations.some((n) => n.controller === "human");
const canClock = () => !humans() || state.you >= 0;
const me = () => (state.you >= 0 ? state.nations[state.you] : null);
const owner = (id) => state.regions[id].nation;
const neighbours = (id) => world.links.filter((l) => l.a === id || l.b === id).map((l) => (l.a === id ? l.b : l.a));

async function command(path, after) {
  try {
    state = await api("POST", `/api/games/${gid}${path}`);
    if (selected !== null) detail = await api("GET", `/api/games/${gid}/regions/${selected}`);
    if (after) after();
    render(true);
  } catch (e) { showError("game-err", e); }
}

// ---- top bar -------------------------------------------------------------------
function renderBar() {
  $("tick").textContent = state.tick;
  $("play").textContent = state.running ? "Pause" : "Play";
  $("speed").value = String(state.speed);
  for (const id of ["play", "speed", "step10", "save"]) $(id).disabled = !canClock();
  $("leave").hidden = state.you < 0;
  const n = me();
  $("you").hidden = !n;
  if (!n) return;
  const info = world.nations[n.id];
  const c = n.campaign ? `<span>⚔ occupying ${world.regions[n.campaign.target].name}: <b>${pct(n.campaign.progress)}</b></span>` : "";
  $("you").innerHTML = `<span><span class="chip" style="background:${info.color}"></span> <b>${esc(info.name)}</b>${n.eliminated ? " (fallen)" : ""}</span>
    <span>tiles <b>${n.tiles}</b></span><span>people <b>${fmt(n.population, 0)}</b></span>
    <span>treasury <b>${fmt(n.treasury, 0)}</b></span><span>income <b>${fmt(n.income, 0)}</b>/tick</span>
    <span>army <b>${fmt(n.army, 0)}</b></span>${c}`;
}
$("to-lobby").onclick = () => { location.hash = "#/"; };
$("play").onclick = () => command(`/clock?running=${!state.running}`);
$("speed").onchange = () => command(`/clock?speed=${$("speed").value}`);
$("step10").onclick = () => command("/step?n=10");
$("leave").onclick = () => command("/leave", () => { setSeat(gid, null); setTab("world"); });
$("save").onclick = async () => {
  const suggestion = ($("game-name").textContent || "game").replace(/[^A-Za-z0-9_-]+/g, "-").slice(0, 40);
  const slot = prompt("Save as (letters, digits, - or _):", suggestion);
  if (!slot) return;
  try {
    const info = await api("POST", `/api/games/${gid}/save?slot=${encodeURIComponent(slot)}`);
    say("game-err", `Saved “${info.slot}” at tick ${info.tick}.`, false);
  } catch (e) { showError("game-err", e); }
};

// ---- tabs ----------------------------------------------------------------------
function setTab(t) {
  tab = t;
  for (const b of document.querySelectorAll(".tabs button")) b.classList.toggle("active", b.dataset.tab === t);
  for (const name of ["tile", "nation", "world"]) $(`tab-${name}`).hidden = name !== t;
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
  const el = $(`tab-${tab}`);
  if (!force && busy(el)) return;
  if (tab === "tile") el.innerHTML = tilePanel();
  if (tab === "nation") el.innerHTML = nationPanel();
  if (tab === "world") el.innerHTML = worldPanel();
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
function share(rs, tier) {
  const total = rs.tier_spend.reduce((a, b) => a + b, 0);
  return total > 0 ? rs.tier_spend[world.tiers.indexOf(tier)] / total : 0;
}
function overlayValue(id) {
  const rs = state.regions[id], g = +$("good").value;
  switch ($("overlay").value) {
    case "utility": return rs.utility;
    case "population": return rs.population;
    case "wage": return rs.wage;
    case "price": return rs.prices[g] / rs.wage;
    case "basic": return share(rs, "basic");
    case "luxury": return share(rs, "luxury");
  }
  return 0;
}
const overlayLabel = () => ($("overlay").value === "price" ? `${world.goods[+$("good").value].name} price ÷ wage` : $("overlay").selectedOptions[0].textContent);

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
    if (kind !== "terrain") { // terrain pip, so terrain stays visible under other overlays
      ctx.beginPath(); ctx.arc(x, y - size * 0.55, size * 0.1, 0, 2 * Math.PI);
      ctx.fillStyle = TERRAIN_COLORS[r.terrain]; ctx.fill(); ctx.strokeStyle = "rgba(0,0,0,0.4)"; ctx.stroke();
    }
    let label, dark;
    if (sc) { const v = overlayValue(r.id); label = isShare() ? pct(v) : fmt(v, 1); dark = sc.t(v) <= 0.6; }
    else if (kind === "nation") { label = rs.nation >= 0 ? world.nations[rs.nation].name.slice(0, 3) : "–"; dark = rs.nation >= 0; }
    else if (kind === "infra") { label = "×" + rs.move_cost.toFixed(2); dark = rs.infra >= 2; }
    else { label = r.terrain[0].toUpperCase(); dark = false; }
    ctx.fillStyle = dark ? "#fff" : "#1b1b1b";
    ctx.font = `600 ${Math.max(10, size * 0.28)}px system-ui`;
    ctx.textAlign = "center"; ctx.textBaseline = "middle";
    ctx.fillText(label, x, y + size * 0.05);
    for (let i = 0; i < rs.infra; i++) { // one bar per road level
      ctx.fillStyle = INFRA_COLORS[2]; ctx.strokeStyle = "#fff"; ctx.lineWidth = 1;
      const bx = x - size * 0.37 + i * size * 0.27, by = y + size * 0.42;
      ctx.fillRect(bx, by, size * 0.22, size * 0.08); ctx.strokeRect(bx, by, size * 0.22, size * 0.08);
    }
    if (rs.project) progressRing(x + size * 0.45, y - size * 0.45, rs.project.delivered / rs.project.required, "#e8590c");
    if (rs.port) {
      ctx.font = `${Math.max(11, size * 0.3)}px system-ui`;
      ctx.fillStyle = "#fff"; ctx.strokeStyle = "rgba(0,0,0,0.75)"; ctx.lineWidth = 3;
      ctx.strokeText("⚓", x - size * 0.45, y - size * 0.4); ctx.fillText("⚓", x - size * 0.45, y - size * 0.4);
    }
  }
  if ($("show-flows").checked) drawFlows(css);
  drawBorders();
  drawCampaigns();
  if (selected !== null) {
    const [x, y] = center(world.regions[selected]);
    hexPath(x, y, size * 0.92);
    ctx.lineWidth = 3; ctx.strokeStyle = css.getPropertyValue("--select"); ctx.stroke();
  }
  drawLegend(sc);
}

function progressRing(x, y, f, color) {
  ctx.beginPath(); ctx.arc(x, y, size * 0.12, -Math.PI / 2, -Math.PI / 2 + 2 * Math.PI * Math.min(1, f));
  ctx.lineWidth = size * 0.06; ctx.strokeStyle = color; ctx.stroke();
  ctx.beginPath(); ctx.arc(x, y, size * 0.12, 0, 2 * Math.PI);
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

function drawCampaigns() {
  for (const n of state.nations) {
    if (!n.campaign) continue;
    const [x, y] = center(world.regions[n.campaign.target]);
    ctx.setLineDash([6, 4]);
    hexPath(x, y, size * 0.8);
    ctx.lineWidth = 3; ctx.strokeStyle = world.nations[n.id].color; ctx.stroke();
    ctx.setLineDash([]);
    ctx.font = `700 ${Math.max(10, size * 0.24)}px system-ui`;
    ctx.fillStyle = "#fff"; ctx.strokeStyle = "rgba(0,0,0,0.7)"; ctx.lineWidth = 3;
    const t = `⚔ ${pct(n.campaign.progress)}`;
    ctx.strokeText(t, x, y + size * 0.32); ctx.fillText(t, x, y + size * 0.32);
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
    const h = 4 + w;
    ctx.setLineDash([]);
    ctx.beginPath(); ctx.moveTo(bx + ux * h, by + uy * h); ctx.lineTo(bx - uy * h * 0.6, by + ux * h * 0.6); ctx.lineTo(bx + uy * h * 0.6, by - ux * h * 0.6);
    ctx.closePath(); ctx.fill();
  }
}

function drawLegend(sc) {
  const kind = $("overlay").value, el = $("legend");
  const sw = (c) => `<span class="swatch" style="background:${c}"></span>`;
  if (kind === "nation") {
    el.innerHTML = world.nations.map((n) => `<span>${sw(n.color)}${esc(n.name)}</span>`).join("")
      + `<span>${sw(UNOWNED)}unowned</span><span>⚔ occupation under way</span><span>⚓ port</span>`;
  } else if (kind === "infra") {
    el.innerHTML = world.infra_levels.map((l, i) => `<span>${sw(INFRA_COLORS[i])}${l.name} (×${l.move_mult})</span>`).join("") + `<span>labels: movement cost</span>`;
  } else if (kind === "terrain") {
    el.innerHTML = world.terrains.map((t) => `<span>${sw(TERRAIN_COLORS[t.name])}${t.name}${t.move_cost < 1 ? ` (move ×${t.move_cost})` : ""}</span>`).join("");
  } else {
    const f = (v) => (isShare() ? pct(v) : fmt(v));
    el.innerHTML = `<span>${esc(overlayLabel())}</span><span>${f(sc.lo)}</span><span class="ramp" style="background:linear-gradient(90deg,${RAMP.join(",")})"></span><span>${f(sc.hi)}</span>`;
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
canvas.addEventListener("click", async (ev) => {
  const r = regionAt(ev);
  if (!r) return;
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
  const who = rs.nation >= 0 ? world.nations[rs.nation].name : "unowned";
  const v = CATEGORICAL.includes($("overlay").value) ? "" : ` · ${overlayLabel()}: ${isShare() ? pct(overlayValue(r.id)) : fmt(overlayValue(r.id))}`;
  tip.textContent = `${r.name} ${r.terrain} · ${who}${v}`;
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
  const nation = s.nation >= 0 ? world.nations[s.nation] : null;
  const mine = state.you >= 0 && s.nation === state.you;
  const total = s.tier_spend.reduce((a, b) => a + b, 0) || 1;
  const bar = world.tiers.map((t, i) => `<span style="width:${(100 * s.tier_spend[i] / total).toFixed(1)}%;background:${TIER_COLORS[t]}"></span>`).join("");
  let html = `<h2>Tile ${esc(d.info.name)}</h2>
    <p class="sub">${d.info.terrain} · ${nation ? `<span class="chip" style="background:${nation.color}"></span> ${esc(nation.name)} · defended by ${fmt(s.defense, 0)}` : "unowned · undefended"}</p>
    <div class="stats">
      <div class="stat"><div class="v">${fmt(s.utility)}</div><div class="k">utility / person</div></div>
      <div class="stat"><div class="v">${fmt(s.wage)}</div><div class="k">wage</div></div>
      <div class="stat"><div class="v">${fmt(s.population, 0)}</div><div class="k">people</div></div>
    </div>`;
  html += militaryActions(s);
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
    `<tr><td>${G[f.good].name}</td><td>${out ? "→" : "←"} ${world.regions[out ? f.to : f.from].name}</td><td>${fmt(f.qty)}</td></tr>`).join("") || `<tr class="dim"><td colspan="3">none</td></tr>`;
  html += `<h3>Exports</h3><table>${trade(d.exports, true)}</table><h3>Imports</h3><table>${trade(d.imports, false)}</table>`;
  return html;
}

function militaryActions(s) {
  const n = me();
  if (!n || n.eliminated || s.nation === state.you) return "";
  if (n.campaign && n.campaign.target === selected) {
    return `<h3>Occupation</h3><div class="progress"><span style="width:${pct(n.campaign.progress)}"></span></div>
      <div class="row"><span class="sub">Your army (${fmt(n.army, 0)}) is occupying this tile: ${pct(n.campaign.progress)}.</span>
      <button data-cmd="/nations/${state.you}/campaign/cancel">Call off</button></div>`;
  }
  if (!neighbours(selected).some((id) => owner(id) === state.you)) return "";
  const odds = s.nation < 0 ? "Undefended: it falls slowly to any army, faster to a bigger one."
    : `Their garrison counts as ${fmt(1.5 * s.defense, 0)}; your army is ${fmt(n.army, 0)}. Fighting costs both sides.`;
  return `<h3>Occupation</h3><p class="sub">${odds}${n.campaign ? " This calls off your current campaign." : ""}</p>
    <div class="row"><button class="primary" data-cmd="/nations/${state.you}/campaign?target=${selected}" ${n.army <= 0 ? "disabled" : ""}>Occupy</button></div>`;
}

const knows = (nation, techName) => !techName || state.nations[nation].known[world.techs.findIndex((t) => t.name === techName)];
const canPort = (nation) => world.techs.some((t, i) => t.ports && state.nations[nation].known[i]);
const portTech = () => (world.techs.find((t) => t.ports) || {}).name;

function infraBlock(s, mine) {
  const lvl = world.infra_levels[s.infra], next = world.infra_levels[s.infra + 1];
  const coast = world.regions[selected].terrain === "coast";
  let html = `<p class="sub">${lvl.name} · moving through costs ×${s.move_cost.toFixed(2)}${lvl.upkeep ? ` · upkeep ${lvl.upkeep}/tick` : ""}</p>`;
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
    const known = knows(state.you, next.tech);
    const mats = Object.entries(next.materials || {}).map(([g, q]) => ` + ${q} ${g}`).join("");
    html += known ? `<div class="row"><button class="primary" data-cmd="/regions/${selected}/build">Build ${next.name}</button><span class="sub">${next.cost} construction${mats}</span></div>`
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
  const tradable = world.goods.map((g, i) => i).filter((i) => !["transport", "construction", "research", "military"].includes(world.goods[i].name));
  const tariff = Math.max(...tradable.map((i) => n.tariff[i]));
  const others = world.nations.filter((o) => o.id !== n.id).map((o) =>
    `<label class="check"><input type="checkbox" data-embargo="${o.id}" ${n.embargo[o.id] ? "checked" : ""}> ${esc(o.name)}</label>`).join("");
  const techRow = (t, i) => {
    let status;
    if (n.known[i]) status = "✓";
    else if (n.researching === i) status = pct(Math.min(1, n.research_points / t.cost));
    else if (t.requires.every((r) => n.known[world.techs.findIndex((x) => x.name === r)])) status = `<button data-cmd="/nations/${n.id}/research?tech=${i}">Research</button>`;
    else status = `<span class="sub">needs ${t.requires.join(", ")}</span>`;
    return `<tr class="${n.known[i] ? "dim" : ""}"><td>${t.name}</td><td>${t.effect}</td><td>${t.cost}</td><td>${status}</td></tr>`;
  };
  const techs = world.branches.map((b) => {
    const rows = world.techs.map((t, i) => [t, i]).filter(([t]) => t.branch === b);
    const known = rows.filter(([, i]) => n.known[i]).length;
    return `<tr class="branch"><th colspan="4">${b} · ${known}/${rows.length}</th></tr>${rows.map(([t, i]) => techRow(t, i)).join("")}`;
  }).join("");
  const campaign = n.campaign
    ? `<div class="progress"><span style="width:${pct(n.campaign.progress)}"></span></div>
       <div class="row"><span class="sub">Occupying tile ${world.regions[n.campaign.target].name}: ${pct(n.campaign.progress)}</span><button data-cmd="/nations/${n.id}/campaign/cancel">Call off</button></div>`
    : `<p class="sub">No campaign. Tap a tile on your border and choose Occupy.</p>`;
  return `<h2><span class="chip" style="background:${info.color}"></span> ${esc(info.name)}</h2>
    <div class="stats">
      <div class="stat"><div class="v">${fmt(n.income, 0)}</div><div class="k">income / tick</div></div>
      <div class="stat"><div class="v">${fmt(n.treasury, 0)}</div><div class="k">treasury</div></div>
      <div class="stat"><div class="v">${fmt(n.army, 0)}</div><div class="k">army</div></div>
    </div>
    <h3>Budget (share of income)</h3>
    <div class="budget">${sliders}</div>
    <div class="row"><span class="sub" id="kept">${pct(kept)} kept in the treasury</span><button class="primary" id="apply-budget">Apply</button></div>
    <p class="sub">Economy pays for roads, upkeep and firm expansions; science for research; military for the army, which shrinks 2% a tick unless it is paid for.</p>
    <h3>Army</h3>
    <p class="sub">Strength ${fmt(n.army, 0)}; each of your tiles can call on ${fmt(n.defense, 0)} of it.</p>${campaign}
    <h3>Taxes and trade</h3>
    <div class="row"><label>Tax <input id="tax" type="number" min="0" max="90" step="1" value="${Math.round(100 * n.tax_rate)}" class="num">%</label><button data-input="tax">Set</button></div>
    <div class="row"><label>Import tariff <input id="tariff" type="number" min="0" max="500" step="5" value="${Math.round(100 * tariff)}" class="num">%</label><button data-input="tariff">Set</button></div>
    <div class="row"><span class="sub">Embargo:</span> ${others}</div>
    <p class="sub">Tax ${fmt(n.revenue, 1)} + tariffs ${fmt(n.tariff_revenue, 1)} a tick · spending ${fmt(n.spending, 1)} · returned to your people ${fmt(n.rebate, 1)}</p>
    <h3>Research</h3>
    <p class="sub">Researching ${n.researching >= 0 ? `${world.techs[n.researching].name}, ${pct(Math.min(1, n.research_points / world.techs[n.researching].cost))}` : "nothing"}. With nothing chosen, the cheapest available tech is picked.</p>
    <div class="table-scroll"><table>${techs}</table></div>`;
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

// ---- world panel ---------------------------------------------------------------
function worldPanel() {
  const rows = state.nations.map((n) => {
    const info = world.nations[n.id];
    const res = n.researching >= 0 ? `${world.techs[n.researching].name} ${pct(Math.min(1, n.research_points / world.techs[n.researching].cost))}` : "—";
    const who = n.id === state.you ? "you" : n.controller === "ai" ? "computer" : "player";
    return `<tr><td><span class="chip" style="background:${info.color}"></span> ${esc(info.name)}</td><td>${who}</td><td>${n.eliminated ? "fallen" : n.tiles}</td>
      <td>${fmt(n.population, 0)}</td><td>${fmt(n.income, 0)}</td><td>${fmt(n.army, 0)}</td>
      <td>${n.budget.map((b) => Math.round(100 * b)).join("/")}</td><td>${n.known.filter(Boolean).length}/${world.techs.length}</td><td>${res}</td></tr>`;
  }).join("");
  return `<h2>Nations</h2><div class="table-scroll"><table>
    <tr><th>nation</th><th>who</th><th>tiles</th><th>people</th><th>income</th><th>army</th><th>econ/sci/mil</th><th>techs</th><th>researching</th></tr>${rows}</table></div>
    <p class="sub">Money in the world: ${fmt(state.money, 0)} (conserved).</p>`;
}

// ---- panel actions -------------------------------------------------------------
document.querySelector(".side").addEventListener("click", (ev) => {
  const b = ev.target.closest("button");
  if (!b) return;
  if (b.dataset.cmd) command(b.dataset.cmd);
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
});

route();
