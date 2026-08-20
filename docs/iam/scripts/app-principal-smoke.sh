#!/usr/bin/env bash
# Application principal 冒烟（选项 A）
# 用法：
#   export API=http://localhost:8080
#   export ADMIN_TOKEN=...   # 可选，用于创建 app / 分配 Role
#   export APP_KEY=... APP_SECRET=...
#   ./docs/iam/scripts/app-principal-smoke.sh
# 授权：POST /api/v1/iam/applications/$APP_KEY/roles（Direct Grant 已 410，见 docs/iam/39）
set -euo pipefail

API="${API:-http://localhost:8080}"
fail=0

check() {
  local name="$1" expect="$2" got="$3"
  if [[ "$got" == "$expect" ]]; then
    echo "OK  $name (HTTP $got)"
  else
    echo "FAIL $name want=$expect got=$got"
    fail=1
  fi
}

if [[ -z "${APP_KEY:-}" || -z "${APP_SECRET:-}" ]]; then
  echo "请设置 APP_KEY / APP_SECRET（管理台创建应用后）"
  exit 2
fi

HDR=(-H "X-App-Key: ${APP_KEY}" -H "X-App-Secret: ${APP_SECRET}")

code=$(curl -sS -o /tmp/app_whoami.json -w "%{http_code}" "${HDR[@]}" "$API/api/v1/app/whoami" || true)
check "whoami" "200" "$code"

code=$(curl -sS -o /tmp/app_ws_list.json -w "%{http_code}" "${HDR[@]}" "$API/api/v1/app/workspaces" || true)
# 有 WORKSPACES grant → 200；无 grant → 403
if [[ "$code" != "200" && "$code" != "403" ]]; then
  echo "FAIL list workspaces unexpected HTTP $code"
  fail=1
else
  echo "OK  list workspaces (HTTP $code — 200=有grant 403=无grant)"
fi

if [[ -n "${WS_ID:-}" ]]; then
  code=$(curl -sS -o /tmp/app_ws_get.json -w "%{http_code}" "${HDR[@]}" "$API/api/v1/app/workspaces/${WS_ID}" || true)
  echo "get workspace $WS_ID → HTTP $code (200 匹配 / 403 无权 / 404 跨org或tag不匹配)"
fi

# 错误密钥
code=$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "X-App-Key: ${APP_KEY}" -H "X-App-Secret: wrong-secret" \
  "$API/api/v1/app/whoami" || true)
check "bad secret" "401" "$code"

exit $fail
