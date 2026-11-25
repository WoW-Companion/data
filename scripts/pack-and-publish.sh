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

step "revision history"
# Every publish also keeps revision-named copies of the archive and manifest (the last 10
# per build) and records them in revisions-$BUILD.json; the unsuffixed names stay as they are.
TAG=dataset-$BUILD
HIST_PREV=$OUT/revisions-prev.json
rm -f "$HIST_PREV"
if [ "${REPUBLISH:-}" = true ]; then
  # Reading the old history is best effort: a release without one starts a new history.
  gh release download "$TAG" --dir "$OUT" --pattern "revisions-$BUILD.json" --clobber \
    && mv "$OUT/revisions-$BUILD.json" "$HIST_PREV" || echo "no earlier revision history"
fi
HIST=$(go run ./cmd/wowc-data history -prev "$HIST_PREV" -out "$OUT/revisions-$BUILD.json" "$DIST")
echo "$HIST"
REV=$(echo "$HIST" | awk '$1 == "revision" {print $2}')
PRUNE=$(echo "$HIST" | awk '$1 == "prune" {print $2}')
[ -n "$REV" ] || { echo "no revision in the manifest" >&2; exit 1; }
cp "$OUT/wowc-dataset-$BUILD.tar.zst" "$OUT/wowc-dataset-$BUILD-$REV.tar.zst"
cp "$OUT/manifest-$BUILD.json" "$OUT/manifest-$BUILD-$REV.json"
REV_ASSETS=(wowc-dataset-"$BUILD-$REV".tar.zst manifest-"$BUILD-$REV".json)

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
  # Exact names: the release also holds revision-named manifests (manifest-<build>-<rev>.json).
  PREV_BUILD=${PREV_TAG#dataset-}
  gh release download "$PREV_TAG" --dir "$OUT/prev" --pattern "manifest-$PREV_BUILD.json" --pattern "stats-$PREV_BUILD.json"
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
ASSETS=(wowc-dataset-"$BUILD".tar.zst manifest-"$BUILD".json coverage-"$BUILD".md stats-"$BUILD".json revisions-"$BUILD".json)
if [ "${REPUBLISH:-}" != true ]; then
  step "publish $TAG"
  gh release create "$TAG" "${ASSETS[@]/#/$OUT/}" "${REV_ASSETS[@]/#/$OUT/}" \
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
gh release upload "$TAG" "$STAGE"/new-* "${REV_ASSETS[@]/#/$OUT/}" --clobber
for a in "${ASSETS[@]}"; do
  new=$(gh_assets "new-$a"); old=$(gh_assets "$a")
  [ -n "$new" ] || { echo "temporary asset new-$a is missing; the release is unchanged" >&2; exit 1; }
  if [ -n "$old" ]; then rename_asset "$old" "old-$a"; fi
  rename_asset "$new" "$a"
  if [ -n "$old" ]; then gh api -X DELETE "repos/$GITHUB_REPOSITORY/releases/assets/$old" >/dev/null; fi
done
gh release edit "$TAG" --notes-file "$OUT/notes.md"
# Prune the revisions that fell out of the last 10; a missing asset is not an error.
for r in $PRUNE; do
  for n in "wowc-dataset-$BUILD-$r.tar.zst" "manifest-$BUILD-$r.json"; do
    gh release delete-asset "$TAG" "$n" --yes || echo "could not delete $n (already gone?)"
  done
done
