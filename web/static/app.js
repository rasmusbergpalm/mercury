"use strict";

// ---- api ---------------------------------------------------------------------
const api = {
  get: (p) => fetch(p).then((r) => { if (!r.ok) throw new Error(r.statusText); return r.json(); }),
  post: (p) => fetch(p, { method: "POST" }).then((r) => { if (!r.ok) throw new Error(r.statusText); return r.json(); }),
};

// ---- state -------------------------------------------------------------------
let world = null;      // static info
let state = null;      // latest dynamic state
let selected = null;   // selected region id
let playing = false;
let hover = null;

const $ = (id) => document.getElementById(id);
const canvas = $("map");
const ctx = canvas.getContext("2d");

const TERRAIN_COLORS = {
  plains: "#c8d67a", forest: "#4f8a4a", hills: "#b89c69", mountains: "#8e8e93",
  tropical: "#2fa37b", coast: "#6db3dc",
};
const TIER_COLORS = { basic: "#7a9e4f", intermediate: "#d4a23c", luxury: "#9b5fc0" };
// a perceptually ordered ramp (dark blue -> teal -> yellow)
const RAMP = ["#2a1b5c", "#26508f", "#1f8a8c", "#49b670", "#b5d84a", "#fde725"];

function rampColor(t) {
  t = Math.max(0, Math.min(1, t));
  const x = t * (RAMP.length - 1), i = Math.min(Math.floor(x), RAMP.length - 2), f = x - i;
  const a = hex(RAMP[i]), b = hex(RAMP[i + 1]);
  return `rgb(${a.map((v, k) => Math.round(v + (b[k] - v) * f)).join(",")})`;
}
function hex(c) { return [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16)); }

const fmt = (x, d = 2) => (Math.abs(x) >= 1000 ? x.toFixed(0) : x.toFixed(d));

// ---- geometry: pointy-top hexes, odd-r offset rows -----------------------------
let size = 40, margin = 10;
function center(r) {
  const w = Math.sqrt(3) * size;
  return [margin + w / 2 + w * (r.col + 0.5 * (r.row & 1)), margin + size + 1.5 * size * r.row];
}
function hexPath(x, y, s) {
  ctx.beginPath();
  for (let i = 0; i < 6; i++) {
    const a = Math.PI / 180 * (60 * i - 30);
    const px = x + s * Math.cos(a), py = y + s * Math.sin(a);
    i ? ctx.lineTo(px, py) : ctx.moveTo(px, py);
  }
  ctx.closePath();
}
function resize() {
  // fit the map to the pane's width and to what is left of the window's height
  const rows = world.rows, cols = world.cols;
  const availW = canvas.parentElement.clientWidth;
  const availH = Math.max(320, window.innerHeight - canvas.parentElement.getBoundingClientRect().top - 48);
  size = Math.min((availW - 2 * margin) / (Math.sqrt(3) * (cols + 0.5)),
                  (availH - 2 * margin) / (2 + 1.5 * (rows - 1)));
  const cssW = 2 * margin + Math.sqrt(3) * size * (cols + 0.5);
  const cssH = 2 * margin + size * (2 + 1.5 * (rows - 1));
  const dpr = window.devicePixelRatio || 1;
  canvas.width = cssW * dpr; canvas.height = cssH * dpr;
  canvas.style.width = cssW + "px"; canvas.style.height = cssH + "px";
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  draw();
}

// ---- overlays ----------------------------------------------------------------
function share(rs, tier) {
  const total = rs.tier_spend.reduce((a, b) => a + b, 0);
  return total > 0 ? rs.tier_spend[world.tiers.indexOf(tier)] / total : 0;
}
function overlayValues() {
  const kind = $("overlay").value, g = +$("good").value;
  return state.regions.map((rs) => {
    switch (kind) {
      case "utility": return rs.utility;
      case "wage": return rs.wage;
      case "price": return rs.prices[g] / rs.wage;
      case "basic": return share(rs, "basic");
      case "luxury": return share(rs, "luxury");
    }
    return null;
  });
}
function overlayLabel() {
  const kind = $("overlay").value;
  if (kind === "price") return `${world.goods[+$("good").value].name} price ÷ wage`;
  return $("overlay").selectedOptions[0].textContent;
}
const isShare = () => ["basic", "luxury"].includes($("overlay").value);
const useLog = () => $("overlay").value === "price";

