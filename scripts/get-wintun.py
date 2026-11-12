"""Download wintun-0.14.1.zip (the WireGuard project's virtual adapter driver), check its published fingerprint
and put the amd64 wintun.dll into engine/wintun/amd64/, where the build embeds it. Prints the DLL's fingerprint,
which the build passes to the engine (it refuses any other file at run time).

Usage: python3 scripts/get-wintun.py   (from the repository root)
"""
import hashlib, io, os, sys, urllib.request, zipfile

URL = "https://www.wintun.net/builds/wintun-0.14.1.zip"
ZIP_SHA256 = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"

data = urllib.request.urlopen(URL, timeout=60).read()
got = hashlib.sha256(data).hexdigest()
if got != ZIP_SHA256:
    sys.exit(f"wintun zip fingerprint {got} is not the published {ZIP_SHA256}")
dll = zipfile.ZipFile(io.BytesIO(data)).read("wintun/bin/amd64/wintun.dll")
out = os.path.join("engine", "wintun", "amd64")
os.makedirs(out, exist_ok=True)
with open(os.path.join(out, "wintun.dll"), "wb") as f:
    f.write(dll)
print(hashlib.sha256(dll).hexdigest())
