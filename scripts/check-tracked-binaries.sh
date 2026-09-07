#!/usr/bin/env bash
# check-tracked-binaries: no build output may be committed. The .gitignore
# list names files, so a rename that swaps those names in the same commit that
# leaves the old ones on disk lets `git add -A` sweep tens of megabytes of ELF
# into history. A tracked file that is an executable image, or larger than the
# cap, fails `make check` instead.
set -euo pipefail
cd "$(dirname "$0")/.."

CAP_BYTES=$((4 * 1024 * 1024))
bad=0
while IFS= read -r -d '' f; do
  [ -f "$f" ] || continue
  if head -c 4 "$f" | grep -q $'^\x7fELF' 2>/dev/null; then
    echo "tracked ELF binary: $f"; bad=1; continue
  fi
  size=$(stat -c %s "$f")
  if [ "$size" -gt "$CAP_BYTES" ]; then
    echo "tracked file over $((CAP_BYTES / 1024 / 1024)) MB: $f ($size bytes)"; bad=1
  fi
done < <(git ls-files -z)
exit $bad