function scale(vals) {
  const f = useLog() ? Math.log : (x) => x;
  const xs = vals.map(f);
  let lo = Math.min(...xs), hi = Math.max(...xs);
  if (hi - lo < 1e-9) { lo -= 1; hi += 1; }
  return { t: (v) => (f(v) - lo) / (hi - lo), lo: vals[xs.indexOf(Math.min(...xs))], hi: vals[xs.indexOf(Math.max(...xs))] };
}

// ---- drawing -----------------------------------------------------------------
function draw() {
  if (!world || !state) return;
  const css = getComputedStyle(document.documentElement);
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  const kind = $("overlay").value;
  const vals = kind === "terrain" ? null : overlayValues();
  const sc = vals && scale(vals);

  for (const r of world.regions) {
    const [x, y] = center(r);
    hexPath(x, y, size * 0.97);
    ctx.fillStyle = vals ? rampColor(sc.t(vals[r.id])) : TERRAIN_COLORS[r.terrain];
    ctx.fill();
    ctx.lineWidth = 1; ctx.strokeStyle = css.getPropertyValue("--hex-stroke"); ctx.stroke();
    // terrain pip, so terrain stays visible under data overlays
    if (vals) {
      ctx.beginPath(); ctx.arc(x, y - size * 0.55, size * 0.1, 0, 2 * Math.PI);
      ctx.fillStyle = TERRAIN_COLORS[r.terrain]; ctx.fill();
      ctx.strokeStyle = "rgba(0,0,0,0.4)"; ctx.stroke();
    }
    ctx.fillStyle = vals && sc.t(vals[r.id]) > 0.6 ? "#111" : (vals ? "#fff" : "#1b1b1b");
    ctx.font = `600 ${Math.max(10, size * 0.28)}px system-ui`;
    ctx.textAlign = "center"; ctx.textBaseline = "middle";
    const label = vals ? (isShare() ? (100 * vals[r.id]).toFixed(0) + "%" : fmt(vals[r.id], 1)) : r.terrain[0].toUpperCase();
    ctx.fillText(label, x, y + size * 0.05);
  }
  if ($("show-flows").checked) drawFlows(css);
  if (selected !== null) {
    const [x, y] = center(world.regions[selected]);
    hexPath(x, y, size * 0.93);
    ctx.lineWidth = 3; ctx.strokeStyle = css.getPropertyValue("--select"); ctx.stroke();
  }
  drawLegend(sc);
}

function drawFlows(css) {
  const fg = $("flow-good").value; // "all" or good index
  const agg = new Map();
  for (const f of state.flows) {
    if (fg !== "all" && f.good !== +fg) continue;
    // "all goods" is measured in value at the origin price, so units are comparable
    const v = fg === "all" ? f.qty * state.regions[f.from].prices[f.good] : f.qty;
    const k = f.from * 1000 + f.to;
    agg.set(k, (agg.get(k) || 0) + v);
  }
  const max = Math.max(1e-9, ...agg.values());
  ctx.strokeStyle = ctx.fillStyle = css.getPropertyValue("--flow");
  for (const [k, v] of agg) {
    if (v < max * 0.02) continue;
    const a = world.regions[Math.floor(k / 1000)], b = world.regions[k % 1000];
    const [x1, y1] = center(a), [x2, y2] = center(b);
    const dx = x2 - x1, dy = y2 - y1, len = Math.hypot(dx, dy), ux = dx / len, uy = dy / len;
    const off = size * 0.12, sx = -uy * off, sy = ux * off; // keep the two directions apart
    const w = 1 + 6 * Math.sqrt(v / max);
    const ax = x1 + ux * size * 0.35 + sx, ay = y1 + uy * size * 0.35 + sy;
    const bx = x2 - ux * size * 0.45 + sx, by = y2 - uy * size * 0.45 + sy;
    ctx.lineWidth = w; ctx.lineCap = "round";
    ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(bx, by); ctx.stroke();
    const h = 4 + w;
    ctx.beginPath(); ctx.moveTo(bx + ux * h, by + uy * h);
    ctx.lineTo(bx - uy * h * 0.6, by + ux * h * 0.6); ctx.lineTo(bx + uy * h * 0.6, by - ux * h * 0.6);
    ctx.closePath(); ctx.fill();
  }
}

