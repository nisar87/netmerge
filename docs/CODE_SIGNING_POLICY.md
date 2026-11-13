# Code signing policy (DRAFT)

> Draft. NetMerge has applied to the SignPath Foundation for free code signing; the application is pending and
> no release is signed yet. This page will be updated when that changes.

NetMerge.exe is built from the source in this repository by the GitHub Actions workflow `.github/workflows/release.yml`.
Once the project is accepted, the release workflow will send that build to SignPath for signing.

## What is signed

Only NetMerge.exe, built by this project. Its network engine is part of the same program. The one file by other
authors it carries, wintun.dll (the WireGuard project's virtual network adapter driver, checked against its
published fingerprint at build time), keeps its authors' own signature and is not signed by this project.

## Roles

| Role | Who |
|---|---|
| Author (writes the code) | the NetMerge maintainer(s), GitHub: nisar87 |
| Reviewer | the NetMerge maintainer(s), GitHub: nisar87 |
| Approver (approves each signing request by hand) | the NetMerge maintainer, GitHub: nisar87 |

Everyone with access to the source repository and to the signing service uses multi-factor authentication.
Every release needs a manual approval before it is signed.

## Privacy

NetMerge does not collect or send usage data, and has no analytics or telemetry. By itself it connects to these
services, and only when the matching feature is used:

- GitHub (`api.github.com`, `github.com`, `raw.githubusercontent.com`, `cdn.jsdelivr.net`): the update check, downloading the network engine, and downloading the optional blocking lists.
- Speed test servers (`speed.cloudflare.com`, `proof.ovh.net`) when you run a speed test.
- Connectivity checks (`www.msftconnecttest.com`, `www.gstatic.com`) to see whether a connection is online.
- `ifconfig.me`, to show the public address of a connection.
- The DNS servers you set.

NetMerge keeps its settings and history on your own computer, in its data folder.

## System changes

NetMerge says before it changes anything on your computer. Changes it can make, when you turn the matching feature on:
start with Windows (a scheduled task), a Windows Firewall rule for sharing your connection with other devices on your local network, and downloading the network engine.
The scheduled task and the firewall rule follow the matching setting. (Before applying, the maintainer must verify that statement, and that the uninstall steps remove everything.)

## Reporting a problem

Open an issue at https://github.com/nisar87/netmerge/issues. For security problems see SECURITY.md.
