#!/bin/sh
set -e

MOD=$MODULE
REPO=$REPO
REF=$REF

mkdir /work
cd /work

git clone "$REPO" proj
cd proj
git checkout "$REF"

mkdir /test
cd /test

go mod init tmp >/dev/null 2>&1
go get "$MOD" >/dev/null 2>&1

MODPATH=$(go list -m -f '{{.Path}}' "$REPO" 2>/dev/null || true)
if [ -n "$MODPATH" ]; then
  go mod edit -replace "$MODPATH=/work/proj"
fi

if go test ./... >/dev/null 2>&1; then
  echo "{\"module\":\"$MOD\",\"passed\":true}"
else
  echo "{\"module\":\"$MOD\",\"passed\":false}"
fi
