#!/usr/bin/env bash
# Run from the repo root. Git and GitHub operations are intercepted.
set -euo pipefail
scripts="$PWD/.github/scripts"
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
cd "$work"
export GITHUB_REPOSITORY=test/music GITHUB_SHA=checkout VERSION=0.1.25-addons.3 RELEASE_ID=4
export GITHUB_OUTPUT="$work/output"
export TRACE="$work/trace"

printf '  version: "0.1.25",\n' > mygo.config.ts
git() {
    case "$1" in
        fetch) [[ "$*" = "fetch --no-tags https://github.com/egoist/mygo-music.git refs/tags/v0.1.25" ]] ;;
        rev-parse) printf 'upstream\n' ;;
        merge-base) [[ "$*" = "merge-base --is-ancestor upstream checkout" && "${CONTAINS_BASE:-yes}" = yes ]] ;;
        ls-remote)
            if [[ "$*" = *'refs/tags/v0.1.25-addons*' ]]; then
                printf 'checkout\trefs/tags/v0.1.25-addons\n'
            else
                [[ "$*" = *"refs/tags/v$VERSION" ]] || return 1
                printf 'checkout\trefs/tags/v%s\n' "$VERSION"
            fi ;;
        push) printf '%s\n' "$*" >> "$TRACE" ;;
        *) return 1 ;;
    esac
}
gh() {
    case "$*" in
        "release view --repo egoist/mygo-music --json tagName --jq .tagName") echo v0.1.25 ;;
        "api --paginate repos/test/music/releases?per_page=100 --jq .[].tag_name") echo v0.1.25-addons.2 ;;
        "api repos/test/music/releases --method POST "*)
            [[ "$*" = *'tag_name=v0.1.25-addons.3'*'draft=true'*'0.1.25 (upstream)'* ]] || return 1
            echo 4 ;;
        "api repos/test/music/releases/4 --jq [.draft, .tag_name] | @tsv")
            printf '%s\tv%s\n' "${DRAFT:-true}" "$VERSION" ;;
        "api --paginate --slurp repos/test/music/releases/4/assets?per_page=100")
            printf '[[{"name":"payload"}],[{"name":"manifest"}]]\n' ;;
        "api --paginate repos/test/music/releases?per_page=100 --jq .[] | select(.draft == false and .prerelease == false) | .tag_name")
            printf 'v0.1.24-addons.99\nv0.1.25-addons\nv0.1.25-addons.2\n%s\n' "${NEWER_PUBLISHED:-}" ;;
        "release upload v0.1.25-addons.3 --repo test/music "*)
            [[ "$#" = 6 && "$*" != *--clobber* ]] || return 1
            printf 'upload %s\n' "${6##*/}" >> "$TRACE" ;;
        "api repos/test/music/releases/4 --method PATCH -F draft=false -F prerelease=false -f make_latest=true --jq .html_url")
            echo publish >> "$TRACE" ;;
        *) printf 'unexpected gh arguments: %s\n' "$*" >&2; return 1 ;;
    esac
}
jq() {
    local input
    input=$(cat)
    [[ "$#" = 1 && "$1" = 'add // []' && "$input" = '[[{"name":"payload"}],[{"name":"manifest"}]]' ]] || return 1
    printf '[{"name":"payload"},{"name":"manifest"}]\n'
}
go() {
    [[ "$*" = 'build -o '*'/assets ./.github/scripts/addons-assets' ]] || return 1
    cat > "$3" <<'ASSETS'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" = prepare ]]; then
    mkdir "$3"
    exit
fi
[[ "$(cat "$3")" = '[{"name":"payload"},{"name":"manifest"}]' ]]
case "$4" in
    payloads) echo payload ;;
    manifests) echo manifest ;;
    complete) echo checked >> "$TRACE" ;;
    *) exit 1 ;;
esac
ASSETS
    chmod +x "$3"
}

printf '  version: "0.1.24",\n' > mygo.config.ts
if ( source "$scripts/addons-draft.sh" ); then
    echo "An older checkout version was accepted" >&2
    exit 1
fi
test ! -e "$TRACE"
printf '  version: "0.1.25",\n' > mygo.config.ts
if ( export CONTAINS_BASE=no; source "$scripts/addons-draft.sh" ); then
    echo "A checkout without the upstream release was accepted" >&2
    exit 1
fi
test ! -e "$TRACE"
( source "$scripts/addons-draft.sh" )
grep -Fx 'version=0.1.25-addons.3' "$GITHUB_OUTPUT"
grep -Fx 'release_id=4' "$GITHUB_OUTPUT"
test "$(cat "$TRACE")" = 'push origin checkout:refs/tags/v0.1.25-addons.3'

: > "$TRACE"
if ( export DRAFT=false; source "$scripts/addons-publish.sh" ); then
    echo "A published release was accepted for upload" >&2
    exit 1
fi
test ! -s "$TRACE"
( source "$scripts/addons-publish.sh" )
test "$(cat "$TRACE")" = $'upload payload\nupload manifest\nchecked\npublish'

: > "$TRACE"
if ( export NEWER_PUBLISHED=v0.1.25-addons.10; source "$scripts/addons-publish.sh" ); then
    echo "An earlier draft was allowed to take Latest" >&2
    exit 1
fi
test "$(cat "$TRACE")" = $'upload payload\nupload manifest\nchecked'
echo "Offline Addons workflow checks passed"
