# Windows application icon

Every Windows executable must embed the NetMerge icon. The committed root-level `netmerge_windows_amd64.syso`, `netmerge_windows_arm64.syso` and `netmerge_windows_386.syso` files are selected automatically by Go for the matching Windows architecture. Ordinary `go build` commands need no extra resource compiler or icon flags. Including an icon for an architecture does not extend the application's supported platforms; the shipped target remains Windows x64.

The source is the existing NetMerge brand mark in `ui/assets/netmerge.svg`. `netmerge.ico` contains native 16, 20, 24, 32, 48, 64, 128 and 256 pixel images. The browser favicon uses the same SVG.

To update the icon, install the existing frontend development dependencies and Chromium, then run from the repository root:

```sh
node frontend/scripts/build-app-icon.mjs
bash scripts/generate-windows-resources.sh
```

`NETMERGE_BROWSER_EXECUTABLE` can select an installed Chromium. The resource generator uses the pinned `github.com/akavel/rsrc@v0.10.2` tool, without changing the application's module dependencies. Commit the SVG, ICO and all three generated resource files together.

After building an EXE, run:

```sh
python3 scripts/check-windows-icon.py NetMerge.exe
```

The verifier reads the PE resource directory and checks every embedded image against the ICO. Both CI and release builds run it, so a missing or stale icon fails the build before publishing. The committed resources contain only icons: they do not change elevation, network behavior or application settings.

Release builds combine the same icon with the stable release's product and version details using `goversioninfo@v1.4.1`. Generation writes a temporary file and replaces the amd64 resource only after success; a generation failure leaves the committed icon available. Keep exactly one resource object per Windows architecture, because Go rejects multiple `.rsrc` sections.
