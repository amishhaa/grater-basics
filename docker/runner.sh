#!/bin/sh
set -e

MOD="$MODULE"
REPO="$REPO"
REF="$REF"

# Send everything to stderr by default
exec 3>&1 1>&2

mkdir /work
cd /work

git clone "$REPO" proj
cd proj
git checkout "$REF"

mkdir /test
cd /test

go mod init tmp >/dev/null
go get "$MOD" >/dev/null

MODPATH=$(go list -m -f '{{.Path}}' "$REPO" 2>/dev/null || true)
if [ -n "$MODPATH" ]; then
  go mod edit -replace "$MODPATH=/work/proj"
fi

if go test ./... >/dev/null; then
  RESULT=true
else
  RESULT=false
fi

# Restore stdout and print ONLY JSON
exec 1>&3
echo "{\"module\":\"$MOD\",\"passed\":$RESULT}"
