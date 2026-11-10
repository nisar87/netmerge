"""Download benchmark through the running NetMerge (download manager switched on).

   python bench.py [link] [runs]

Downloads the same file several times and prints, for each run: total time, average speed, time for the first half,
time for the last 10 MB (the "tail") and the bytes each line carried. Keep the numbers with the date and the lines that
were plugged in; compare only runs made close together, because the internet itself changes speed (some ISPs slow a
line after the first ~100 MB).

The default file is Google Chrome for Linux (~150 MB): the server allows ranges, so the file is split over the lines.
Cloudflare's speed-test links are NOT suitable: they ignore ranges and use one connection.
"""
import json, sys, time, urllib.request

API = "http://127.0.0.1:9196/api/downloads"
LINK = sys.argv[1] if len(sys.argv) > 1 else "https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb"
RUNS = int(sys.argv[2]) if len(sys.argv) > 2 else 3


def call(d):
    r = urllib.request.Request(API, json.dumps(d).encode(), {"Content-Type": "application/json", "X-NetMerge": "1"})
    return json.load(urllib.request.urlopen(r))


def item(i):
    for x in json.load(urllib.request.urlopen(API))["items"]:
        if x["id"] == i:
            return x


probe = call({"action": "probe", "url": LINK})
info = probe.get("info") or {}
print(f"{LINK}\n{info.get('size', 0) / 1e6:.1f} MB, ranges: {info.get('ranges')}, lines: {probe.get('lineNames')}")
for run in range(RUNS):
    time.sleep(5)
    r = call({"action": "add", "url": LINK, "name": f"bench{run}.bin", "force": True})
    i = r["added"][0]["id"]
    t0, half, tail, size, last = time.time(), 0.0, 0.0, 0, {}
    while True:
        time.sleep(0.25)
        d = item(i)
        el = time.time() - t0
        size = d.get("size") or size
        if size and not half and d["done"] >= size / 2:
            half = el
        if size and not tail and d["done"] >= size - 10e6:
            tail = el
        last = {l["name"]: l["bytes"] for l in d.get("lineStats") or []} or last
        if d["state"] in ("done", "error") or el > 600:
            break
    lines = ", ".join(f"{n} {b / 1e6:.0f} MB" for n, b in last.items())
    print(f"run {run + 1}: {d['state']}, {el:.1f} s, {size * 8 / el / 1e6:.0f} Mbps average, first half {half:.1f} s, "
          f"last 10 MB {el - tail:.1f} s ({lines})")
    call({"action": "remove", "id": i, "deleteFiles": True})
