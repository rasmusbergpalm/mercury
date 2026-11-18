#!/bin/sh
# Builds web/static/economy.html, the economy explainer the server hosts,
# from fresh runs of the engine on the classic map with no nations: the
# transport sweep and the explainer's experiments (TestExplainerData).
#
#   sh tools/explainer/build.sh
#   sh tools/explainer/build.sh DIR   # re-wrap the page from DIR's runs (printed by a full build)
set -e
if [ -n "$1" ]; then
  out=$1
else
  out=$(mktemp -d)
  TRANSPORT_SWEEP_OUT=$out/sweep go test ./game -run TestTransportSweep -timeout 60m
  EXPLAINER_OUT=$out/data go test ./game -run TestExplainerData -timeout 30m
fi
python3 tools/explainer/prep.py "$out"
python3 - "$out" <<'PY'
import sys
out = sys.argv[1]
style = open("tools/explainer/common.css", encoding="utf-8").read()
common = open("tools/explainer/common.js", encoding="utf-8").read()
data = open(f"{out}/explainer-data.json", encoding="utf-8").read().replace("</", "<\\/")
for name in ("economy",):
    page = open(f"tools/explainer/{name}.html", encoding="utf-8").read()
    page = page.replace("__STYLE__", style).replace("__COMMON__", common).replace("__DATA__", data)
    html = ('<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8">\n'
            '<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">\n'
            + page.replace('<div class="page">', '</head>\n<body>\n<div class="page">', 1)
            + "\n</body>\n</html>\n")
    open(f"web/static/{name}.html", "w", encoding="utf-8").write(html)
PY
echo "wrote web/static/economy.html from the runs in $out"
