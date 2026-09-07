#!/usr/bin/env bash
# check-docpaths: every repo path a doc or a comment names must exist. Both
# cite code by path — `internal/local/store`, `plugins/fs`,
# `apps/desktop/...` — and those paths rot silently when code moves. A stale
# path is a false fact about where something lives; this gate is what checks
# it.
#
# Scope: every tracked *.md, *.go and *.ts, the go.mod/go.work files, and
# .github/workflows/*.yml. A token is path-like when it starts with one of the
# top-level dirs (internal/ plugins/ api/ client/ apps/ scripts/ test/ web/
# docs/) and continues with path characters; a trailing `/` or `/...` is fine,
# trailing punctuation and a `.GoSymbol` suffix (`internal/dbformat.
# EnsureVersion`) are stripped. Each must satisfy `test -e` from the repo
# root. A lowercase symbol suffix is NOT stripped, because `columns.go` is a
# file and `internal/cli.resolveBinary` is not — write those the other way
# round ("resolveBinary in internal/cli").
#
# Exemptions go in scripts/docpaths-allow.txt as "<file> <path>" lines
# with a comment saying why the path is gone.
set -euo pipefail
cd "$(dirname "$0")/.."

allow=scripts/docpaths-allow.txt
skip_files=()

# allowed compares the two fields LITERALLY. Matching an exemption as a
# regex silently failed to exempt any path containing a glob character —
# `apps/desktop/out/*.dmg` reads as "out, some slashes, any char, dmg" —
# and a gate that cannot be exempted is a gate people route around.
allowed() {
  while read -r af ap _; do
    case "$af" in \#* | '') continue ;; esac
    if [ "$af" = "$1" ] && [ "$ap" = "$2" ]; then return 0; fi
  done <"$allow"
  return 1
}

files=$(git ls-files '*.md' '*.go' '*.ts' 'go.mod' '*/go.mod' 'go.work' \
  '.github/workflows/*.yml' | grep -v '/node_modules/')
bad=0
for f in $files; do
  for s in "${skip_files[@]}"; do [ "$f" = "$s" ] && continue 2; done
  # Path-like tokens; the leading dir must sit on a word boundary so
  # `desktop/` inside `apps/desktop/` (say) is not split in two.
  for p in $(grep -oE '(^|[^A-Za-z0-9_./-])(internal|plugins|api|client|apps|scripts|test|web|docs)/[A-Za-z0-9_./*-]*' "$f" \
      | sed -E 's/^[^A-Za-z]//' | sed -E 's#/\.\.\.$##; s#[.,;:)*]+$##; s#\.[A-Z][A-Za-z0-9]*$##; s#/$##' | LC_ALL=C sort -u); do
    [ -z "$p" ] && continue
    if [ -e "$p" ]; then continue; fi
    if allowed "$f" "$p"; then continue; fi
    echo "$f: path does not exist: $p"
    bad=1
  done
done
if [ "$bad" != 0 ]; then
  echo "a doc or comment cites a path that no longer exists — fix it, or record a deliberately historical mention in $allow"
  exit 1
fi
echo "check-docpaths: ok"
