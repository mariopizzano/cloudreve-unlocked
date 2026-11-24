#!/bin/bash
# Scheduled Cloudreve DB backup (host side). Defaults match CT 112 on this host.
set -euo pipefail

SRC="${SRC:-/main_pool/subvol-112-disk-0/opt/cloudreve/data/cloudreve.db}"
DEST="${DEST:-/main_pool/cloudreve-backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
TS=$(date +%Y%m%d-%H%M%S)
OUT="$DEST/$TS"

mkdir -p "$OUT"
sqlite3 "$SRC" <<SQL
.timeout 10000
.backup '$OUT/cloudreve.db'
SQL

CHECK=$(sqlite3 "$OUT/cloudreve.db" "PRAGMA integrity_check;")
if [ "$CHECK" != "ok" ]; then
  echo "$(date -Is) ERROR integrity_check: $CHECK" >&2
  rm -rf "$OUT"
  exit 1
fi

sha256sum "$SRC" "$OUT/cloudreve.db" > "$OUT/SHA256SUMS"

find "$DEST" -maxdepth 1 -type d -name '20*' -mtime +"$RETENTION_DAYS" -exec rm -rf {} +

echo "$(date -Is) backup ok $OUT ($(du -sh "$OUT" | cut -f1))"
