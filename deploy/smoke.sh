#!/bin/sh
# Read-only checks for a deployed site. Usage: sh deploy/smoke.sh https://hadir.example.org
set -eu

base=${1:?usage: sh deploy/smoke.sh https://hadir.example.org}
base=${base%/}
case "$base" in
    https://*|http://127.0.0.1:*|http://localhost:*) ;;
    *) echo "Use HTTPS for deployment, or localhost HTTP for a local test." >&2; exit 2 ;;
esac

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

get_ok() {
    curl --silent --show-error --fail --max-time 15 \
        --dump-header "$tmp_dir/headers" --output "$tmp_dir/body" "$base$1"
}

get_ok /healthz
grep -q '^ok' "$tmp_dir/body"
get_ok /readyz
grep -q '"status":"ok"' "$tmp_dir/body"
get_ok /
grep -q 'id="root"' "$tmp_dir/body"
get_ok /manifest.webmanifest
grep -q '"short_name"' "$tmp_dir/body"
grep -qi '^content-type: application/manifest+json' "$tmp_dir/headers"
get_ok /sw.js
grep -q 'addEventListener' "$tmp_dir/body"
get_ok /icon-192.png
get_ok /icon-512.png

code=$(curl --silent --show-error --max-time 15 --output "$tmp_dir/body" \
    --write-out '%{http_code}' "$base/api/auth/me")
[ "$code" = 401 ] || { echo "Unauthenticated /api/auth/me returned $code, expected 401" >&2; exit 1; }

code=$(curl --silent --show-error --max-time 15 --output "$tmp_dir/body" \
    --write-out '%{http_code}' --request POST \
    --header "Origin: $base" --header 'Content-Type: application/json' \
    --data '{"email":"smoke@example.invalid","password":"invalid"}' \
    "$base/api/auth/login")
[ "$code" = 401 ] || { echo "Same-origin login check returned $code, expected 401" >&2; exit 1; }

code=$(curl --silent --show-error --max-time 15 --output "$tmp_dir/body" \
    --write-out '%{http_code}' --request POST \
    --header 'Origin: https://not-the-site.example' --header 'Content-Type: application/json' \
    --data '{"email":"smoke@example.invalid","password":"invalid"}' \
    "$base/api/auth/login")
[ "$code" = 403 ] || { echo "Cross-origin login check returned $code, expected 403" >&2; exit 1; }

echo "Smoke test passed: web, API, PWA assets, and origin guard at $base"
