"""Builds Mercury's world map (game/worldmap.go) from Natural Earth data.

The world from 78N to 56S on an odd-r hex grid, 120 columns by 51 rows
(about 2000 land tiles: the scale of a grand-strategy map), the Pacific at
its seam (170W). A cell is land when its centre is, or when land covers 30%
of it; the great lakes and inland seas are water.

Each land tile gets:
- a terrain: Natural Earth's mountain ranges, plateaus, deserts and tundra
  where they lie, else by climate (latitude and region): ice-free polar
  land is tundra, the boreal belt forest, the dry interiors desert or
  steppe, the tropics tropical, the rest plains with patches of forest;
- a name: the most populous place within it, else the nearest unused one
  near it, else the region it lies in (Natural Earth's geography regions);
- an owner in 1820, from today's countries as a rough stand-in for the
  great powers' lands then (the rest is free land, to settle or colonise).

    python3 tools/world_map.py <dir of Natural Earth geojson>  # writes game/worldmap.go

The geojson files (from github.com/nvkelso/natural-earth-vector, public
domain): ne_50m_land, ne_50m_lakes, ne_50m_geography_regions_polys,
ne_10m_populated_places_simple, ne_110m_admin_0_countries.
"""
import json
import math
import sys
import unicodedata

from shapely import affinity
from shapely.geometry import Point, Polygon, shape
from shapely.ops import unary_union
from shapely.prepared import prep

COLS, ROWS = 120, 51
WEST, NORTH = -170.0, 78.0
DLON = 360 / COLS
DLAT = DLON * math.sqrt(3) / 2
R = DLON / math.sqrt(3)  # hex radius, in degrees


def centre(r, c):
    return WEST + (c + 0.5 + 0.5 * (r % 2)) * DLON, NORTH - (r + 0.5) * DLAT


def hexagon(r, c):
    lon, lat = centre(r, c)
    return Polygon([(lon + R * math.cos(math.radians(a)), lat + R * math.sin(math.radians(a))) for a in range(30, 390, 60)])


def load(d, name):
    return json.load(open(f"{d}/{name}.geojson", encoding="utf-8"))["features"]


def shifted(geom):
    """The geometry with what lies west of 170W moved east of 180."""
    west = geom.intersection(Polygon([(-180, -90), (WEST, -90), (WEST, 90), (-180, 90)]))
    if west.is_empty:
        return geom
    return unary_union([geom.difference(west), affinity.translate(west, xoff=360)])


