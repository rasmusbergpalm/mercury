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
      case "population": return rs.population;
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
const CATEGORICAL = ["terrain", "nation", "infra"];
const INFRA_COLORS = ["#d9d4c7", "#b07d48", "#5b3a1e"];
const owner = (r) => state.regions[r.id].nation; // ownership changes when tiles are annexed
const nationColor = (r) => (owner(r) >= 0 ? world.nations[owner(r)].color : "#9a9a9a");
function categoryColor(kind, r) {
  if (kind === "nation") return nationColor(r);
  if (kind === "infra") return INFRA_COLORS[state.regions[r.id].infra];
  return TERRAIN_COLORS[r.terrain];
}
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
  const vals = CATEGORICAL.includes(kind) ? null : overlayValues();
  const sc = vals && scale(vals);

  for (const r of world.regions) {
    const [x, y] = center(r);
    hexPath(x, y, size * 0.97);
    ctx.fillStyle = vals ? rampColor(sc.t(vals[r.id])) : categoryColor(kind, r);
    ctx.fill();
    ctx.lineWidth = 1; ctx.strokeStyle = css.getPropertyValue("--hex-stroke"); ctx.stroke();
    // terrain pip, so terrain stays visible under data overlays
    if (vals) {
      ctx.beginPath(); ctx.arc(x, y - size * 0.55, size * 0.1, 0, 2 * Math.PI);
      ctx.fillStyle = TERRAIN_COLORS[r.terrain]; ctx.fill();
      ctx.strokeStyle = "rgba(0,0,0,0.4)"; ctx.stroke();
    }
    const rs = state.regions[r.id];
    const dark = vals ? sc.t(vals[r.id]) <= 0.6 : (kind === "nation" || (kind === "infra" && rs.infra === 2));
    ctx.fillStyle = dark ? "#fff" : "#1b1b1b";
    ctx.font = `600 ${Math.max(10, size * 0.28)}px system-ui`;
    ctx.textAlign = "center"; ctx.textBaseline = "middle";
    let label;
    if (vals) label = isShare() ? (100 * vals[r.id]).toFixed(0) + "%" : fmt(vals[r.id], 1);
    else if (kind === "nation") label = owner(r) >= 0 ? world.nations[owner(r)].name.slice(0, 3) : "–";
    else if (kind === "infra") label = "×" + rs.move_cost.toFixed(2);
    else label = r.terrain[0].toUpperCase();
    ctx.fillText(label, x, y + size * 0.05);
    drawInfra(r, rs, x, y);
  }
  drawBorders();
  if ($("show-flows").checked) drawFlows(css);
  if (selected !== null) {
    const [x, y] = center(world.regions[selected]);
    hexPath(x, y, size * 0.93);
    ctx.lineWidth = 3; ctx.strokeStyle = css.getPropertyValue("--select"); ctx.stroke();
  }
  drawLegend(sc);
}

// roads: a small bar under the label, one segment per level; projects: a progress arc
function drawInfra(r, rs, x, y) {
  const by = y + size * 0.42;
  for (let i = 0; i < rs.infra; i++) {
    ctx.fillStyle = INFRA_COLORS[2]; ctx.strokeStyle = "#fff"; ctx.lineWidth = 1;
    ctx.fillRect(x - size * 0.25 + i * size * 0.27, by, size * 0.22, size * 0.08);
    ctx.strokeRect(x - size * 0.25 + i * size * 0.27, by, size * 0.22, size * 0.08);
  }
  if (rs.project) {
    const f = rs.project.delivered / rs.project.required;
    ctx.beginPath(); ctx.arc(x + size * 0.45, y - size * 0.45, size * 0.12, -Math.PI / 2, -Math.PI / 2 + 2 * Math.PI * f);
    ctx.lineWidth = size * 0.06; ctx.strokeStyle = "#e8590c"; ctx.stroke();
    ctx.beginPath(); ctx.arc(x + size * 0.45, y - size * 0.45, size * 0.12, 0, 2 * Math.PI);
    ctx.lineWidth = 1; ctx.strokeStyle = "rgba(0,0,0,0.4)"; ctx.stroke();
  }
}

