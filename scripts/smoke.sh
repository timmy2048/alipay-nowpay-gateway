#!/usr/bin/env bash
set -euo pipefail
BASE="${BASE:-http://127.0.0.1:43123}"
JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT

json() { python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]] if len(sys.argv)>1 else "")' "$1"; }

echo "health"
curl -fsS "$BASE/api/health" | python3 -m json.tool

echo "login"
curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d '{"email":"admin@yin.local","password":"changeme-yin","portal":"admin"}' \
  "$BASE/api/auth/login" >/dev/null

STAMP="$(date +%s)"
CREATOR=$(curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d "{\"name\":\"北窗写作者\",\"contact_email\":\"north${STAMP}@example.com\",\"portal_email\":\"north${STAMP}@example.com\",\"portal_password\":\"portal-pass-1\",\"portal_name\":\"北窗\"}" \
  "$BASE/api/admin/creators")
CID=$(printf '%s' "$CREATOR" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')

PROJECT=$(curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d "{\"creator_id\":\"$CID\",\"name\":\"写作助手\",\"customer_price_yuan\":\"29.90\",\"seat_price_yuan\":\"199.00\",\"commission_percent\":\"2.50\",\"billing_mode\":\"ONE_TIME\",\"callback_url\":\"https://creator.example/return\",\"notify_url\":\"\"}" \
  "$BASE/api/admin/projects")
PID=$(printf '%s' "$PROJECT" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')

SEAT=$(curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d '{"pay_now":true,"channel":"admin_recorded"}' \
  "$BASE/api/admin/projects/$PID/seats")
SID=$(printf '%s' "$SEAT" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
STATUS=$(printf '%s' "$SEAT" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
test "$STATUS" = "paid_unbound"

curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' -d '{}' \
  "$BASE/api/admin/seats/$SID/bind" >/dev/null
BOUND=$(curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d '{"price_yuan":"29.90","logon_id":"13800138000"}' \
  "$BASE/api/admin/seats/$SID/bind/complete")
printf '%s' "$BOUND" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="bound", d; assert d["capability_status"]=="ENABLED", d; assert "138****8000"==d["alipay_logon_masked"], d'

PUR=$(curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d '{"external_buyer_id":"buyer_smoke"}' \
  "$BASE/api/admin/seats/$SID/purchases")
printf '%s' "$PUR" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="paid"; assert d["gross_cents"]==2990; assert d["commission_cents"]==75; assert d["decision"]=="ALLOW", d'

echo "second seat uses a new price"
curl -fsS -c "$JAR" -b "$JAR" -X PUT -H 'Content-Type: application/json' \
  -d "{\"creator_id\":\"$CID\",\"name\":\"写作助手\",\"status\":\"active\",\"customer_price_yuan\":\"29.90\",\"seat_price_yuan\":\"299.00\",\"commission_percent\":\"2.50\",\"billing_mode\":\"ONE_TIME\",\"callback_url\":\"https://creator.example/return\",\"notify_url\":\"\"}" \
  "$BASE/api/admin/projects/$PID" >/dev/null
SEAT2=$(curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d '{"pay_now":true}' \
  "$BASE/api/admin/projects/$PID/seats")
printf '%s' "$SEAT2" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["amount_cents"]==29900, d; assert d["ordinal"]==2, d'

echo "creator portal"
curl -fsS -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' \
  -d "{\"email\":\"north${STAMP}@example.com\",\"password\":\"portal-pass-1\",\"portal\":\"creator\"}" \
  "$BASE/api/auth/login" >/dev/null
curl -fsS -b "$JAR" "$BASE/api/creator/projects" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert len(d["projects"])==1'

echo "smoke ok"
