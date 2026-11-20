const D = JSON.parse(document.getElementById("data").textContent);
const css = (v) => getComputedStyle(document.documentElement).getPropertyValue(v).trim();
const NS = "http://www.w3.org/2000/svg";
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const fmt = (v, d = 1) => (Math.abs(v) >= 100 ? v.toFixed(0) : v.toFixed(d));
const tiles = D.map.tiles;
const byId = Object.fromEntries(tiles.map((t) => [t.id, t]));
const tileData = Object.fromEntries(D.tiles.map((t) => [t.id, t]));
const SERIES = ["--s1", "--s2", "--s3", "--s4", "--s5", "--s6"];
const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);

// ---- hex maps -------------------------------------------------------------
// the classic map is 6x6, odd rows shifted half a hex right
const R = 30, W = Math.sqrt(3) * R;
function hexXY(t) { return [W * (t.col + 0.5 + 0.5 * (t.row % 2)) + 6, R * (1 + 1.5 * t.row) + 6]; }
function hexPts(x, y, r = R) { return [...Array(6)].map((_, i) => { const a = Math.PI / 180 * (60 * i - 30); return `${(x + r * Math.cos(a)).toFixed(1)},${(y + r * Math.sin(a)).toFixed(1)}`; }).join(" "); }
function hexMap(svg, { fill, label = (t) => t.name, arrows = [], selected = null, onPick = null, path = null, edges = [] }) {
  const w = W * 6.5 + 12, h = R * (2 + 1.5 * 5) + 12;
  svg.setAttribute("viewBox", `0 0 ${w} ${h}`);
  let s = `<rect x="0" y="0" width="${w}" height="${h}" fill="${css("--sea")}"/>`;
  s += `<defs><marker id="ah-${svg.id}" viewBox="0 0 10 10" refX="7" refY="5" markerWidth="4" markerHeight="4" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="${css("--ink")}"/></marker></defs><g class="hexes">`;
  for (const t of tiles) {
    const [x, y] = hexXY(t);
    s += `<polygon data-id="${t.id}" points="${hexPts(x, y)}" fill="${fill(t)}" stroke="${t.id === selected ? css("--accent") : css("--hex")}" stroke-width="${t.id === selected ? 3 : 1}" ${onPick ? 'style="cursor:pointer" tabindex="0" role="button" aria-label="' + esc(t.name) + '"' : ""}/>`;
  }
  s += "</g>";
  // undirected links between tiles' centres, [a, b, width]
  for (const [a, b, sw] of edges) {
    const [x1, y1] = hexXY(byId[a]), [x2, y2] = hexXY(byId[b]);
    s += `<line x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}" stroke="${css("--s1")}" stroke-opacity=".8" stroke-width="${sw.toFixed(2)}" stroke-linecap="round"/>`;
  }
  // a route, drawn through the centres of its tiles
  if (path) s += `<polyline points="${path.map((id) => hexXY(byId[id]).join(",")).join(" ")}" fill="none" stroke="${css("--ink")}" stroke-opacity=".4" stroke-width="2.5" stroke-dasharray="7 5" stroke-linecap="round"/>`;
  for (const [a, b, q] of arrows) {
    const [x1, y1] = hexXY(byId[a]), [x2, y2] = hexXY(byId[b]);
    const dx = x2 - x1, dy = y2 - y1, L = Math.hypot(dx, dy), sh = 0.28 * R;
    const sw = q.w;
    s += `<line x1="${(x1 + dx / L * sh).toFixed(1)}" y1="${(y1 + dy / L * sh).toFixed(1)}" x2="${(x2 - dx / L * sh).toFixed(1)}" y2="${(y2 - dy / L * sh).toFixed(1)}" stroke="${css("--ink")}" stroke-opacity=".75" stroke-width="${sw.toFixed(2)}" marker-end="url(#ah-${svg.id})" stroke-linecap="round"/>`;
  }
  for (const t of tiles) {
    const [x, y] = hexXY(t), l = label(t);
    if (l) s += `<text x="${x}" y="${y + 4}" text-anchor="middle" style="font-size:9.5px;fill:${css("--ink")};pointer-events:none">${esc(l)}</text>`;
  }
  svg.innerHTML = s;
  if (onPick) svg.querySelectorAll("polygon").forEach((p) => {
    p.addEventListener("click", () => onPick(+p.dataset.id));
    p.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onPick(+p.dataset.id); } });
  });
}
// a single warm ramp, light to dark (or the reverse in the dark theme)
function ramp(t) {
  t = Math.max(0, Math.min(1, t));
  const dark = matchMedia("(prefers-color-scheme: dark)").matches && document.documentElement.dataset.theme !== "light" || document.documentElement.dataset.theme === "dark";
  const stops = dark ? [[58, 46, 34], [120, 84, 40], [190, 130, 60], [235, 190, 110]] : [[246, 236, 208], [228, 196, 128], [190, 120, 60], [125, 42, 23]];
  const x = t * (stops.length - 1), i = Math.min(stops.length - 2, Math.floor(x)), f = x - i;
  const c = stops[i].map((v, k) => Math.round(v + (stops[i + 1][k] - v) * f));
  return `rgb(${c.join(",")})`;
}
// a diverging scale: blue for a gain, red for a loss, paper for none.
// gamma below 1 lifts small changes out of the paper colour
function diverge(v, maxAbs, gamma = 1) {
  const t = Math.max(-1, Math.min(1, v / maxAbs));
  const base = css("--paper");
  const col = t >= 0 ? css("--s2") : css("--accent");
  return mix(base, col, 0.85 * Math.abs(t) ** gamma);
}
function mix(a, b, t) {
  const p = (c) => { const d = document.createElement("div"); d.style.color = c; document.body.appendChild(d); const v = getComputedStyle(d).color.match(/\d+/g).map(Number); d.remove(); return v; };
  const x = p(a), y = p(b);
  return `rgb(${x.map((v, i) => Math.round(v + (y[i] - v) * t)).join(",")})`;
}

