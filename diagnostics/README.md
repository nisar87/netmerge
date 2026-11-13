# Diagnostics database

Firebase project `netmerge-windows` ("NetMerge Windows", Spark plan, no cost), Cloud Firestore `(default)` in
asia-south1, set up on 13 November 2026 in the Firebase console:

- the rules in `firestore.rules` (published in Firestore > Rules): reports may only be added, with the fields and
  sizes listed there; nothing can be read, changed or deleted from the app;
- the web app "NetMerge Windows diagnostics" gives the API key in `report.go`. A Firebase key is not a secret: it
  only names the project, and the rules decide what it may do.

Checked from a browser on 13 November 2026: adding a report gives 200; reading, an unknown field or another `kind`
gives 403. CI checks the same on every change (`TestReportAcceptedByFirestore`).

## What the app sends

`report.go`: every 6 hours while NetMerge runs, and about a minute after a crash (or at the next start), a document
in the collection `reports`:

| Field | What |
| --- | --- |
| `install` | random id made once per installation (CI's test reports start with `ci-test`) |
| `kind` | `log`, or `crash` when a crash report is waiting |
| `app` | NetMerge version |
| `os`, `device` | Windows version and build; PC make, model, processor, memory, language |
| `settings` | settings as on/off values and numbers, lists as counts (`reportSettings`) |
| `engine` | engine version and state, each connection's type, health, delay, speed |
| `log` | `netmerge.log` since the last report, repeated lines counted, newest ~100 KB, scrubbed (`scrub`) |
| `crash` | Go runtime crash (`go-crash.log`), a recovered crash with its stack, or the engine's last output when it stopped unexpectedly |
| `at` | time sent |

The log (`eventlog.go`) holds: start-up facts, how the last run ended (Windows shut down, or an unexpected end),
every start and stop of merging and who asked for it, connections appearing, going away or changing network,
settings changes, failures that used to be dropped silently (with the place in the code), errors the window met,
update steps (including the updater script's own log), and once a minute the counts of failed connections per line
and cause (`tcp.LINK2.timeout=3`) and of failed health checks. Website names are never written to it.

## Who reads them

Nobody has to open the console: `diagnostics/read.sh [days]`, run by a daily scheduled Claude task, makes a new key
pair, starts the **Diagnostics** workflow (`.github/workflows/diagnostics.yml`) with the public half, and the
workflow reads the reports with the CI service account (`netmerge-ci@netmerge-tests`, Cloud Datastore Viewer on
`netmerge-windows`, signed in through Workload Identity Federation, provider `netmerge-windows`, no key file),
encrypts them with that key and leaves them on the branch `diagnostics-data` (one commit, replaced each run). This
repository is public: the workflow log shows only counts, and no lasting key exists that opens a run (the private
half lives only while read.sh runs).