def main(d):
    land = unary_union([shape(f["geometry"]) for f in load(d, "ne_50m_land")])
    lakes = unary_union([shape(f["geometry"]) for f in load(d, "ne_50m_lakes") if f["properties"].get("scalerank", 9) <= 1])
    land = shifted(land.difference(lakes))
    pland = prep(land)

    regions = []  # (class, name, prepared geometry)
    for f in load(d, "ne_50m_geography_regions_polys"):
        p = f["properties"]
        regions.append((p["FEATURECLA"], (p.get("NAME_EN") or p["NAME"]).strip(), prep(shifted(shape(f["geometry"])))))

    def region(lon, lat, *classes):
        pt = Point(lon, lat)
        for cl, name, g in regions:
            if cl in classes and g.contains(pt):
                return name
        return None

    # ---- land and terrain
    grid = [["~"] * COLS for _ in range(ROWS)]
    for r in range(ROWS):
        for c in range(COLS):
            lon, lat = centre(r, c)
            h = hexagon(r, c)
            if pland.contains(Point(lon, lat)) or (pland.intersects(h) and land.intersection(h).area >= 0.3 * h.area):
                grid[r][c] = terrain(lon, lat, region, r, c)

    # ---- owners in 1820
    countries = {}
    for f in load(d, "ne_110m_admin_0_countries"):
        countries.setdefault(f["properties"]["ADM0_A3"], []).append(shifted(shape(f["geometry"])))
    owners = []  # (symbol, prepared geometry)
    for sym, name, colour, codes, box in NATIONS:
        geoms = [g for code in codes for g in countries.get(code, [])]
        g = unary_union(geoms)
        if box:
            g = g.intersection(Polygon([(box[0], box[1]), (box[2], box[1]), (box[2], box[3]), (box[0], box[3])]))
        owners.append((sym, g, prep(g)))
    nations = [["."] * COLS for _ in range(ROWS)]
    for r in range(ROWS):
        for c in range(COLS):
            if grid[r][c] == "~":
                continue
            lon, lat = centre(r, c)
            pt, h = Point(lon, lat), hexagon(r, c)
            best, share = ".", 0.0
            for sym, g, pg in owners:
                if pg.contains(pt):
                    best = sym
                    break
                if pg.intersects(h):  # a coastal hex whose centre is at sea
                    a = g.intersection(h).area / h.area
                    if a > share and a >= 0.25:
                        best, share = sym, a
            nations[r][c] = best

    # ---- names
    places = []
    for f in load(d, "ne_10m_populated_places_simple"):
        p = f["properties"]
        lon = p["longitude"] + (360 if p["longitude"] < WEST else 0)
        places.append((p["pop_max"] or 0, ascii_name(p["name"]), lon, p["latitude"]))
    places.sort(key=lambda x: -x[0])
    used, names = set(), {}
    cells = [(r, c) for r in range(ROWS) for c in range(COLS) if grid[r][c] != "~"]
    # the most populous place in each tile (a port whose point lies in a hex
    # counted as sea goes to the nearest land hex): the biggest towns first
    land_cells = set(cells)
    best = {}
    for pop, name, lon, lat in places:
        if name in used:
            continue
        r0, c0 = cell_of(lon, lat)
        near = [(r0 + dr, c0 + dc) for dr in (-1, 0, 1) for dc in (-1, 0, 1)]
        near = [rc for rc in near if rc in land_cells]
        if not near:
            continue
        rc = min(near, key=lambda rc: (centre(*rc)[0] - lon) ** 2 + (centre(*rc)[1] - lat) ** 2)
        x, y = centre(*rc)
        if rc not in best and (x - lon) ** 2 + (y - lat) ** 2 <= (1.2 * R) ** 2:
            best[rc] = name
            used.add(name)
    names.update(best)
    # else the nearest unused place within two tiles
    for rc in cells:
        if rc in names:
            continue
        lon, lat = centre(*rc)
        near = sorted(((lon - x) ** 2 + (lat - y) ** 2, n) for _, n, x, y in places if abs(x - lon) < 2 * DLON and abs(y - lat) < 2 * DLAT and n not in used)
        if near:
            names[rc] = near[0][1]
            used.add(near[0][1])
    # else the region it lies in, told apart by compass points
    for rc in cells:
        if rc in names:
            continue
        lon, lat = centre(*rc)
        base = region(lon, lat, *REGION_CLASSES)
        if base:
            base = title(base)
            for q in [""] + [f"{p} " for p in ("North", "South", "East", "West", "Upper", "Lower", "Inner", "Outer", "Central", "Far")]:
                if q + base not in used:
                    names[rc] = q + base
                    used.add(q + base)
                    break
    unnamed = [rc for rc in cells if rc not in names]

    write_go(grid, nations, names, cells, unnamed)
    write_preview(grid, nations)
    counts = {}
    for rc in cells:
        counts[grid[rc[0]][rc[1]]] = counts.get(grid[rc[0]][rc[1]], 0) + 1
    print(f"{len(cells)} land tiles; terrain {counts}; {len(unnamed)} named by the game")


REGION_CLASSES = ("Desert", "Tundra", "Range/mtn", "Plateau", "Plain", "Basin", "Lowland", "Geoarea", "Island", "Island group",
                  "Pen/cape", "Peninsula", "Coast", "Delta", "Valley", "Wetlands", "Foothills", "Isthmus", "Continent")


def title(s):
    return " ".join(w if w.isupper() and len(w) <= 3 and "." in w else w.capitalize() for w in s.replace("MTS.", "Mountains").split())


def ascii_name(s):
    return unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode().strip()


