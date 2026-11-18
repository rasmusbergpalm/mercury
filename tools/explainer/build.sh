#!/bin/sh
# Builds web/static/economy.html, the economy page the server hosts, from
# the experiments' results in experiments/data. To rerun an experiment
# first (see package experiments):
#
#   go generate -run corridor ./experiments    # one
#   go generate ./experiments                  # all of them
#
#   sh tools/explainer/build.sh
set -e
data=experiments/data
tmp=$(mktemp -d)
python3 tools/explainer/prep.py "$data" "$tmp/explainer-data.json"
python3 - "$tmp" <<'PY'
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
rm -r "$tmp"
echo "wrote web/static/economy.html from $data"
