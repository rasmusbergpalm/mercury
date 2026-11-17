#!/bin/sh
# Builds web/static/economy.html, the economy explainer the server hosts,
# from fresh runs of the engine: the transport sweep and the explainer data
# (the classic map with no nations).
#
#   sh tools/explainer/build.sh
set -e
out=$(mktemp -d)
TRANSPORT_SWEEP_OUT=$out/sweep go test ./engine -run TestTransportSweep -timeout 60m
EXPLAINER_OUT=$out/data go test ./engine -run TestExplainerData -timeout 30m
python3 tools/explainer/prep.py "$out"
python3 - "$out" <<'PY'
import sys
out = sys.argv[1]
page = open("tools/explainer/template.html", encoding="utf-8").read()
data = open(f"{out}/explainer-data.json", encoding="utf-8").read().replace("</", "<\\/")
html = ('<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8">\n'
        '<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">\n'
        + page.replace("__DATA__", data).replace('<div class="page">', '</head>\n<body>\n<div class="page">', 1)
        + "\n</body>\n</html>\n")
open("web/static/economy.html", "w", encoding="utf-8").write(html)
PY
echo "wrote web/static/economy.html"