def cell_of(lon, lat):
    r = int((NORTH - lat) / DLAT)
    c = int((lon - WEST) / DLON - 0.5 * (r % 2))
    return r, c


def noise(r, c, salt):
    """A smooth 0..1 pattern over the grid, for patches of forest and hills."""
    x, y = c * 0.35, r * 0.4
    v = math.sin(x * 1.3 + salt) + math.sin(y * 1.7 + 2 * salt) + math.sin((x + y) * 0.9 + 3 * salt) + math.sin((x - y) * 1.1 + salt / 2)
    return (v + 4) / 8


def within(lon, lat, box):
    return box[0] <= lon <= box[2] and box[1] <= lat <= box[3]


# dry lands the region polygons leave out: (west, south, east, north)
DESERTS = [
    (-17, 17, 33, 31),     # Sahara
    (34, 15.5, 57, 32),    # Arabia
    (44, 28, 66, 36),      # Persia's deserts and Baluchistan
    (69, 24, 75, 30),      # Thar
    (52, 37, 66, 45),      # Karakum and Kyzylkum
    (77, 37, 92, 41.5),    # Taklamakan
    (95, 39, 112, 45),     # Gobi
    (118, -32, 143, -19),  # inland Australia
    (11, -28, 22, -17),    # Namib and Kalahari
    (-72, -27, -68, -17),  # Atacama
    (-118, 30, -108, 37),  # Mojave and Sonora
    (-120, 37, -112, 42),  # Great Basin
    (40, 2, 51, 12),       # Somalia and the Ogaden
]
OASES = [
    (30.5, 22, 32.5, 31.5),  # the Nile valley
    (43, 13, 46, 17),        # the Yemen highlands
    (44, 30, 48.5, 36),      # Mesopotamia
]
STEPPES = [
    (-106, 30, -97, 50),    # Great Plains
    (-114, 49, -98, 54),    # the Canadian prairies
    (28, 44, 60, 51),       # the Pontic and Caspian steppe
    (60, 45, 90, 52),       # the Kazakh steppe
    (90, 44, 122, 51),      # Mongolia
    (-17, 12, 40, 17),      # the Sahel
    (-72, -52, -63, -38),   # Patagonia
    (114, -35, 152, -19),   # Australia's dry rim (inside: desert)
    (20, -30, 32, -22),     # the Highveld
]
FORESTS = [  # where forest is likelier than open land
    (-96, 29, -60, 50),     # eastern North America
    (-128, 40, -117, 60),   # the Pacific northwest
    (-10, 44, 40, 62),      # Europe
    (-76, -56, -71, -38),   # southern Chile
    (165, -48, 180, -34),   # New Zealand
    (125, 30, 146, 52),     # Manchuria, Korea, Japan
    (144, -44, 149, -40),   # Tasmania
]


def terrain(lon, lat, region, r, c):
    if region(lon, lat, "Range/mtn"):
        return "M"
    # tundra beyond the tree line: about 67N, further south round Hudson Bay
    # and Labrador, in Greenland and in the far north-east of Asia
    polar = lat >= 67 or (lat >= 58 and -100 < lon < -55) or (lat >= 60 and -75 < lon < -10) or (lat >= 60 and lon > 160)
    if polar or region(lon, lat, "Tundra"):
        return "U"
    if not any(within(lon, lat, b) for b in OASES):
        if region(lon, lat, "Desert") or any(within(lon, lat, b) for b in DESERTS):
            # a dry rim of steppe round the Australian desert's core
            return "D"
    if region(lon, lat, "Plateau", "Foothills") or noise(r, c, 7) > 0.84:
        return "H"
    if any(within(lon, lat, b) for b in STEPPES):
        return "S"
    tropics = abs(lat) <= 23 or (lat <= 26 and 90 <= lon <= 125)
    if tropics:
        return "T"
    if lat >= 52 or (lat >= 48 and (lon > 60 or lon < -60)):  # the boreal forest
        return "F"
    if any(within(lon, lat, b) for b in FORESTS) and noise(r, c, 3) > 0.5:
        return "F"
    return "P"


