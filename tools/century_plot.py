#!/usr/bin/env python3
"""Graphs of century runs (engine TestCentury), against history and a baseline.

    python3 tools/century_plot.py RUN_DIR [BASELINE_DIR] [--out OUT_DIR]

RUN_DIR (and BASELINE_DIR, say the same runs on main) hold the
century-s<seed>.csv files TestCentury writes. For each seed it draws:

  - macro-s<seed>.png: GDP a head, people, investment, capital, the return on
    capital and the living standard, against the rough 1820-1913 history of
    Western Europe and Britain, and the baseline if given;
  - methods-s<seed>.png: each production method's share of its good's output;
  - prices-s<seed>.png: machinery, chemicals, power, coal and iron.

It prints a decade table of the macro numbers for each run too. The
historical figures are orders of magnitude (Maddison, Feinstein, Crafts,
Piketty), not targets to hit to the decimal. Needs matplotlib.
"""
import argparse
import csv
import glob
import os

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402
from matplotlib.ticker import FuncFormatter  # noqa: E402

INK, INK2, SURF, GRID, BAND = "#0b0b0b", "#52514e", "#fcfcfb", "#e6e5e0", "#e9e8e2"
SLOTS = ["#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#6250d6"]
plt.rcParams.update({"font.size": 9, "axes.edgecolor": INK2, "axes.labelcolor": INK2,
                     "xtick.color": INK2, "ytick.color": INK2, "axes.spines.top": False,
                     "axes.spines.right": False, "figure.facecolor": SURF, "axes.facecolor": SURF})


def load(path):
    rows = list(csv.DictReader(open(path)))
    d = {k: [float(r[k]) for r in rows] for k in rows[0]}
    d["headIdx"] = [x / d["gdpHead"][0] for x in d["gdpHead"]]
    d["popIdx"] = [x / d["pop"][0] for x in d["pop"]]
    d["investPct"] = [x * 100 for x in d["invest"]]
    d["retPct"] = [x * 100 for x in d["ret"]]
    return d


def table(name, d):
    n = len(d["year"])
    print(f"== {name}")
    print(f"{'years':9s} {'people':>7s} {'GDP/head':>8s} {'invest':>7s} {'K/Y':>5s} {'return':>7s} {'farms':>6s} {'techs':>5s}")
    for a in range(0, n, 10):
        b = min(a + 9, n - 1)
        span = max(b - a, 1)
        rate = lambda k: (d[k][b] / d[k][a]) ** (1 / span) - 1
        avg = lambda k: sum(d[k][a:b + 1]) / (b - a + 1)
        print(f"{int(d['year'][a]):3d}-{int(d['year'][b]):<5d} {rate('pop')*100:6.2f}% {rate('gdpHead')*100:7.2f}% "
              f"{avg('invest')*100:6.1f}% {avg('KY'):5.2f} {avg('ret')*100:6.1f}% {avg('primary')*100:5.0f}% {int(d['techs'][b]):5d}")
    y = n - 1
    print(f"century: people {((d['pop'][y]/d['pop'][0])**(1/y)-1)*100:.2f}%/yr, GDP a head "
          f"{((d['gdpHead'][y]/d['gdpHead'][0])**(1/y)-1)*100:.2f}%/yr, living {d['living'][0]:.1f} -> {d['living'][y]:.1f}")


def log_axis(ax):
    ax.set_yscale("log")
    ax.yaxis.set_major_formatter(FuncFormatter(lambda v, _: f"{v:g}"))
    ax.yaxis.set_minor_formatter(FuncFormatter(lambda v, _: ""))


def macro(run, base, seed, out):
    years = run["year"]
    panels = [
        ("headIdx", "GDP a head, against year 1 (log)", "log", lambda y: 1.01 ** (y - 1), "~1%/yr (W. Europe 1820-1913)"),
        ("popIdx", "People, against year 1 (log)", "log", lambda y: 1.01 ** (y - 1), "~1%/yr (Britain ~1.2, Europe ~0.7)"),
        ("investPct", "Investment, % of GDP (gross, with upkeep)", "lin", (7, 20), "7-20% (Britain ... Germany/US)"),
        ("KY", "Capital, years of GDP", "lin", (2, 5), "~2-5"),
        ("retPct", "Return on capital, % a year", "lin", (5, 10), "~5-10%"),
        ("living", "Living standard", "lin", None, None),
    ]
    fig, axes = plt.subplots(3, 2, figsize=(10, 10), sharex=True)
    for ax, (key, title, scale, ref, label) in zip(axes.flat, panels):
        if callable(ref):
            ax.plot(years, [ref(y) for y in years], color=INK2, lw=1.2, ls="--", label=label)
        elif ref:
            ax.axhspan(ref[0], ref[1], color=BAND, lw=0, label=label)
        if base:
            ax.plot(base["year"], base[key], color=SLOTS[0], lw=2, label="Baseline")
        ax.plot(years, run[key], color=SLOTS[1], lw=2, label="This run")
        if scale == "log":
            log_axis(ax)
        ax.set_title(title, fontsize=10, color=INK, loc="left")
        ax.grid(axis="y", color=GRID, lw=0.8)
        ax.legend(frameon=False, labelcolor=INK, fontsize=8)
    for ax in axes[-1]:
        ax.set_xlabel("Year of the game (1820 + ...)")
    fig.suptitle(f"One nation, classic map, seed {seed}, against history", fontsize=11, color=INK, x=0.01, ha="left")
    fig.tight_layout()
    fig.savefig(os.path.join(out, f"macro-s{seed}.png"), dpi=150)
    plt.close(fig)


