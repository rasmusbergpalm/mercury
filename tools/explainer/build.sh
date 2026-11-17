#!/bin/sh
# Builds the two pages the server hosts about the economy, from fresh runs of
# the engine (the classic map with no nations): web/static/economy.html, the
# explainer, and web/static/transport.html, the working paper on transport
# (the transport sweep, and carriage against the iceberg model). Both share
# common.css and common.js.
#
#   sh tools/explainer/build.sh
set -e
out=$(mktemp -d)
TRANSPORT_SWEEP_OUT=$out/sweep go test ./engine -run TestTransportSweep -timeout 60m
EXPLAINER_OUT=$out/data go test ./engine -run TestExplainerData -timeout 30m
ICEBERG_OUT=$out/iceberg go test ./engine -run TestIcebergComparison -timeout 60m
python3 tools/explainer/prep.py "$out"
python3 - "$out" <<'PY'
import sys
out = sys.argv[1]
style = open("tools/explainer/common.css", encoding="utf-8").read()
common = open("tools/explainer/common.js", encoding="utf-8").read()
data = open(f"{out}/explainer-data.json", encoding="utf-8").read().replace("</", "<\\/")
for name in ("economy", "transport"):
    page = open(f"tools/explainer/{name}.html", encoding="utf-8").read()
    page = page.replace("__STYLE__", style).replace("__COMMON__", common).replace("__DATA__", data)
    html = ('<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8">\n'
            '<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">\n'
            + page.replace('<div class="page">', '</head>\n<body>\n<div class="page">', 1)
            + "\n</body>\n</html>\n")
    open(f"web/static/{name}.html", "w", encoding="utf-8").write(html)
PY
echo "wrote web/static/economy.html and web/static/transport.html"
