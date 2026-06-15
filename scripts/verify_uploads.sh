#!/usr/bin/env bash
#
# End-to-end verification for Phase 5 category admin + attachments.
# Requires: curl, jq, psql; the API running on $BASE_URL.

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DATABASE_URL="${DATABASE_URL:-postgres://ticketing_user:ticketing_password@localhost:5432/ticketing_db?sslmode=disable}"
TS="$(date +%s)$$"
PW='password123'

CUSTOMER_EMAIL="p5-cust+${TS}@example.com"
OTHER_EMAIL="p5-other+${TS}@example.com"
AGENT_EMAIL="p5-agent+${TS}@example.com"
ADMIN_EMAIL="p5-admin+${TS}@example.com"

trap 'echo "verify_uploads.sh failed on line $LINENO" >&2' ERR

req() { curl -sS -w '\n%{http_code}' "$@"; }
http_status() { echo "$1" | tail -n1; }
http_body()   { echo "$1" | sed '$d'; }

expect_status() {
  local got="$1" want="$2" ctx="$3"
  if [[ "$got" != "$want" ]]; then
    echo "FAIL: ${ctx}: expected HTTP ${want}, got HTTP ${got}"
    exit 1
  fi
}

register() {
  local r
  r=$(req -X POST "${BASE_URL}/api/v1/auth/register" -H 'Content-Type: application/json' \
       -d "$(jq -n --arg name "$1" --arg email "$2" --arg pw "$PW" '{name:$name,email:$email,password:$pw}')")
  expect_status "$(http_status "$r")" 201 "register $2"
  http_body "$r" | jq -er '.data.access_token'
}
login() {
  local r
  r=$(req -X POST "${BASE_URL}/api/v1/auth/login" -H 'Content-Type: application/json' \
       -d "$(jq -n --arg email "$1" --arg pw "$PW" '{email:$email,password:$pw}')")
  expect_status "$(http_status "$r")" 200 "login $1"
  http_body "$r" | jq -er '.data.access_token'
}
user_id_for() { psql "$DATABASE_URL" -tAc "SELECT id FROM users WHERE email='$1'"; }
set_role()    { psql "$DATABASE_URL" -c "UPDATE users SET role='$2' WHERE email='$1';" >/dev/null; }

echo "→ Register users"
_=$(register "Cust ${TS}" "$CUSTOMER_EMAIL")
_=$(register "Other ${TS}" "$OTHER_EMAIL")
_=$(register "Agent ${TS}" "$AGENT_EMAIL")
_=$(register "Admin ${TS}" "$ADMIN_EMAIL")

echo "→ Promote roles"
set_role "$AGENT_EMAIL" agent
set_role "$ADMIN_EMAIL" admin

CUSTOMER_TOKEN=$(login "$CUSTOMER_EMAIL")
OTHER_TOKEN=$(login "$OTHER_EMAIL")
AGENT_TOKEN=$(login "$AGENT_EMAIL")
ADMIN_TOKEN=$(login "$ADMIN_EMAIL")
AGENT_ID=$(user_id_for "$AGENT_EMAIL")

echo "→ Admin creates category"
SLUG="phase5-${TS}"
RESP=$(req -X POST "${BASE_URL}/api/v1/admin/categories" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg name "Phase5 ${TS}" --arg slug "$SLUG" '{name:$name, slug:$slug, description:"phase 5 verify"}')")
expect_status "$(http_status "$RESP")" 201 "admin create"
CATEGORY_ID=$(http_body "$RESP" | jq -er '.data.id')

echo "→ Non-admin cannot create category (403)"
RESP=$(req -X POST "${BASE_URL}/api/v1/admin/categories" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"name":"NoCust","slug":"nocust"}')
expect_status "$(http_status "$RESP")" 403 "customer create blocked"

