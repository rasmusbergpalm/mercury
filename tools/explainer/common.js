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
function hexMap(svg, { fill, label = (t) => t.name, arrows = [], selected = null, onPick = null }) {
  const w = W * 6.5 + 12, h = R * (2 + 1.5 * 5) + 12;
  svg.setAttribute("viewBox", `0 0 ${w} ${h}`);
  let s = `<rect x="0" y="0" width="${w}" height="${h}" fill="${css("--sea")}"/>`;
  s += `<defs><marker id="ah-${svg.id}" viewBox="0 0 10 10" refX="7" refY="5" markerWidth="4" markerHeight="4" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="${css("--ink")}"/></marker></defs><g class="hexes">`;
  for (const t of tiles) {
    const [x, y] = hexXY(t);
    s += `<polygon data-id="${t.id}" points="${hexPts(x, y)}" fill="${fill(t)}" stroke="${t.id === selected ? css("--accent") : css("--hex")}" stroke-width="${t.id === selected ? 3 : 1}" ${onPick ? 'style="cursor:pointer" tabindex="0" role="button" aria-label="' + esc(t.name) + '"' : ""}/>`;
  }
  s += "</g>";
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
function diverge(v, maxAbs) {
  const t = Math.max(-1, Math.min(1, v / maxAbs));
  const base = css("--paper");
  const col = t >= 0 ? css("--brass") : css("--accent");
  return mix(base, col, Math.abs(t));
}
function mix(a, b, t) {
  const p = (c) => { const d = document.createElement("div"); d.style.color = c; document.body.appendChild(d); const v = getComputedStyle(d).color.match(/\d+/g).map(Number); d.remove(); return v; };
  const x = p(a), y = p(b);
  return `rgb(${x.map((v, i) => Math.round(v + (y[i] - v) * t)).join(",")})`;
}

// ---- charts -----------------------------------------------------------------
// lineChart draws series of [x, y] points with a log or linear x axis.
function lineChart(svg, { series, xLog = false, xTicks, xLabel, yLabel, yFmt = (v) => fmt(v), title = "", marks = [], w = 520, h = 300, yMin, yMax, points = [] }) {
  const m = { l: 54, r: 14, t: title ? 30 : 12, b: 44 };
  const xs = series.flatMap((s) => s.pts.map((p) => p[0])).concat(points.map((p) => p.x));
  const ys = series.flatMap((s) => s.pts.map((p) => p[1])).concat(points.map((p) => p.y));
  const fx = xLog ? Math.log10 : (v) => v;
  let x0 = Math.min(...xs.map(fx)), x1 = Math.max(...xs.map(fx));
  let y0 = yMin ?? Math.min(...ys), y1 = yMax ?? Math.max(...ys);
  const pad = (y1 - y0) * 0.06 || 1; if (yMin === undefined) y0 -= pad; if (yMax === undefined) y1 += pad;
  const X = (v) => m.l + (fx(v) - x0) / (x1 - x0) * (w - m.l - m.r);
  const Y = (v) => h - m.b - (v - y0) / (y1 - y0) * (h - m.t - m.b);
  svg.setAttribute("viewBox", `0 0 ${w} ${h}`);
  let s = "";
  if (title) s += `<text class="title" x="${m.l}" y="18">${esc(title)}</text>`;
  const yt = niceTicks(y0, y1, 5);
  for (const v of yt) s += `<line class="grid" x1="${m.l}" x2="${w - m.r}" y1="${Y(v)}" y2="${Y(v)}"/><text x="${m.l - 6}" y="${Y(v) + 4}" text-anchor="end">${yFmt(v)}</text>`;
  for (const v of xTicks) { const x = X(v); if (x < m.l - 1 || x > w - m.r + 1) continue; s += `<line class="axis" x1="${x}" x2="${x}" y1="${h - m.b}" y2="${h - m.b + 4}"/><text x="${x}" y="${h - m.b + 18}" text-anchor="middle">${v}</text>`; }
  s += `<line class="axis" x1="${m.l}" x2="${w - m.r}" y1="${h - m.b}" y2="${h - m.b}"/>`;
  if (xLabel) s += `<text x="${(m.l + w - m.r) / 2}" y="${h - 6}" text-anchor="middle">${esc(xLabel)}</text>`;
  if (yLabel) s += `<text x="12" y="${(m.t + h - m.b) / 2}" text-anchor="middle" transform="rotate(-90 12 ${(m.t + h - m.b) / 2})">${esc(yLabel)}</text>`;
  for (const mk of marks) { const x = X(mk.x); s += `<line x1="${x}" x2="${x}" y1="${m.t}" y2="${h - m.b}" stroke="${css("--muted")}" stroke-dasharray="4 4"/>${mk.label ? `<text x="${x + 5}" y="${m.t + 12}">${esc(mk.label)}</text>` : ""}`; }
  // lines are clipped to the plot (a series may run off a fixed scale)
  const clip = `clip-${svg.id || Math.random().toString(36).slice(2)}`;
  s += `<clipPath id="${clip}"><rect x="${m.l}" y="${m.t}" width="${w - m.l - m.r}" height="${h - m.t - m.b}"/></clipPath><g clip-path="url(#${clip})">`;
  for (const se of series) {
    const d = se.pts.map((p, i) => `${i ? "L" : "M"}${X(p[0]).toFixed(1)},${Y(p[1]).toFixed(1)}`).join("");
    s += `<path d="${d}" fill="none" stroke="${css(se.color)}" stroke-width="${se.width || 2.4}" ${se.dash ? `stroke-dasharray="${se.dash}"` : ""} stroke-linejoin="round" stroke-linecap="round"/>`;
  }
  s += "</g>";
  for (const se of series) {
    if (se.end) { const p = se.pts[se.pts.length - 1]; s += `<circle cx="${X(p[0])}" cy="${Y(p[1])}" r="3.5" fill="${css(se.color)}"/><text x="${X(p[0]) - 6}" y="${Y(p[1]) - 8}" text-anchor="end" style="fill:${css(se.color)}" ${se.below ? `dy="26"` : ""}>${esc(se.end)}</text>`; }
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

