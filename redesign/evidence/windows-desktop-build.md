# Windows preview startup and permanent EXE branding

Implemented 9 November 2026 after the user reported that the preview did not start and required the NetMerge icon in every future EXE.

## Startup diagnosis

Read-only Windows inspection found NetMerge **2.3.3** listening on `127.0.0.1:9196`, with merging enabled. The preview's crash log contained launch markers but no panic, and its normal application log/settings had not been created. This matches the existing single-instance path in `main.go`: another NetMerge instance owns the port, so a new process attempts to show that instance's window and exits before initialization. The preview must be launched after quitting the existing app from its tray menu. No engine, networking, persistence or API changes are needed for this cause.

A related frontend integration issue was corrected: Windows `enumFocus` recognizes browser-app windows whose titles start with `NetMerge`. The redesigned route titles put the section name first. Every route now keeps the `NetMerge · Section` prefix so tray actions can find and focus its existing window. Route navigation and refresh tests enforce this contract.

## Windows icon

The existing converging-lines brand mark is now a shared vector favicon and source for the multi-resolution Windows ICO. Native icon sizes are 16, 20, 24, 32, 48, 64, 128 and 256 pixels. Committed architecture-specific `.syso` files make ordinary Windows Go builds include the icon automatically, without changing application dependencies or requiring a resource compiler on every build. The resources contain only icons.

CI and release builds run `scripts/check-windows-icon.py` against the built executable. It parses the PE resource tree and verifies each icon entry and image against the source ICO. Missing icons and stale image data fail verification. The pinned resource-generation command and update procedure are documented in `assets/windows/README.md`.

The user explicitly extended the original frontend-only scope to Windows EXE branding. The scope guard allows only the specific icon source, generated resources, maintenance scripts, documentation and CI/release verification changes. Backend Go source and dependencies remain untouched.

## Verification

- **47 browser tests passed**: all shell/deep-link/refresh cases, including the desktop title prefix, and all speed-test cases.
- The unbranded first preview fails icon verification with `Missing Windows icon resources`.
- The rebuilt Windows x64 GUI executable passes all eight embedded-icon image checks.
- A deliberately altered source ICO is rejected, proving the check catches a stale image rather than only looking for a resource section.
- Go selects exactly the appropriate `.syso` for Windows amd64, arm64 and 386, and selects none for Linux. The supported desktop target remains Windows x64; resource inclusion does not establish other architectures' runtime support.
- Inspected the rendered icon and verified source syntax and whitespace. The portable preview remains separate from the installed application and is not merged into `main` or published as a release.