// borders: the shared hex edge between tiles of different nations
function drawBorders() {
  for (const l of world.links) {
    const a = world.regions[l.a], b = world.regions[l.b];
    if (owner(a) === owner(b)) continue;
    const [x1, y1] = center(a), [x2, y2] = center(b);
    const mx = (x1 + x2) / 2, my = (y1 + y2) / 2, len = Math.hypot(x2 - x1, y2 - y1);
    const px = -(y2 - y1) / len * size / 2, py = (x2 - x1) / len * size / 2;
    ctx.lineCap = "round";
    ctx.lineWidth = 5; ctx.strokeStyle = "rgba(255,255,255,0.85)";
    ctx.beginPath(); ctx.moveTo(mx + px, my + py); ctx.lineTo(mx - px, my - py); ctx.stroke();
    ctx.lineWidth = 2.5; ctx.strokeStyle = "#222";
    ctx.beginPath(); ctx.moveTo(mx + px, my + py); ctx.lineTo(mx - px, my - py); ctx.stroke();
  }
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
  const kind = $("overlay").value;
  if (!sc && kind === "nation") {
    el.innerHTML = world.nations.map((n) => `<span><span class="swatch" style="background:${n.color}"></span>${n.name}</span>`).join("");
    return;
  }
  if (!sc && kind === "infra") {
    el.innerHTML = world.infra_levels.map((l, i) =>
      `<span><span class="swatch" style="background:${INFRA_COLORS[i]}"></span>${l.name} (move ×${l.move_mult})</span>`).join("") +
      `<span>· labels show the tile's movement cost</span>`;
    return;
  }
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
  const firms = d.firms.map((f, i) => ({ ...f, i })).filter((f) => f.labor > 0.05 || f.produced > 0.05 || f.levels > 1)
    .sort((a, b) => b.labor - a.labor)
    .map((f) => {
      let grow = "";
      if (f.expansion) grow = `${(100 * f.expansion.delivered / world.level_cost).toFixed(0)}%${f.expansion.public ? " (state)" : ""}`;
      else if (s.nation >= 0 && f.levels < f.max_levels) grow = `<button class="expand" data-firm="${f.i}" title="Nation pays ${world.level_cost} construction for one more level">+1</button>`;
      return `<tr><td>${f.name}</td><td>${G[f.output].name}</td><td>${f.levels}/${f.max_levels}</td><td>${fmt(f.labor, 1)}</td><td>${fmt(f.produced)}</td><td>${fmt(f.profit, 1)}</td><td>${grow}</td></tr>`;
    }).join("");
  const markets = d.markets.filter((m) => m.supply > 1e-3 || m.demand > 1e-3)
    .map((m) => `<tr class="${G[m.good].tier ? "" : "dim"}"><td>${G[m.good].name}</td><td>${fmt(m.price)}</td><td>${fmt(m.price / s.wage)}</td><td>${fmt(m.supply)}</td><td>${fmt(m.demand)}</td><td>${m.consumed_per_person > 0 ? fmt(m.consumed_per_person, 3) : ""}</td></tr>`).join("");
  const trade = (list, dir) => {
    const rows = list.sort((a, b) => b.qty - a.qty).slice(0, 10).map((f) => {
      const other = world.regions[dir === "out" ? f.to : f.from];
      return `<tr><td>${G[f.good].name}</td><td>${dir === "out" ? "→" : "←"} ${other.name} (${other.terrain})</td><td>${fmt(f.qty)}</td></tr>`;
    }).join("");
    return rows || `<tr class="dim"><td colspan="3">none</td></tr>`;
  };
  const nation = s.nation >= 0 ? world.nations[s.nation] : null;
  const ns = nation && state.nations[nation.id];
  const lvl = world.infra_levels[s.infra], next = world.infra_levels[s.infra + 1];
  const cPrice = d.markets[world.goods.findIndex((g) => g.name === "construction")].price;
  let infra = `<p class="sub">${lvl.name} · moving through costs ×${s.move_cost.toFixed(2)}${lvl.upkeep ? ` · upkeep ${lvl.upkeep} construction/tick` : ""}</p>`;
  if (s.project) {
    const f = s.project.delivered / s.project.required;
    infra += `<div class="progress"><span style="width:${(100 * f).toFixed(1)}%"></span></div>
      <div class="row"><span class="sub">Building ${world.infra_levels[s.project.target].name}: ${fmt(s.project.delivered, 0)} / ${s.project.required} construction</span>
      <button id="cancel">Cancel</button></div>`;
  } else if (next && nation) {
    const mats = Object.entries(next.materials || {}).map(([g, q]) => ` + ${q} ${g}`).join("");
    const locked = next.tech && !state.nations[nation.id].known[world.techs.findIndex((t) => t.name === next.tech)];
    infra += locked
      ? `<p class="sub">${next.name} needs ${next.tech}.</p>`
      : `<div class="row"><button id="build" class="primary">Build ${next.name}</button>
      <span class="sub">${next.cost} construction${mats} (≈ ${fmt(next.cost * cPrice, 0)} at today's construction price ${fmt(cPrice)})</span></div>`;
  }
  infra += `<div id="act-err" class="err"></div>`;
  const nationHtml = nation ? nationPanel(nation, ns) : annexPanel(d);
  $("panel").innerHTML = `
    <h2>Region ${d.info.name}</h2>
    <p class="sub">${d.info.terrain} · ${nation ? nation.name : "unowned"} · row ${d.info.row}, col ${d.info.col} · tick ${state.tick}</p>
    <div class="stats">
      <div class="stat"><div class="v">${fmt(s.utility)}</div><div class="k">utility / person</div></div>
      <div class="stat"><div class="v">${fmt(s.wage)}</div><div class="k">wage</div></div>
      <div class="stat"><div class="v">${fmt(s.employed, 0)}/${fmt(s.population, 0)}</div><div class="k">employed</div></div>
    </div>
    <h3>Spending by need tier</h3>
    <div class="bar">${bar}</div>
    <p class="sub">${world.tiers.map((t, i) => `${t} ${(100 * s.tier_spend[i] / total).toFixed(0)}%`).join(" · ")}</p>
    ${nationHtml}
    <h3>Infrastructure</h3>${infra}
    <h3>Firms</h3>
    <table><tr><th>firm</th><th>makes</th><th>levels</th><th>workers</th><th>output</th><th>profit</th><th>grow</th></tr>${firms}</table>
    <h3>Markets</h3>
    <table><tr><th>good</th><th>price</th><th>÷ wage</th><th>supply</th><th>demand</th><th>eaten/p</th></tr>${markets}</table>
    <h3>Exports</h3><table><tr><th>good</th><th>to</th><th>units</th></tr>${trade(d.exports, "out")}</table>
    <h3>Imports</h3><table><tr><th>good</th><th>from</th><th>units</th></tr>${trade(d.imports, "in")}</table>`;
  wirePanel(id, nation);
}

