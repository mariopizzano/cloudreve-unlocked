#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-4.19.1}"
ARCH="${2:-amd64}"
OUT="$ROOT/dist/cloudreve-${VERSION}-linux-${ARCH}"

mkdir -p "$ROOT/dist"

echo "== frontend $VERSION"
cd "$ROOT/frontend"
if [ ! -d node_modules ]; then
  if command -v yarn >/dev/null 2>&1; then
    yarn install --frozen-lockfile --ignore-scripts
  else
    npm ci --legacy-peer-deps
  fi
fi
npm pkg set version="$VERSION"
npm run build

echo "== embed frontend"
STAGE="$(mktemp -d)"
mkdir -p "$STAGE/assets"
cp -a "$ROOT/frontend/build" "$STAGE/assets/build"
python3 - "$STAGE" "$ROOT/cloudreve/application/statics/assets.zip" <<'PY'
import os, sys, zipfile
stage, out = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
    for root, dirs, files in os.walk(stage):
        dirs.sort()
        for name in sorted(files):
            full = os.path.join(root, name)
            z.write(full, os.path.relpath(full, stage))
PY
rm -rf "$STAGE"

echo "== backend $VERSION linux/$ARCH"
cd "$ROOT/cloudreve"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo 000000)"
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
  go build -trimpath \
  -ldflags "-s -w -X github.com/cloudreve/Cloudreve/v4/application/constants.BackendVersion=${VERSION} -X github.com/cloudreve/Cloudreve/v4/application/constants.LastCommit=${COMMIT}" \
  -o "$OUT" .

echo "== done"
ls -lh "$OUT"
sha256sum "$OUT"
