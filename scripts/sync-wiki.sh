#!/usr/bin/env bash
# Sync memory/ into the companion wiki repo.
#
# What it does:
#   1. Clones the wiki repo into $WORK_DIR
#   2. Copies every file under memory/ verbatim into the wiki working tree
#   3. Renames memory/README.md -> Home.md so it lands on the wiki front page
#   4. Pushes the result
#
# What it does NOT do:
#   - It does not rewrite memory/ in the main repo.
#   - It does not push the main repo. Run `git push` separately.
#
# Usage:
#   scripts/sync-wiki.sh [<main-repo-path>]
#
# Env overrides:
#   WIKI_REPO  default: git@github.com:gearshell/matrix.wiki.git
#   WIKI_BRANCH default: master
set -euo pipefail

MAIN_REPO="${1:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
MAIN_REPO="$(cd "$MAIN_REPO" && pwd)"
WIKI_REPO="${WIKI_REPO:-git@github.com:gearshell/matrix.wiki.git}"
WIKI_BRANCH="${WIKI_BRANCH:-master}"

if [[ ! -d "$MAIN_REPO/memory" ]]; then
  echo "error: $MAIN_REPO/memory not found" >&2
  exit 1
fi

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

echo "Cloning $WIKI_REPO ..."
git clone --depth 1 --branch "$WIKI_BRANCH" "$WIKI_REPO" "$WORK_DIR/wiki"

cd "$WORK_DIR/wiki"
git config user.email "crush@btwiuse.local"
git config user.name "Crush"

echo "Copying memory/ -> wiki working tree (README.md -> Home.md) ..."
# Clear any leftover files from the previous sync (except .git).
find . -mindepth 1 -maxdepth 1 ! -name '.git' -exec rm -rf {} +

shopt -s dotglob nullglob
items=("$MAIN_REPO"/memory/*)
if [[ ${#items[@]} -eq 0 ]]; then
  echo "error: no files found under $MAIN_REPO/memory" >&2
  exit 1
fi
for src in "${items[@]}"; do
  name="$(basename "$src")"
  if [[ "$name" == "README.md" ]]; then
    # Publish as the wiki front page (Home.md). README.md is not mirrored.
    cp "$src" ./Home.md
  else
    cp "$src" "./$name"
  fi
done
unset items

git add -A

if git diff --cached --quiet; then
  echo "No changes to sync."
  exit 0
fi

git commit -m "sync memory snapshot $(date -u +%Y-%m-%dT%H:%M:%SZ)"
git push origin "HEAD:$WIKI_BRANCH"
echo "Done. Wiki now at: $WIKI_REPO"