// ---- charts -----------------------------------------------------------------
// lineChart draws series of [x, y] points with a log or linear x axis.
// right, if given, is a second set of series on their own y axis, at the
// right ({ series, yMin, yMax, yFmt, yLabel }).
function lineChart(svg, { series, xLog = false, xRev = false, xFmt = (v) => v, xTicks, xLabel, yLabel, yFmt = (v) => fmt(v), title = "", marks = [], w = 520, h = 300, yMin, yMax, points = [], right }) {
  const m = { l: 54, r: right ? (right.yLabel ? 62 : 48) : 14, t: title ? 30 : 12, b: 44 };
  const xs = series.flatMap((s) => s.pts.map((p) => p[0])).concat(points.map((p) => p.x));
  const ys = series.flatMap((s) => s.pts.map((p) => p[1])).concat(points.map((p) => p.y));
  const fx = xLog ? Math.log10 : (v) => v;
  let x0 = Math.min(...xs.map(fx)), x1 = Math.max(...xs.map(fx));
  let y0 = yMin ?? Math.min(...ys), y1 = yMax ?? Math.max(...ys);
  const pad = (y1 - y0) * 0.06 || 1; if (yMin === undefined) y0 -= pad; if (yMax === undefined) y1 += pad;
  const X = (v) => m.l + (xRev ? x1 - fx(v) : fx(v) - x0) / (x1 - x0) * (w - m.l - m.r);
  const Y = (v) => h - m.b - (v - y0) / (y1 - y0) * (h - m.t - m.b);
  let Y2 = Y, r0, r1;
  if (right) {
    const rs = right.series.flatMap((s) => s.pts.map((p) => p[1]));
    r0 = right.yMin ?? Math.min(...rs); r1 = right.yMax ?? Math.max(...rs);
    const rp = (r1 - r0) * 0.06 || 1; if (right.yMin === undefined) r0 -= rp; if (right.yMax === undefined) r1 += rp;
    Y2 = (v) => h - m.b - (v - r0) / (r1 - r0) * (h - m.t - m.b);
  }
  svg.setAttribute("viewBox", `0 0 ${w} ${h}`);
  let s = "";
  if (title) s += `<text class="title" x="${m.l}" y="18">${esc(title)}</text>`;
  const yt = niceTicks(y0, y1, 5);
  for (const v of yt) s += `<line class="grid" x1="${m.l}" x2="${w - m.r}" y1="${Y(v)}" y2="${Y(v)}"/><text x="${m.l - 6}" y="${Y(v) + 4}" text-anchor="end">${yFmt(v)}</text>`;
  for (const v of xTicks) { const x = X(v); if (x < m.l - 1 || x > w - m.r + 1) continue; s += `<line class="axis" x1="${x}" x2="${x}" y1="${h - m.b}" y2="${h - m.b + 4}"/><text x="${x}" y="${h - m.b + 18}" text-anchor="middle">${xFmt(v)}</text>`; }
  s += `<line class="axis" x1="${m.l}" x2="${w - m.r}" y1="${h - m.b}" y2="${h - m.b}"/>`;
  if (xLabel) s += `<text x="${(m.l + w - m.r) / 2}" y="${h - 6}" text-anchor="middle">${esc(xLabel)}</text>`;
  if (yLabel) s += `<text x="12" y="${(m.t + h - m.b) / 2}" text-anchor="middle" transform="rotate(-90 12 ${(m.t + h - m.b) / 2})">${esc(yLabel)}</text>`;
  if (right) {
    const rf = right.yFmt || ((v) => fmt(v));
    for (const v of niceTicks(r0, r1, 5)) s += `<line class="axis" x1="${w - m.r}" x2="${w - m.r + 4}" y1="${Y2(v)}" y2="${Y2(v)}"/><text x="${w - m.r + 7}" y="${Y2(v) + 4}">${rf(v)}</text>`;
    s += `<line class="axis" x1="${w - m.r}" x2="${w - m.r}" y1="${m.t}" y2="${h - m.b}"/>`;
    if (right.yLabel) { const x = w - 10, y = (m.t + h - m.b) / 2; s += `<text x="${x}" y="${y}" text-anchor="middle" transform="rotate(90 ${x} ${y})">${esc(right.yLabel)}</text>`; }
  }
  for (const mk of marks) { const x = X(mk.x); s += `<line x1="${x}" x2="${x}" y1="${m.t}" y2="${h - m.b}" stroke="${css("--muted")}" stroke-dasharray="4 4"/>${mk.label ? `<text x="${x + 5}" y="${m.t + 12}">${esc(mk.label)}</text>` : ""}`; }
  // lines are clipped to the plot (a series may run off a fixed scale)
  const clip = `clip-${svg.id || Math.random().toString(36).slice(2)}`;
  s += `<clipPath id="${clip}"><rect x="${m.l}" y="${m.t}" width="${w - m.l - m.r}" height="${h - m.t - m.b}"/></clipPath><g clip-path="url(#${clip})">`;
  const all = series.map((se) => [se, Y]).concat(right ? right.series.map((se) => [se, Y2]) : []);
  for (const [se, Yf] of all) {
    const d = se.pts.map((p, i) => `${i ? "L" : "M"}${X(p[0]).toFixed(1)},${Yf(p[1]).toFixed(1)}`).join("");
    s += `<path d="${d}" fill="none" stroke="${css(se.color)}" stroke-width="${se.width || 2.4}" ${se.dash ? `stroke-dasharray="${se.dash}"` : ""} stroke-linejoin="round" stroke-linecap="round"/>`;
  }
  s += "</g>";
  for (const [se, Yf] of all) {
    if (se.end) { const p = se.pts[se.pts.length - 1]; s += `<circle cx="${X(p[0])}" cy="${Yf(p[1])}" r="3.5" fill="${css(se.color)}"/><text x="${X(p[0]) - 6}" y="${Yf(p[1]) - 8}" text-anchor="end" style="fill:${css(se.color)}" ${se.below ? `dy="26"` : ""}>${esc(se.end)}</text>`; }
  }
  for (const p of points) s += `<circle cx="${X(p.x)}" cy="${Y(p.y)}" r="4.5" fill="${css(p.color)}" stroke="${css("--paper")}" stroke-width="1.5"/>${p.label ? `<text x="${X(p.x) - 8}" y="${Y(p.y) + (p.below ? 18 : -9)}" text-anchor="end">${esc(p.label)}</text>` : ""}`;
  svg.innerHTML = s;
}
function niceTicks(a, b, n) {
  const span = b - a, step0 = span / n, mag = Math.pow(10, Math.floor(Math.log10(step0)));
  const step = [1, 2, 2.5, 5, 10].map((k) => k * mag).find((k) => span / k <= n) || 10 * mag;
  const out = []; for (let v = Math.ceil(a / step) * step; v <= b + 1e-9; v += step) out.push(+v.toFixed(10));
  return out;
}
function legend(el, items) { el.innerHTML = items.map(([c, l, dash]) => `<span><i style="background:${dash ? "transparent" : css(c)};border-color:${css(c)};${dash ? "border-style:dashed" : ""}"></i>${esc(l)}</span>`).join(""); }

