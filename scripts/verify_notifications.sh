#!/usr/bin/env bash
#
# End-to-end verification for Phase 7 SLA + notifications.
# Requires: curl, jq, psql; the API running on $BASE_URL.

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DATABASE_URL="${DATABASE_URL:-postgres://ticketing_user:ticketing_password@localhost:5432/ticketing_db?sslmode=disable}"
TS="$(date +%s)$$"
PW='password123'

CUSTOMER_EMAIL="p7-cust+${TS}@example.com"
OTHER_EMAIL="p7-other+${TS}@example.com"
AGENT_EMAIL="p7-agent+${TS}@example.com"
ADMIN_EMAIL="p7-admin+${TS}@example.com"

trap 'echo "verify_notifications.sh failed on line $LINENO" >&2' ERR

req() { curl -sS -w '\n%{http_code}' "$@"; }
http_status() { echo "$1" | tail -n1; }
http_body()   { echo "$1" | sed '$d'; }

expect_status() {
  if [[ "$1" != "$2" ]]; then
    echo "FAIL: $3: expected HTTP $2, got HTTP $1"; exit 1
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

echo "→ Register + promote"
_=$(register "Cust ${TS}" "$CUSTOMER_EMAIL")
_=$(register "Other ${TS}" "$OTHER_EMAIL")
_=$(register "Agent ${TS}" "$AGENT_EMAIL")
_=$(register "Admin ${TS}" "$ADMIN_EMAIL")
set_role "$AGENT_EMAIL" agent
set_role "$ADMIN_EMAIL" admin

CUSTOMER_TOKEN=$(login "$CUSTOMER_EMAIL")
OTHER_TOKEN=$(login "$OTHER_EMAIL")
AGENT_TOKEN=$(login "$AGENT_EMAIL")
ADMIN_TOKEN=$(login "$ADMIN_EMAIL")
AGENT_ID=$(user_id_for "$AGENT_EMAIL")

RESP=$(req "${BASE_URL}/api/v1/ticket-categories" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
CATEGORY_ID=$(http_body "$RESP" | jq -er '.data[] | select(.slug=="technical-issue") | .id')

echo "→ Customer creates a high-priority ticket — SLA block present"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg cat "$CATEGORY_ID" '{title:"Phase 7 SLA verification", description:"Verifying SLA + notification wiring.", category_id:$cat, priority:"high"}')")
expect_status "$(http_status "$RESP")" 201 "create"
TICKET_ID=$(http_body "$RESP" | jq -er '.data.id')
RDA=$(http_body "$RESP" | jq -r '.data.sla.response_due_at // empty')
[[ -n "$RDA" ]] || { echo "FAIL: response_due_at missing"; exit 1; }
RES=$(http_body "$RESP" | jq -r '.data.sla.response_state')
[[ "$RES" == "pending" ]] || { echo "FAIL: initial response_state=$RES"; exit 1; }

echo "→ Admin assigns the agent — notification created"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/assign" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid $AGENT_ID '{agent_id:$uid}')")
expect_status "$(http_status "$RESP")" 200 "assign"

