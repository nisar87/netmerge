#!/bin/sh
# Fetches and decrypts the diagnostics reports: diagnostics/read.sh [DAYS] -> reports.jsonl (or $OUT).
# Makes a new key pair, starts the Diagnostics workflow with the public half, waits for it, fetches the encrypted
# reports from the diagnostics-data branch and decrypts them; the private half is deleted at the end. Run inside a
# clone of the repository.
set -e
DAYS=${1:-2}; R=nisar87/netmerge; OUT=${OUT:-$PWD/reports.jsonl}
D=$(mktemp -d); trap 'rm -rf "$D"' EXIT
KEY=$D/reader.pem
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "$KEY" 2>/dev/null
PUB=$(openssl pkey -in "$KEY" -pubout | base64 -w0)
before=$(gh api "repos/$R/actions/workflows/diagnostics.yml/runs?per_page=1" -q '.workflow_runs[0].id // 0')
gh api -X POST "repos/$R/actions/workflows/diagnostics.yml/dispatches" -f ref=main -f "inputs[days]=$DAYS" -f "inputs[pubkey]=$PUB"
run=$before
while [ "$run" = "$before" ]; do sleep 5; run=$(gh api "repos/$R/actions/workflows/diagnostics.yml/runs?per_page=1" -q '.workflow_runs[0].id'); done
until [ "$(gh api repos/$R/actions/runs/$run -q .status)" = completed ]; do sleep 10; done
[ "$(gh api repos/$R/actions/runs/$run -q .conclusion)" = success ] || { echo "workflow run $run failed"; exit 1; }
git fetch -q origin diagnostics-data
for f in count.txt key.enc reports.enc; do git show FETCH_HEAD:$f > "$D/$f"; done
cat "$D/count.txt"
openssl pkeyutl -decrypt -inkey "$KEY" -pkeyopt rsa_padding_mode:oaep -in "$D/key.enc" -out "$D/k.bin"
openssl enc -d -aes-256-cbc -pbkdf2 -pass file:"$D/k.bin" -in "$D/reports.enc" | gunzip > "$OUT"
echo "$OUT: $(wc -l < "$OUT") reports"
