#!/usr/bin/env bash
#
# Phase 8 representative end-to-end demo.
#
# Assumes the API is running on $BASE_URL and that `cmd/seed` has been run
# (so the well-known demo accounts exist). The script does NOT promote
# roles — it relies on the seeded admin/agent/customer rows.

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
PW="${SEED_ADMIN_PASSWORD:-password123}"

trap 'echo "demo_flow.sh failed on line $LINENO" >&2' ERR

req() { curl -sS -w '\n%{http_code}' "$@"; }
http_status() { echo "$1" | tail -n1; }
http_body()   { echo "$1" | sed '$d'; }

expect_status() {
  if [[ "$1" != "$2" ]]; then
    echo "FAIL: $3: expected HTTP $2, got HTTP $1"
    exit 1
  fi
}

login() {
  local email="$1" r
  r=$(req -X POST "${BASE_URL}/api/v1/auth/login" -H 'Content-Type: application/json' \
       -d "$(jq -n --arg email "$email" --arg pw "$PW" '{email:$email,password:$pw}')")
  expect_status "$(http_status "$r")" 200 "login $email"
  http_body "$r" | jq -er '.data.access_token'
}

echo "==> 1. Login as customer"
CUST=$(login "user1@example.com")

echo "==> 2. Create a ticket"
CAT=$(curl -sS "${BASE_URL}/api/v1/categories" -H "Authorization: Bearer ${CUST}" \
  | jq -er '.data[] | select(.slug=="technical-issue") | .id')
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets" \
  -H "Authorization: Bearer ${CUST}" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg cat "$CAT" '{title:"Phase 8 demo flow", description:"Walking the whole ticket lifecycle end to end.", category_id:$cat, priority:"high"}')")
expect_status "$(http_status "$RESP")" 201 "create ticket"
TICKET_ID=$(http_body "$RESP" | jq -er '.data.id')
echo "   created ticket ${TICKET_ID}"

echo "==> 3. Login as admin, assign agent1"
ADMIN=$(login "admin@example.com")
AGENT_ID=$(curl -sS "${BASE_URL}/api/v1/dashboard/summary" -H "Authorization: Bearer ${ADMIN}" >/dev/null && \
  psql "${DATABASE_URL:-postgres://ticketing_user:ticketing_password@localhost:5432/ticketing_db?sslmode=disable}" -tAc "SELECT id FROM users WHERE email='agent1@example.com'")
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/assign" \
  -H "Authorization: Bearer ${ADMIN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid "$AGENT_ID" '{agent_id:$uid, note:"Demo flow"}')")
expect_status "$(http_status "$RESP")" 200 "assign agent"

echo "==> 4. Login as agent, move to in_progress and comment"
AGENT=$(login "agent1@example.com")
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${AGENT}" -H 'Content-Type: application/json' \
  -d '{"status":"in_progress","note":"Investigating"}')
expect_status "$(http_status "$RESP")" 200 "in_progress"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" \
  -H "Authorization: Bearer ${AGENT}" -H 'Content-Type: application/json' \
  -d '{"body":"Looking into the login flow."}')
expect_status "$(http_status "$RESP")" 201 "agent comment"

echo "==> 5. Customer comments + uploads a PNG attachment"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" \
  -H "Authorization: Bearer ${CUST}" -H 'Content-Type: application/json' \
  -d '{"body":"Thanks, sharing a screenshot."}')
expect_status "$(http_status "$RESP")" 201 "customer comment"

TMP_DIR=$(mktemp -d); trap 'rm -rf "$TMP_DIR"' EXIT
PNG="$TMP_DIR/demo.png"
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89' > "$PNG"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/attachments" \
  -H "Authorization: Bearer ${CUST}" -F "file=@${PNG}")
expect_status "$(http_status "$RESP")" 201 "upload"

echo "==> 6. Agent marks resolved"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${AGENT}" -H 'Content-Type: application/json' \
  -d '{"status":"resolved"}')
expect_status "$(http_status "$RESP")" 200 "resolved"

echo "==> 7. Admin closes"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${ADMIN}" -H 'Content-Type: application/json' \
  -d '{"status":"closed"}')
expect_status "$(http_status "$RESP")" 200 "closed"

echo "==> 8. Customer reopens"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${CUST}" -H 'Content-Type: application/json' \
  -d '{"status":"reopened","note":"Issue returned"}')
expect_status "$(http_status "$RESP")" 200 "reopened"

echo "==> 9. Read timeline (histories + comments)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/timeline" -H "Authorization: Bearer ${CUST}")
expect_status "$(http_status "$RESP")" 200 "timeline"
COUNT=$(http_body "$RESP" | jq '.data | length')
echo "   timeline carries ${COUNT} items"

echo "==> 10. Dashboard with SLA + unread notifications"
RESP=$(req "${BASE_URL}/api/v1/dashboard/summary" -H "Authorization: Bearer ${ADMIN}")
expect_status "$(http_status "$RESP")" 200 "dashboard"
http_body "$RESP" | jq '.data | {total_tickets, sla, unread_notifications}'

echo "==> 11. Agent unread notifications"
RESP=$(req "${BASE_URL}/api/v1/notifications?unread_only=true" -H "Authorization: Bearer ${AGENT}")
expect_status "$(http_status "$RESP")" 200 "notifications"
http_body "$RESP" | jq '.meta'

echo
echo "demo_flow.sh complete — ticket ${TICKET_ID} walked through every phase."
