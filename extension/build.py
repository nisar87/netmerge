#!/usr/bin/env python3
"""Build the extension packages: dist/netmerge-chrome.zip (Chrome, Edge, Brave) and dist/netmerge-firefox.zip.
Also writes the icons (plain PNGs, no libraries needed). Usage: python3 extension/build.py"""
import json, os, struct, zlib, zipfile

here = os.path.dirname(os.path.abspath(__file__))

def png(size):
    # blue rounded square with a white down arrow and a bar
    bg, fg = (37, 99, 235), (255, 255, 255)
    rows = []
    r = size * 0.22
    for y in range(size):
        row = bytearray([0])
        for x in range(size):
            cx, cy = min(x, size - 1 - x), min(y, size - 1 - y)
            inside = True
            if cx < r and cy < r:
                inside = (r - cx - 0.5) ** 2 + (r - cy - 0.5) ** 2 <= r * r
            px = (0, 0, 0, 0)
            if inside:
                px = bg + (255,)
                u, v = x / size, y / size
                shaft = 0.42 <= u <= 0.58 and 0.18 <= v <= 0.52
                head = 0.52 <= v <= 0.72 and abs(u - 0.5) <= (0.72 - v) * 1.1 + 0.0
                bar = 0.24 <= u <= 0.76 and 0.78 <= v <= 0.86
                if shaft or head or bar:
                    px = fg + (255,)
            row += bytes(px)
        rows.append(bytes(row))
    raw = b"".join(rows)
    def chunk(t, d):
        c = struct.pack(">I", len(d)) + t + d
        return c + struct.pack(">I", zlib.crc32(t + d) & 0xFFFFFFFF)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b"")

os.makedirs(os.path.join(here, "icons"), exist_ok=True)
for s in (16, 32, 48, 128):
    with open(os.path.join(here, "icons", f"{s}.png"), "wb") as f:
        f.write(png(s))

files = ["core.js", "background.js", "popup.html", "popup.js", "popup.css"]
os.makedirs(os.path.join(here, "dist"), exist_ok=True)
for name, manifest in (("chrome", "manifest.chrome.json"), ("firefox", "manifest.firefox.json")):
    m = json.load(open(os.path.join(here, manifest)))
    out = os.path.join(here, "dist", f"netmerge-{name}.zip")
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("manifest.json", json.dumps(m, indent=2))
        for f in files:
            z.write(os.path.join(here, "src", f), f)
        for s in (16, 32, 48, 128):
            z.write(os.path.join(here, "icons", f"{s}.png"), f"icons/{s}.png")
    print("wrote", out)