def methods(run, seed, out):
    groups = {}
    for k in run:
        if k.startswith("share "):
            g, f = k[6:].split("|")
            groups.setdefault(g, []).append(f)
    live = {g: fs for g, fs in groups.items() if any(max(run["share " + g + "|" + f]) > 0.005 for f in fs[1:])}
    if not live:
        return
    cols = 3
    rows = (len(live) + cols - 1) // cols
    fig, axes = plt.subplots(rows, cols, figsize=(11, 2.7 * rows + 0.6), sharex=True, sharey=True, squeeze=False)
    years = run["year"]
    for ax, (g, fs) in zip(axes.flat, live.items()):
        shown = [i for i, f in enumerate(fs) if max(run["share " + g + "|" + f]) > 0.005]
        ax.stackplot(years, [[x * 100 for x in run["share " + g + "|" + fs[i]]] for i in shown],
                     colors=[SLOTS[i % len(SLOTS)] for i in shown], edgecolor=SURF, linewidth=0.6,
                     labels=[fs[i] for i in shown])
        ax.set_title(g, fontsize=10, color=INK, loc="left")
        ax.set_ylim(0, 100)
        ax.yaxis.set_major_formatter(FuncFormatter(lambda v, _: f"{v:.0f}%"))
        ax.legend(frameon=False, fontsize=7, labelcolor=INK, loc="lower left")
    for ax in list(axes.flat)[len(live):]:
        ax.axis("off")
    for ax in axes[-1]:
        ax.set_xlabel("Year of the game (1820 + ...)")
    fig.suptitle(f"Each good's output by production method, seed {seed}", fontsize=11, color=INK, x=0.01, ha="left")
    fig.tight_layout()
    fig.savefig(os.path.join(out, f"methods-s{seed}.png"), dpi=150)
    plt.close(fig)


def prices(run, seed, out):
    fig, ax = plt.subplots(figsize=(9, 4))
    for i, k in enumerate(k for k in run if k.startswith("price ")):
        vals = run[k]
        first = next((j for j, v in enumerate(vals) if abs(v - 1.0) > 1e-3), len(vals))  # 1.00: nobody trades it yet
        if first < len(vals):
            ax.plot(run["year"][first:], vals[first:], color=(SLOTS + ["#1a1a19", "#c3c2b7", "#008300"])[i % 9], lw=2, label=k[6:])
    log_axis(ax)
    ax.set_title(f"Prices, map average (log), seed {seed}", fontsize=10, color=INK, loc="left")
    ax.grid(axis="y", color=GRID, lw=0.8)
    ax.legend(frameon=False, labelcolor=INK, fontsize=8, ncol=3)
    ax.set_xlabel("Year of the game (1820 + ...)")
    fig.tight_layout()
    fig.savefig(os.path.join(out, f"prices-s{seed}.png"), dpi=150)
    plt.close(fig)


def main():
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("run")
    p.add_argument("baseline", nargs="?")
    p.add_argument("--out")
    a = p.parse_args()
    out = a.out or a.run
    os.makedirs(out, exist_ok=True)
    for path in sorted(glob.glob(os.path.join(a.run, "century-s*.csv"))):
        seed = os.path.basename(path)[len("century-s"):-len(".csv")]
        run = load(path)
        base = None
        if a.baseline and os.path.exists(os.path.join(a.baseline, os.path.basename(path))):
            base = load(os.path.join(a.baseline, os.path.basename(path)))
            table(f"baseline, seed {seed}", base)
        table(f"run, seed {seed}", run)
        macro(run, base, seed, out)
        methods(run, seed, out)
        prices(run, seed, out)
    print(f"graphs in {out}")


if __name__ == "__main__":
    main()