# The great powers of 1820, on today's countries (ADM0_A3 codes), cut to a
# box where they held only part of it: (symbol, name, colour, codes, box).
NATIONS = [
    ("G", "Britain", "#b8322e", ["GBR", "IRL"], None),
    ("F", "France", "#2c5aa0", ["FRA"], (-6, 41, 10, 52)),
    ("S", "Spain", "#c08a1e", ["ESP"], (-10, 35, 5, 44.5)),
    ("P", "Portugal", "#4e7d3a", ["PRT"], (-10, 36, -6, 43)),
    ("K", "Prussia", "#3d4a5c", ["DEU"], None),
    ("A", "Austria", "#a83c78", ["AUT", "HUN", "CZE", "SVK", "SVN", "HRV"], None),
    ("R", "Russia", "#4f6b3a", ["RUS", "FIN", "EST", "LVA", "LTU", "BLR", "UKR", "MDA", "POL", "GEO"], None),
    ("O", "Ottoman Empire", "#1f7a7a", ["TUR", "GRC", "BGR", "ROU", "SRB", "BIH", "ALB", "MKD", "MNE", "KOS", "SYR", "LBN",
                                        "ISR", "PSE", "JOR", "IRQ", "EGY", "LBY", "TUN", "CYP"], None),
    ("I", "Persia", "#c25e1b", ["IRN"], None),
    ("Q", "Qing", "#7a5c2e", ["CHN", "MNG", "TWN"], None),
    ("J", "Japan", "#6b3f8c", ["JPN"], None),
    ("U", "United States", "#5a8fbf", ["USA"], (-95, 24, -66, 50)),
]


def write_go(grid, nations, names, cells, unnamed):
    out = ["// Code generated by tools/world_map.py from Natural Earth data; DO NOT EDIT.", "", "package engine", "",
           "// WorldMap is the world on an odd-r hex grid, 78N to 56S, the Pacific at", "// its seam (see tools/world_map.py).",
           "var WorldMap = []string{"]
    out += [f'\t"{"".join(row)}",' for row in grid]
    out += ["}", "", "// WorldNations is who held which tile in 1820 ('.': free land).", "var WorldNations = []string{"]
    out += [f'\t"{"".join(row)}",' for row in nations]
    out += ["}", "", "// WorldNationSpecs are the great powers of 1820.", "var WorldNationSpecs = []NationSpec{"]
    out += [f'\t{{\'{s}\', "{n}", "{col}"}},' for s, n, col, _, _ in NATIONS]
    out += ["}", "", "// worldNames are the tiles' names, row by row (\"\": named by the game).", "var worldNames = []string{"]
    line = "\t"
    for rc in cells:
        item = json.dumps(names.get(rc, ""), ensure_ascii=False) + ", "
        if len(line) + len(item) > 110:
            out.append(line.rstrip())
            line = "\t"
        line += item
    out += [line.rstrip(), "}", ""]
    open("game/worldmap.go", "w", encoding="utf-8").write("\n".join(out))


def write_preview(grid, nations):
    from PIL import Image, ImageDraw
    colours = {"~": (60, 90, 140), "P": (227, 213, 155), "F": (110, 150, 80), "H": (200, 170, 120), "M": (150, 140, 130),
               "T": (90, 160, 80), "D": (240, 220, 160), "U": (200, 210, 215), "S": (205, 200, 120)}
    s = 8
    img = Image.new("RGB", (int((COLS + 1) * s), int(ROWS * s * 0.866) + s), (60, 90, 140))
    dr = ImageDraw.Draw(img)
    for r in range(ROWS):
        for c in range(COLS):
            x, y = (c + 0.5 * (r % 2)) * s, r * s * 0.866
            dr.ellipse([x, y, x + s, y + s], fill=colours[grid[r][c]])
    img.save("/dev/stdout" if len(sys.argv) > 2 and sys.argv[2] == "-" else (sys.argv[2] if len(sys.argv) > 2 else "world_preview.png"))


if __name__ == "__main__":
    main(sys.argv[1])
