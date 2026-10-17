#!/bin/sh
# Run before committing: format, vet, tests, and a Windows build (where the static plays).
# ./check.sh fuzz   also spends 15s per fuzz test hunting crashes.
set -e
cd "$(dirname "$0")"
unformatted=$(gofmt -l .)
[ -z "$unformatted" ] || { echo "gofmt needed: $unformatted"; exit 1; }
go vet ./...
go test ./...
GOOS=windows GOARCH=amd64 go build -o /dev/null ./...
if [ "$1" = fuzz ]; then
  for t in FuzzSplitter FuzzUnpack FuzzParseDamage; do go test ./wire -run '^$' -fuzz="^$t\$" -fuzztime=15s; done
  go test ./tcp -run '^$' -fuzz='^FuzzStream$' -fuzztime=15s
fi
echo "all good 🌳"
