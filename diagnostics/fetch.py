#!/usr/bin/env python3
"""Reads the diagnostics reports of the last DAYS days from Firestore (project netmerge-windows) into reports.jsonl.

Run by .github/workflows/diagnostics.yml with an access token of netmerge-ci (Cloud Datastore Viewer on netmerge-windows).
Prints only counts: the reports themselves leave the workflow encrypted (see the workflow)."""
import datetime, json, os, sys, urllib.request

token = os.environ["TOKEN"]
days = float(os.environ.get("DAYS") or "2")
since = (datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=days)).strftime("%Y-%m-%dT%H:%M:%SZ")
url = "https://firestore.googleapis.com/v1/projects/netmerge-windows/databases/(default)/documents:runQuery"


def value(v):
    for k in ("stringValue", "timestampValue", "integerValue", "booleanValue", "doubleValue"):
        if k in v:
            return v[k]
    return None


out, after = [], None
while True:
    q = {"from": [{"collectionId": "reports"}],
         "where": {"fieldFilter": {"field": {"fieldPath": "at"}, "op": "GREATER_THAN_OR_EQUAL",
                                   "value": {"timestampValue": since}}},
         "orderBy": [{"field": {"fieldPath": "at"}}], "limit": 300}
    if after:
        q["startAt"] = {"values": [{"timestampValue": after}], "before": False}
    req = urllib.request.Request(url, json.dumps({"structuredQuery": q}).encode(),
                                 {"Authorization": "Bearer " + token, "Content-Type": "application/json"})
    rows = json.load(urllib.request.urlopen(req, timeout=60))
    docs = [r["document"] for r in rows if "document" in r]
    for d in docs:
        r = {k: value(v) for k, v in d.get("fields", {}).items()}
        r["id"] = d["name"].rsplit("/", 1)[1]
        out.append(r)
    if len(docs) < 300:
        break
    after = value(docs[-1]["fields"]["at"])

with open("reports.jsonl", "w") as f:
    for r in out:
        f.write(json.dumps(r) + "\n")
kinds = {}
for r in out:
    kinds[r.get("kind")] = kinds.get(r.get("kind"), 0) + 1
print(f"since {since}: {len(out)} reports {kinds}, {len({r.get('install') for r in out})} installs")