function drawLegend(sc) {
  const el = $("legend");
  if (!sc) {
    el.innerHTML = world.terrains.map((t) =>
      `<span><span class="swatch" style="background:${TERRAIN_COLORS[t.name]}"></span>${t.name}${t.move_cost < 1 ? ` (move ×${t.move_cost})` : ""}</span>`).join("");
    return;
  }
  const f = (v) => (isShare() ? (100 * v).toFixed(0) + "%" : fmt(v));
  const grad = RAMP.join(",");
  el.innerHTML = `<span>${overlayLabel()}${useLog() ? " (log scale)" : ""}</span>
    <span>${f(sc.lo)}</span><span class="ramp" style="background:linear-gradient(90deg,${grad})"></span><span>${f(sc.hi)}</span>`;
}

// ---- side panel --------------------------------------------------------------
async function showRegion(id) {
  selected = id;
  draw();
  const d = await api.get(`/api/regions/${id}`);
  const G = world.goods, s = d.state;
  const total = s.tier_spend.reduce((a, b) => a + b, 0) || 1;
  const bar = world.tiers.map((t, i) =>
    `<span style="width:${(100 * s.tier_spend[i] / total).toFixed(1)}%;background:${TIER_COLORS[t]}" title="${t} ${(100 * s.tier_spend[i] / total).toFixed(1)}%"></span>`).join("");
  const firms = d.firms.filter((f) => f.labor > 0.05 || f.produced > 0.05)
    .sort((a, b) => b.labor - a.labor)
    .map((f) => `<tr><td>${f.name}</td><td>${G[f.output].name}</td><td>${fmt(f.labor, 1)}</td><td>${fmt(f.produced)}</td></tr>`).join("");
  const markets = d.markets.filter((m) => m.supply > 1e-3 || m.demand > 1e-3)
    .map((m) => `<tr class="${G[m.good].tier ? "" : "dim"}"><td>${G[m.good].name}</td><td>${fmt(m.price)}</td><td>${fmt(m.price / s.wage)}</td><td>${fmt(m.supply)}</td><td>${fmt(m.demand)}</td><td>${m.consumed_per_person > 0 ? fmt(m.consumed_per_person, 3) : ""}</td></tr>`).join("");
  const trade = (list, dir) => {
    const rows = list.sort((a, b) => b.qty - a.qty).slice(0, 10).map((f) => {
      const other = world.regions[dir === "out" ? f.to : f.from];
      return `<tr><td>${G[f.good].name}</td><td>${dir === "out" ? "→" : "←"} ${other.name} (${other.terrain})</td><td>${fmt(f.qty)}</td></tr>`;
    }).join("");
    return rows || `<tr class="dim"><td colspan="3">none</td></tr>`;
  };
  $("panel").innerHTML = `
    <h2>Region ${d.info.name}</h2>
    <p class="sub">${d.info.terrain} · row ${d.info.row}, col ${d.info.col} · tick ${state.tick}</p>
    <div class="stats">
      <div class="stat"><div class="v">${fmt(s.utility)}</div><div class="k">utility / person</div></div>
      <div class="stat"><div class="v">${fmt(s.wage)}</div><div class="k">wage</div></div>
      <div class="stat"><div class="v">${fmt(s.employed, 0)}/${fmt(s.population, 0)}</div><div class="k">employed</div></div>
    </div>
    <h3>Spending by need tier</h3>
    <div class="bar">${bar}</div>
    <p class="sub">${world.tiers.map((t, i) => `${t} ${(100 * s.tier_spend[i] / total).toFixed(0)}%`).join(" · ")}</p>
    <h3>Firms</h3>
    <table><tr><th>firm</th><th>makes</th><th>workers</th><th>output</th></tr>${firms}</table>
    <h3>Markets</h3>
    <table><tr><th>good</th><th>price</th><th>÷ wage</th><th>supply</th><th>demand</th><th>eaten/p</th></tr>${markets}</table>
    <h3>Exports</h3><table><tr><th>good</th><th>to</th><th>units</th></tr>${trade(d.exports, "out")}</table>
    <h3>Imports</h3><table><tr><th>good</th><th>from</th><th>units</th></tr>${trade(d.imports, "in")}</table>`;
}

