#!/bin/sh
set -e

MOD="$MODULE"
REPO="$REPO"
REF="$REF"

# send all logs to stderr, only JSON to stdout
exec 3>&1 1>&2

mkdir -p /work
cd /work

echo "Cloning repo..."
git clone "$REPO" proj
cd proj
git checkout "$REF"

# download module using its own go.mod
echo "Downloading module $MOD..."
GO111MODULE=on go env -w GOPATH=/go
go install "$MOD@latest"

MODDIR=$(go env GOPATH)/pkg/mod/$(echo "$MOD" | sed 's|/|@|g' | sed 's|@|/|1')*
MODDIR=$(ls -d $MODDIR 2>/dev/null | head -n 1)

if [ ! -d "$MODDIR" ]; then
  exec 1>&3
  echo "{\"module\":\"$MOD\",\"passed\":false}"
  exit 0
fi

cd "$MODDIR"

# find module path of repo under test
MODPATH=$(go list -m -f '{{.Path}}' "$REPO" 2>/dev/null || true)
if [ -n "$MODPATH" ]; then
  echo "Replacing $MODPATH with local repo..."
  go mod edit -replace "$MODPATH=/work/proj"
fi

go mod tidy

if go build ./... && go test ./... ; then
  RESULT=true
else
  RESULT=false
fi

# restore stdout and print JSON
exec 1>&3
echo "{\"module\":\"$MOD\",\"passed\":$RESULT}"
