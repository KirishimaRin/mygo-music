#!/usr/bin/env bash
set -euo pipefail

upstream=egoist/mygo-music
upstream_tag=$(gh release view --repo "$upstream" --json tagName --jq .tagName)
if [[ ! "$upstream_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Expected a stable upstream tag, got $upstream_tag" >&2
    exit 1
fi
base=${upstream_tag#v}
source_version=$(tr -d '\r' < mygo.config.ts | sed -nE 's/^ +version: "([^"]+)",$/\1/p')
if [[ "$source_version" != "$base" ]]; then
    echo "Checkout version $source_version does not match upstream $upstream_tag" >&2
    exit 1
fi
git fetch --no-tags "https://github.com/$upstream.git" "refs/tags/$upstream_tag"
upstream_commit=$(git rev-parse 'FETCH_HEAD^{commit}')
if ! git merge-base --is-ancestor "$upstream_commit" "$GITHUB_SHA"; then
    echo "Checkout $GITHUB_SHA does not contain upstream $upstream_tag ($upstream_commit)" >&2
    exit 1
fi

# A new dispatch gets a fresh tag. A failed publish job can reuse its draft
# and the matrix artifacts from this run.
tags=$(git ls-remote --tags --refs origin "refs/tags/v$base-addons*" | cut -f2)
releases=$(gh api --paginate "repos/$GITHUB_REPOSITORY/releases?per_page=100" --jq '.[].tag_name')
version="$base-addons"
n=1
while grep -Fxq "refs/tags/v$version" <<< "$tags" ||
    grep -Fxq "v$version" <<< "$releases"; do
    n=$((n + 1))
    version="$base-addons.$n"
done
tag="v$version"
git push origin "$GITHUB_SHA:refs/tags/$tag"
release_id=$(gh api "repos/$GITHUB_REPOSITORY/releases" --method POST \
    -f tag_name="$tag" -f name="$tag" -F draft=true \
    -f body="MyGo Music $base ($upstream_commit) with this fork's additions: see [FORK.md](https://github.com/$GITHUB_REPOSITORY/blob/$tag/FORK.md)." \
    --jq '.id')
printf 'base=%s\nversion=%s\nrelease_id=%s\n' "$base" "$version" "$release_id" >> "$GITHUB_OUTPUT"
