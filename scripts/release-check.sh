#!/usr/bin/env bash
# Verify a commit exactly as it will be tagged, from a clean checkout.
#
#   scripts/release-check.sh            # checks HEAD
#   scripts/release-check.sh v0.4.1     # checks a tag or any commit
#
# v0.4.0 shipped without two files because checks ran on the working tree,
# which still had them. A worktree of the commit contains only what is
# committed, so missing files fail here instead of after release.
set -euo pipefail

ref="${1:-HEAD}"
repo="$(git rev-parse --show-toplevel)"
dir="$(mktemp -d)"
trap 'git -C "$repo" worktree remove --force "$dir" >/dev/null 2>&1 || rm -rf "$dir"' EXIT

git -C "$repo" worktree add --detach -q "$dir" "$ref"
cd "$dir"
echo "== checking $(git rev-parse --short HEAD) ($ref) in clean worktree"

unformatted="$(gofmt -l . | grep -v '^tools/yoga-oracle/' || true)"
if [ -n "$unformatted" ]; then
  echo "gofmt needed:"; echo "$unformatted"; exit 1
fi
go vet ./...
if command -v staticcheck >/dev/null; then
  for goos in linux darwin windows freebsd; do GOOS=$goos staticcheck ./...; done
  (cd tools/ptycheck && staticcheck ./...)
else
  echo "warning: staticcheck not installed; CI runs it"
fi
go test -count=1 -race ./...
go build ./...
(cd tools/ptycheck && go vet ./... && go test -count=1 -race ./...)
echo "== windows cross-compile (tests run on Windows CI)"
GOOS=windows GOARCH=amd64 go vet ./...
GOOS=windows GOARCH=amd64 go build ./...
GOOS=windows GOARCH=amd64 go build ./examples/...
(cd tools/ptycheck && GOOS=windows GOARCH=amd64 go vet ./...)
echo "== release check passed for $ref"