function nationPanel(nation, ns) {
  const others = world.nations.filter((o) => o.id !== nation.id).map((o) =>
    `<label><input type="checkbox" class="embargo" data-target="${o.id}" ${ns.embargo[o.id] ? "checked" : ""}> ${o.name}</label>`).join(" ");
  const tradable = world.goods.map((g, i) => i).filter((i) => !["transport", "construction", "research"].includes(world.goods[i].name));
  const tariff = Math.max(...tradable.map((i) => ns.tariff[i]));
  const techs = world.techs.map((t, i) => {
    let status;
    if (ns.known[i]) status = "✓ known";
    else if (ns.researching === i) status = `researching ${(100 * Math.min(1, ns.research_points / t.cost)).toFixed(0)}%`;
    else if (t.requires.every((r) => ns.known[world.techs.findIndex((x) => x.name === r)])) status = `<button class="research" data-tech="${i}">Research</button>`;
    else status = `needs ${t.requires.join(", ")}`;
    return `<tr class="${ns.known[i] ? "dim" : ""}"><td>${t.name}</td><td title="${t.effect}">${t.effect}</td><td>${t.cost}</td><td>${status}</td></tr>`;
  }).join("");
  return `
    <h3>Nation</h3>
    <p><span class="chip" style="background:${nation.color}"></span><b>${nation.name}</b> · ${fmt(ns.population, 0)} people · treasury ${fmt(ns.treasury, 0)}</p>
    <div class="row"><label>Tax <input id="tax" type="number" min="0" max="90" step="1" value="${(100 * ns.tax_rate).toFixed(0)}" style="width:60px">%</label>
      <button id="set-tax">Set</button>
      <label>Import tariff <input id="tariff" type="number" min="0" max="500" step="5" value="${(100 * tariff).toFixed(0)}" style="width:60px">%</label>
      <button id="set-tariff">Set</button></div>
    <div class="row"><span class="sub">Embargo:</span> ${others}</div>
    <p class="sub">tax ${fmt(ns.revenue, 1)} + tariffs ${fmt(ns.tariff_revenue, 1)}/tick · spending ${fmt(ns.spending, 1)}/tick · rebate ${fmt(ns.rebate, 1)}/tick</p>
    <h3>Research</h3>
    <div class="row"><label>Funding <input id="funding" type="number" min="0" step="10" value="${fmt(ns.research_funding, 0)}" style="width:70px">/tick</label>
      <button id="set-funding">Set</button></div>
    <table><tr><th>tech</th><th>effect</th><th>cost</th><th></th></tr>${techs}</table>`;
}

function annexPanel(d) {
  const nbs = world.links.filter((l) => l.a === d.info.id || l.b === d.info.id).map((l) => (l.a === d.info.id ? l.b : l.a));
  const ids = [...new Set(nbs.map((id) => state.regions[id].nation).filter((n) => n >= 0))];
  const cost = d.state.annex_cost;
  const buttons = ids.map((n) => {
    const ns = state.nations[n];
    return `<button class="annex" data-nation="${n}" ${ns.treasury < cost ? "disabled" : ""}>Annex for ${world.nations[n].name}</button>`;
  }).join(" ");
  return `<h3>Unowned</h3>
    <p class="sub">${ids.length ? `Annexing costs ${fmt(cost, 0)} (${fmt(d.state.population, 0)} people), paid to them.` : "No nation borders this tile yet."}</p>
    <div class="row">${buttons}</div>`;
}