// ---- interaction -------------------------------------------------------------
function regionAt(ev) {
  const rect = canvas.getBoundingClientRect();
  const x = ev.clientX - rect.left, y = ev.clientY - rect.top;
  let best = null, bd = Infinity;
  for (const r of world.regions) {
    const [cx, cy] = center(r), d = Math.hypot(cx - x, cy - y);
    if (d < bd) { bd = d; best = r; }
  }
  return bd < size ? best : null;
}
canvas.addEventListener("click", (ev) => { const r = regionAt(ev); if (r) showRegion(r.id); });
canvas.addEventListener("mousemove", (ev) => {
  const r = regionAt(ev), tip = $("tooltip");
  hover = r;
  if (!r) { tip.hidden = true; return; }
  const kind = $("overlay").value;
  let v = "";
  if (kind !== "terrain") {
    const x = overlayValues()[r.id];
    v = ` · ${overlayLabel()}: ${isShare() ? (100 * x).toFixed(1) + "%" : fmt(x)}`;
  }
  tip.textContent = `${r.name} ${r.terrain}${v}`;
  const rect = canvas.getBoundingClientRect();
  tip.style.left = ev.clientX - rect.left + 12 + "px"; tip.style.top = ev.clientY - rect.top + 12 + "px";
  tip.hidden = false;
});
canvas.addEventListener("mouseleave", () => { $("tooltip").hidden = true; });

function setState(s) {
  state = s;
  $("tick").textContent = s.tick;
  draw();
  if (selected !== null && !playing) showRegion(selected);
}
async function step(n) { setState(await api.post(`/api/step?n=${n}`)); }

async function loop() {
  while (playing) {
    await step(+$("speed").value);
    if (selected !== null) await showRegion(selected);
    await new Promise((r) => requestAnimationFrame(r));
  }
}
$("step1").onclick = () => step(1);
$("step10").onclick = () => step(10);
$("step100").onclick = () => step(100);
$("play").onclick = () => {
  playing = !playing;
  $("play").textContent = playing ? "Pause" : "Play";
  if (playing) loop();
};
$("reset").onclick = async () => {
  const c = $("cost").value.trim();
  try { setState(await api.post(`/api/reset?cost=${encodeURIComponent(c)}`)); }
  catch (e) { alert("Transport cost must be a non-negative number or inf"); }
};
for (const id of ["overlay", "good", "show-flows", "flow-good"]) $(id).addEventListener("change", () => {
  $("good-wrap").hidden = $("overlay").value !== "price";
  $("flow-good-wrap").hidden = !$("show-flows").checked;
  draw();
});
window.addEventListener("resize", () => world && resize());
window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", draw);

// ---- boot --------------------------------------------------------------------
(async function boot() {
  world = await api.get("/api/world");
  const opts = world.goods.map((g, i) => `<option value="${i}">${g.name}${g.tier ? "" : " (input)"}</option>`).join("");
  $("good").innerHTML = opts;
  $("good").value = String(world.goods.findIndex((g) => g.name === "bread"));
  $("flow-good").innerHTML = `<option value="all">all goods (by value)</option>` + opts;
  state = await api.get("/api/state");
  $("tick").textContent = state.tick;
  $("cost").value = state.autarky ? "inf" : state.transport_cost;
  resize();
  if (state.tick === 0) await step(200); // start from a settled economy
})();
