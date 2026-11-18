#!/usr/bin/env bash
set -euo pipefail

tag="v$VERSION"
release="repos/$GITHUB_REPOSITORY/releases/$RELEASE_ID"
work=$(mktemp -d "$PWD/.addons-release.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

require_draft() {
    local state tag_sha
    state=$(gh api "$release" --jq '[.draft, .tag_name] | @tsv')
    if [[ "$state" != "true"$'\t'"$tag" ]]; then
        echo "Release $RELEASE_ID is not the draft for $tag" >&2
        exit 1
    fi
    tag_sha=$(git ls-remote --tags --refs origin "refs/tags/$tag" | cut -f1)
    if [[ "$tag_sha" != "$GITHUB_SHA" ]]; then
        echo "Tag $tag is not pinned to $GITHUB_SHA" >&2
        exit 1
    fi
}

check_assets() {
    gh api --paginate --slurp "$release/assets?per_page=100" | jq 'add // []' > "$work/remote.json"
    "$work/assets" check "$work/inventory.json" "$work/remote.json" "$1"
}

require_newest() {
    local published newest
    published=$(gh api --paginate "repos/$GITHUB_REPOSITORY/releases?per_page=100" \
        --jq '.[] | select(.draft == false and .prerelease == false) | .tag_name')
    newest=$(printf '%s\n%s\n' "$tag" "$published" |
        awk '/^v[0-9]+\.[0-9]+\.[0-9]+-addons(\.[0-9]+)?$/' | sort -V | tail -n 1)
    if [[ "$newest" != "$tag" ]]; then
        echo "Release $newest is already published; $tag cannot become Latest" >&2
        exit 1
    fi
}

upload_missing() {
    local name
    local -a batch=()
    check_assets "$1" > "$work/missing"
    while IFS= read -r name; do
        batch+=("$work/files/$name")
    done < "$work/missing"
    if (( ${#batch[@]} > 0 )); then
        require_draft
        gh release upload "$tag" --repo "$GITHUB_REPOSITORY" "${batch[@]}"
    fi
}

require_draft
go build -o "$work/assets" ./.github/scripts/addons-assets
"$work/assets" prepare release-builds "$work/files" "$work/inventory.json" "$GITHUB_REPOSITORY" "$VERSION"
upload_missing payloads
upload_missing manifests
check_assets complete
require_draft
require_newest
gh api "$release" --method PATCH -F draft=false -F prerelease=false -f make_latest=true --jq .html_url