async function command(path) {
  try {
    const r = await fetch(path, { method: "POST" });
    if (!r.ok) throw new Error((await r.text()).trim());
    setState(await r.json());
    if (selected !== null) await showRegion(selected);
  } catch (e) {
    const el = $("act-err");
    if (el) el.textContent = e.message;
  }
}
function wirePanel(id, nation) {
  const panel = $("panel");
  if ($("build")) $("build").onclick = () => command(`/api/regions/${id}/build`);
  if ($("cancel")) $("cancel").onclick = () => command(`/api/regions/${id}/cancel`);
  for (const b of panel.querySelectorAll("button.expand")) b.onclick = () => command(`/api/regions/${id}/firms/${b.dataset.firm}/expand`);
  for (const b of panel.querySelectorAll("button.annex")) b.onclick = () => command(`/api/regions/${id}/annex?nation=${b.dataset.nation}`);
  if (!nation) return;
  const n = nation.id;
  $("set-tax").onclick = () => command(`/api/nations/${n}/tax?rate=${(+$("tax").value / 100).toFixed(3)}`);
  $("set-tariff").onclick = () => command(`/api/nations/${n}/tariff?rate=${(+$("tariff").value / 100).toFixed(3)}`);
  $("set-funding").onclick = () => command(`/api/nations/${n}/funding?amount=${+$("funding").value}`);
  for (const c of panel.querySelectorAll("input.embargo")) c.onchange = () => command(`/api/nations/${n}/embargo?target=${c.dataset.target}&on=${c.checked}`);
  for (const b of panel.querySelectorAll("button.research")) b.onclick = () => command(`/api/nations/${n}/research?tech=${b.dataset.tech}`);
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
  if (kind === "nation" || kind === "infra") {
    const rs = state.regions[r.id];
    v = ` · ${owner(r) >= 0 ? world.nations[owner(r)].name : "unowned"} · ${world.infra_levels[rs.infra].name}`;
  } else if (kind !== "terrain") {
    const x = overlayValues()[r.id];
    v = ` · ${overlayLabel()}: ${isShare() ? (100 * x).toFixed(1) + "%" : fmt(x)}`;
  }
  tip.textContent = `${r.name} ${r.terrain}${v}`;
  const rect = canvas.getBoundingClientRect();
  tip.style.left = ev.clientX - rect.left + 12 + "px"; tip.style.top = ev.clientY - rect.top + 12 + "px";
  tip.hidden = false;
});
canvas.addEventListener("mouseleave", () => { $("tooltip").hidden = true; });

function drawNations() {
  const rows = world.nations.map((n) => {
    const ns = state.nations[n.id];
    const tiles = world.regions.filter((r) => owner(r) === n.id);
    const u = tiles.reduce((a, r) => a + state.regions[r.id].utility * state.regions[r.id].population, 0) / ns.population;
    const roads = tiles.reduce((a, r) => a + state.regions[r.id].infra, 0);
    const known = ns.known.filter(Boolean).length;
    const res = ns.researching >= 0
      ? `${world.techs[ns.researching].name} ${(100 * Math.min(1, ns.research_points / world.techs[ns.researching].cost)).toFixed(0)}%`
      : (known === world.techs.length ? "all known" : "—");
    const tariffs = ns.tariff.filter((x) => x > 0);
    const tariff = tariffs.length ? `${(100 * Math.max(...tariffs)).toFixed(0)}%` : "0%";
    return `<tr><td><span class="chip" style="background:${n.color}"></span>${n.name}</td><td>${tiles.length}</td>
      <td>${fmt(ns.population, 0)}</td><td>${fmt(u)}</td><td>${(100 * ns.tax_rate).toFixed(0)}%</td><td>${tariff}</td>
      <td>${fmt(ns.treasury, 0)}</td><td>${fmt(ns.revenue + ns.tariff_revenue, 1)}</td><td>${fmt(ns.spending, 1)}</td>
      <td>${roads}</td><td>${known}/${world.techs.length}</td><td>${res}</td></tr>`;
  }).join("");
  $("nations").innerHTML = `<tr><th>nation</th><th>tiles</th><th>people</th><th>utility</th><th>tax</th><th>tariff</th>
    <th>treasury</th><th>income/tick</th><th>spending/tick</th><th>road levels</th><th>techs</th><th>researching</th></tr>${rows}`;
}

function setState(s) {
  state = s;
  $("tick").textContent = s.tick;
  draw();
  drawNations();
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