echo "→ Admin lists ALL (including new)"
RESP=$(req "${BASE_URL}/api/v1/admin/categories" -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "admin list"
http_body "$RESP" | jq -e --arg id "$CATEGORY_ID" '.data | any(.id == $id)' >/dev/null

echo "→ Public list shows the active category"
RESP=$(req "${BASE_URL}/api/v1/categories" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "public list"
http_body "$RESP" | jq -e --arg id "$CATEGORY_ID" '.data | any(.id == $id)' >/dev/null

echo "→ Customer creates a ticket using the new category"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg cat "$CATEGORY_ID" '{title:"Upload flow test", description:"Verifying phase 5 attachments end-to-end.", category_id:$cat, priority:"medium"}')")
expect_status "$(http_status "$RESP")" 201 "create ticket"
TICKET_ID=$(http_body "$RESP" | jq -er '.data.id')

echo "→ Admin assigns the agent"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/assign" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid $AGENT_ID '{agent_id:$uid}')")
expect_status "$(http_status "$RESP")" 200 "assign"

echo "→ Assigned agent updates classification to urgent"
RESP=$(req -X PUT "${BASE_URL}/api/v1/tickets/${TICKET_ID}/classification" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"priority":"urgent"}')
expect_status "$(http_status "$RESP")" 200 "agent classify"
PRIO=$(http_body "$RESP" | jq -er '.data.priority')
[[ "$PRIO" == "urgent" ]] || { echo "FAIL: priority=$PRIO"; exit 1; }

echo "→ Customer cannot change priority via generic PATCH (403)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"priority":"low"}')
expect_status "$(http_status "$RESP")" 403 "customer locked out"

# Upload tests
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
PNG_FILE="$TMP_DIR/screen.png"
# Minimal PNG: header + IHDR chunk (33 bytes).
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89' > "$PNG_FILE"

echo "→ Customer uploads PNG attachment"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/attachments" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" \
  -F "file=@${PNG_FILE};filename=../../etc/passwd.png")
expect_status "$(http_status "$RESP")" 201 "upload png"
ATTACH_ID=$(http_body "$RESP" | jq -er '.data.id')
SAFE_NAME=$(http_body "$RESP" | jq -er '.data.original_filename')
case "$SAFE_NAME" in
  */* | *..* )
    echo "FAIL: original_filename leaked traversal: $SAFE_NAME"; exit 1 ;;
esac
echo "$SAFE_NAME" | grep -qv 'storage_path' # sanity check

echo "→ JSON response should NOT contain storage_path"
http_body "$RESP" | jq -e 'has("data") and (.data | has("storage_path") | not)' >/dev/null

echo "→ Unsupported MIME (zip) rejected (422)"
ZIP_FILE="$TMP_DIR/x.zip"
printf 'PK\x03\x04\x14\x00\x00\x00\x00\x00' > "$ZIP_FILE"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/attachments" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -F "file=@${ZIP_FILE}")
expect_status "$(http_status "$RESP")" 422 "zip rejected"

echo "→ Other customer cannot list attachments (404)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/attachments" -H "Authorization: Bearer ${OTHER_TOKEN}")
expect_status "$(http_status "$RESP")" 404 "other customer list"

echo "→ Other customer cannot download attachment (404)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/attachments/${ATTACH_ID}/download" -H "Authorization: Bearer ${OTHER_TOKEN}")
expect_status "$(http_status "$RESP")" 404 "other customer download"

echo "→ Customer downloads own attachment"
DL_FILE="$TMP_DIR/dl.png"
HTTP_CODE=$(curl -sS -o "$DL_FILE" -w '%{http_code}' \
  "${BASE_URL}/api/v1/tickets/${TICKET_ID}/attachments/${ATTACH_ID}/download" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$HTTP_CODE" 200 "download"
cmp -s "$PNG_FILE" "$DL_FILE" || { echo "FAIL: downloaded bytes don't match upload"; exit 1; }

echo "→ Customer deletes own attachment"
RESP=$(req -X DELETE "${BASE_URL}/api/v1/tickets/${TICKET_ID}/attachments/${ATTACH_ID}" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 204 "delete attachment"

echo "→ Admin deactivates the category (200)"
RESP=$(req -X DELETE "${BASE_URL}/api/v1/admin/categories/${CATEGORY_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "deactivate"

echo "→ Deactivated category no longer in public list"
RESP=$(req "${BASE_URL}/api/v1/categories" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "public list after deactivate"
http_body "$RESP" | jq -e --arg id "$CATEGORY_ID" '.data | all(.id != $id)' >/dev/null

echo
echo "Phase 5 categories + classification + attachments verification passed"
