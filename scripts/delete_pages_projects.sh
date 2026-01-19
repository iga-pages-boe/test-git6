#!/usr/bin/env bash
set -euo pipefail
ENDPOINT='https://v-vconsole.bytedance.net/api/top/dcdn/cn-north-1/2021-04-01/DeletePagesProject'
if [ -z "${X_CSRF_TOKEN:-}" ] || [ -z "${COOKIE:-}" ]; then
  echo "请先设置环境变量 X_CSRF_TOKEN 和 COOKIE" >&2
  exit 1
fi
python3 scripts/find_deploy_failed_ids.py iga_with_condition_routes.json | while read -r id; do
  [ -z "$id" ] && continue
  curl --location "$ENDPOINT" \
    --header 'accept: application/json' \
    --header 'content-type: application/json; charset=UTF-8' \
    --header "x-csrf-token: $X_CSRF_TOKEN" \
    --header "Cookie: $COOKIE" \
    --data "{\"ID\": \"$id\"}"
done
