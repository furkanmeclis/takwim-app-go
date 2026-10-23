#!/usr/bin/env bash
# Tags a commit on main with the next patch version (v0.1.0, v0.1.1, …) and
# publishes a GitHub release with generated notes. Safe to run twice for the
# same commit: a commit that already carries a tag is skipped.
#
# Usage: GH_REPO=owner/repo GH_TOKEN=… scripts/release-tag.sh <commit-sha>
set -euo pipefail

sha="${1:?commit sha required}"
tags="$(gh api --paginate "repos/$GH_REPO/tags" --jq '.[] | "\(.name) \(.commit.sha)"')"

if grep -q " $sha\$" <<<"$tags"; then
  echo "$sha is already tagged; skipping."
  exit 0
fi

last="$(awk '{print $1}' <<<"$tags" | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n 1 || true)"
if [ -z "$last" ]; then
  next="v0.1.0"
else
  IFS=. read -r major minor patch <<<"${last#v}"
  next="v$major.$minor.$((patch + 1))"
fi

gh release create "$next" --target "$sha" --title "$next" --generate-notes
echo "Released $next at $sha"
