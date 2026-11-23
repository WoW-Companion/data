#!/usr/bin/env bash
# Packs dist/$BUILD, writes the release notes and publishes dataset-$BUILD.
# Used by the dataset workflow and by `make release-dryrun`.
#   BUILD      the game build (required)
#   DRYRUN=1   do everything except `gh release create`
#   PREV_TAG   the previous dataset release's tag; when the variable is set, even to
#              empty (the first release), the lookup through `gh` is skipped
#   REPUBLISH  "true" when dataset-$BUILD already exists: its assets are replaced (new
#              ones uploaded under temporary names, then swapped in) and its notes rewritten
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
ASSETS=(wowc-dataset-"$BUILD".tar.zst manifest-"$BUILD".json coverage-"$BUILD".md stats-"$BUILD".json)
TAG=dataset-$BUILD
if [ "${REPUBLISH:-}" != true ]; then
  step "publish $TAG"
  gh release create "$TAG" "${ASSETS[@]/#/$OUT/}" \
    --title "Dataset $BUILD" --notes-file "$OUT/notes.md" --target "${GITHUB_SHA:-main}"
  exit 0
fi

step "republish $TAG (same build, new revision)"
# Upload the new assets under temporary names first; only when all are up, swap each
# pair member by renaming, so the release never lacks a manifest or an archive.
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
gh_assets() { gh api --paginate "repos/$GITHUB_REPOSITORY/releases/tags/$TAG" --jq ".assets[] | select(.name == \"$1\") | .id"; }
rename_asset() { gh api -X PATCH "repos/$GITHUB_REPOSITORY/releases/assets/$1" -f name="$2" >/dev/null; }
STAGE=$OUT/stage && rm -rf "$STAGE" && mkdir "$STAGE"
for a in "${ASSETS[@]}"; do cp "$OUT/$a" "$STAGE/new-$a"; done
gh release upload "$TAG" "$STAGE"/new-* --clobber
for a in "${ASSETS[@]}"; do
  new=$(gh_assets "new-$a"); old=$(gh_assets "$a")
  [ -n "$new" ] || { echo "temporary asset new-$a is missing; the release is unchanged" >&2; exit 1; }
  if [ -n "$old" ]; then rename_asset "$old" "old-$a"; fi
  rename_asset "$new" "$a"
  if [ -n "$old" ]; then gh api -X DELETE "repos/$GITHUB_REPOSITORY/releases/assets/$old" >/dev/null; fi
done
gh release edit "$TAG" --notes-file "$OUT/notes.md"
