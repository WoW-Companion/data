#!/usr/bin/env bash
# Packs dist/$BUILD, writes the release notes and publishes dataset-$BUILD.
# Used by the dataset workflow and by `make release-dryrun`.
#   BUILD      the game build (required)
#   DRYRUN=1   do everything except `gh release create`
#   PREV_TAG   the previous dataset release's tag; when the variable is set, even to
#              empty (the first release), the lookup through `gh` is skipped
#   OUT        where the release files are written (default: the current directory)
set -euo pipefail

: "${BUILD:?BUILD is required}"
OUT=${OUT:-.}
DIST=dist/$BUILD
step() { printf '\n==> %s\n' "$*"; }
mkdir -p "$OUT"

command -v zstd >/dev/null || { echo "zstd is not installed" >&2; exit 1; }

step "pack $DIST"
tar -C dist -cf - "$BUILD" | zstd -19 -T0 -f -o "$OUT/wowc-dataset-$BUILD.tar.zst"
cp "$DIST/manifest.json" "$OUT/manifest-$BUILD.json"
go run ./cmd/wowc-data coverage "$DIST" > "$OUT/coverage-$BUILD.md"
go run ./cmd/wowc-data coverage -json "$DIST" > "$OUT/stats-$BUILD.json"
ls -l "$OUT"/wowc-dataset-"$BUILD".tar.zst "$OUT"/manifest-"$BUILD".json "$OUT"/coverage-"$BUILD".md "$OUT"/stats-"$BUILD".json

step "previous release"
# The previous release is the newest dataset release other than this build's; its
# manifest and stats are all the diff needs. A release without stats still diffs
# tables and rows.
if [ -z "${PREV_TAG+x}" ]; then
  PREV_TAG=$(gh release list --limit 20 --json tagName,createdAt \
    --jq "[.[] | select(.tagName | startswith(\"dataset-\")) | select(.tagName != \"dataset-$BUILD\")] | sort_by(.createdAt) | last | .tagName // \"\"")
fi
echo "previous: ${PREV_TAG:-none (first release)}"

step "release notes"
if [ -n "$PREV_TAG" ]; then
  rm -rf "$OUT/prev" && mkdir "$OUT/prev"
  gh release download "$PREV_TAG" --dir "$OUT/prev" --pattern 'manifest-*.json' --pattern 'stats-*.json'
  mv "$OUT"/prev/manifest-*.json "$OUT/prev/manifest.json"
  mv "$OUT"/prev/stats-*.json "$OUT/prev/stats.json" 2>/dev/null || true
  go run ./cmd/wowc-data release-notes -prev-tag "$PREV_TAG" -prev-dir "$OUT/prev" "$DIST" > "$OUT/notes.md"
else
  go run ./cmd/wowc-data release-notes "$DIST" > "$OUT/notes.md"
fi
cat "$OUT/notes.md"

if [ "${DRYRUN:-}" = 1 ]; then
  step "dry run: not publishing dataset-$BUILD"
  exit 0
fi
step "publish dataset-$BUILD"
gh release create "dataset-$BUILD" "$OUT/wowc-dataset-$BUILD.tar.zst" "$OUT/manifest-$BUILD.json" "$OUT/coverage-$BUILD.md" "$OUT/stats-$BUILD.json" \
  --title "Dataset $BUILD" --notes-file "$OUT/notes.md" --target "${GITHUB_SHA:-main}"