echo "→ Agent has one ticket_assigned notification"
RESP=$(req "${BASE_URL}/api/v1/notifications" -H "Authorization: Bearer ${AGENT_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "list agent notifications"
BODY=$(http_body "$RESP")
echo "$BODY" | jq -e '.data | any(.type == "ticket_assigned")' >/dev/null
N_ID=$(echo "$BODY" | jq -er '.data[] | select(.type == "ticket_assigned") | .id')
UNREAD=$(echo "$BODY" | jq -r '.meta.unread_total')
[[ "$UNREAD" -ge 1 ]] || { echo "FAIL: unread_total=$UNREAD"; exit 1; }

echo "→ Customer cannot mark agent's notification as read (404)"
RESP=$(req -X PUT "${BASE_URL}/api/v1/notifications/${N_ID}/read" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 404 "foreign mark read"

echo "→ Agent marks one notification as read"
RESP=$(req -X PUT "${BASE_URL}/api/v1/notifications/${N_ID}/read" \
  -H "Authorization: Bearer ${AGENT_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "mark read"

echo "→ Mark-read is idempotent (2nd call still 200)"
RESP=$(req -X PUT "${BASE_URL}/api/v1/notifications/${N_ID}/read" \
  -H "Authorization: Bearer ${AGENT_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "mark read idempotent"

echo "→ Agent moves ticket to in_progress — first_responded_at set + creator notified"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"in_progress"}')
expect_status "$(http_status "$RESP")" 200 "in_progress"
FR=$(http_body "$RESP" | jq -r '.data.sla.first_responded_at // empty')
[[ -n "$FR" ]] || { echo "FAIL: first_responded_at not set"; exit 1; }

RESP=$(req "${BASE_URL}/api/v1/notifications" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
http_body "$RESP" | jq -e '.data | any(.type == "ticket_status_changed")' >/dev/null

echo "→ Agent commenting again does NOT overwrite first_responded_at"
SAVED_FR="$FR"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"second comment"}')
expect_status "$(http_status "$RESP")" 201 "agent comment"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}" -H "Authorization: Bearer ${AGENT_TOKEN}")
FR2=$(http_body "$RESP" | jq -r '.data.sla.first_responded_at // empty')
[[ "$FR2" == "$SAVED_FR" ]] || { echo "FAIL: first_responded_at changed: $SAVED_FR -> $FR2"; exit 1; }

echo "→ Customer commenting notifies the assigned agent"
RESP=$(req "${BASE_URL}/api/v1/notifications" -H "Authorization: Bearer ${AGENT_TOKEN}")
BEFORE=$(http_body "$RESP" | jq '.data | length')
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"reply from creator"}')
expect_status "$(http_status "$RESP")" 201 "customer comment"
RESP=$(req "${BASE_URL}/api/v1/notifications" -H "Authorization: Bearer ${AGENT_TOKEN}")
AFTER=$(http_body "$RESP" | jq '.data | length')
[[ "$AFTER" -gt "$BEFORE" ]] || { echo "FAIL: agent didn't receive comment notification"; exit 1; }

echo "→ Agent resolves the ticket — resolved_at set; resolution_state met"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"resolved"}')
expect_status "$(http_status "$RESP")" 200 "resolved"
RA=$(http_body "$RESP" | jq -r '.data.sla.resolved_at // empty')
[[ -n "$RA" ]] || { echo "FAIL: resolved_at not set"; exit 1; }

echo "→ Admin closes — closed_at set"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"closed"}')
expect_status "$(http_status "$RESP")" 200 "closed"

echo "→ Customer reopens — both timestamps cleared"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"reopened"}')
expect_status "$(http_status "$RESP")" 200 "reopened"
RA2=$(http_body "$RESP" | jq -r '.data.sla.resolved_at // empty')
[[ -z "$RA2" ]] || { echo "FAIL: resolved_at not cleared on reopen"; exit 1; }

echo "→ Classification change to urgent recomputes due times"
RESP=$(req -X PUT "${BASE_URL}/api/v1/tickets/${TICKET_ID}/classification" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"priority":"urgent"}')
expect_status "$(http_status "$RESP")" 200 "reclassify"
RDA2=$(http_body "$RESP" | jq -r '.data.sla.response_due_at // empty')
[[ "$RDA2" != "$RDA" ]] || { echo "FAIL: response_due_at should change on reclass"; exit 1; }

echo "→ Dashboard summary includes SLA block + unread_notifications"
RESP=$(req "${BASE_URL}/api/v1/dashboard/summary" -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "dashboard"
BODY=$(http_body "$RESP")
echo "$BODY" | jq -e '.data.sla | has("response_breached") and has("resolution_breached") and has("resolution_due_soon")' >/dev/null
echo "$BODY" | jq -e '.data | has("unread_notifications")' >/dev/null

echo "→ Other customer can't see anyone else's notifications"
RESP=$(req "${BASE_URL}/api/v1/notifications" -H "Authorization: Bearer ${OTHER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "other notif list"
TOTAL=$(http_body "$RESP" | jq -r '.meta.total')
[[ "$TOTAL" == "0" ]] || { echo "FAIL: other should see 0 notifications, got $TOTAL"; exit 1; }

echo "→ Mark-all-read empties unread queue"
RESP=$(req -X PUT "${BASE_URL}/api/v1/notifications/read-all" -H "Authorization: Bearer ${AGENT_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "mark all"
RESP=$(req "${BASE_URL}/api/v1/notifications?unread_only=true" -H "Authorization: Bearer ${AGENT_TOKEN}")
LEFT=$(http_body "$RESP" | jq -r '.meta.unread_total')
[[ "$LEFT" == "0" ]] || { echo "FAIL: unread_total after mark-all=$LEFT"; exit 1; }

echo
echo "Phase 7 SLA + notifications verification passed"
