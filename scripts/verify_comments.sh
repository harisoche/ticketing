#!/usr/bin/env bash
#
# End-to-end verification for Phase 4 comments + timeline.
# Requires: curl, jq, psql; the API running on $BASE_URL.

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DATABASE_URL="${DATABASE_URL:-postgres://ticketing_user:ticketing_password@localhost:5432/ticketing_db?sslmode=disable}"
TS="$(date +%s)$$"
PW='password123'

CUSTOMER_EMAIL="p4-cust+${TS}@example.com"
OTHER_EMAIL="p4-other+${TS}@example.com"
AGENT_EMAIL="p4-agent+${TS}@example.com"
AGENT2_EMAIL="p4-agent2+${TS}@example.com"
ADMIN_EMAIL="p4-admin+${TS}@example.com"

trap 'echo "verify_comments.sh failed on line $LINENO" >&2' ERR

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
set_role() { psql "$DATABASE_URL" -c "UPDATE users SET role='$2' WHERE email='$1';" >/dev/null; }

echo "→ Register users"
_=$(register "Cust ${TS}" "$CUSTOMER_EMAIL")
_=$(register "Other ${TS}" "$OTHER_EMAIL")
_=$(register "Agent ${TS}" "$AGENT_EMAIL")
_=$(register "Agent2 ${TS}" "$AGENT2_EMAIL")
_=$(register "Admin ${TS}" "$ADMIN_EMAIL")

echo "→ Promote roles"
set_role "$AGENT_EMAIL"  agent
set_role "$AGENT2_EMAIL" agent
set_role "$ADMIN_EMAIL"  admin

CUSTOMER_TOKEN=$(login "$CUSTOMER_EMAIL")
OTHER_TOKEN=$(login "$OTHER_EMAIL")
AGENT_TOKEN=$(login "$AGENT_EMAIL")
AGENT2_TOKEN=$(login "$AGENT2_EMAIL")
ADMIN_TOKEN=$(login "$ADMIN_EMAIL")

AGENT_ID=$(user_id_for "$AGENT_EMAIL")

echo "→ Customer creates a ticket"
RESP=$(req "${BASE_URL}/api/v1/ticket-categories" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
CATEGORY_ID=$(http_body "$RESP" | jq -er '.data[] | select(.slug=="technical-issue") | .id')
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets" -H "Authorization: Bearer ${CUSTOMER_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg cat "$CATEGORY_ID" '{title:"Comment-flow test", description:"Verifying comments end to end.", category_id:$cat, priority:"medium"}')")
expect_status "$(http_status "$RESP")" 201 "create ticket"
TICKET_ID=$(http_body "$RESP" | jq -er '.data.id')

echo "→ Admin assigns the agent"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/assign" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid $AGENT_ID '{agent_id:$uid}')")
expect_status "$(http_status "$RESP")" 200 "assign"

echo "→ Customer creates a comment"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"Issue still happens"}')
expect_status "$(http_status "$RESP")" 201 "customer add"
CUST_COMMENT_ID=$(http_body "$RESP" | jq -er '.data.id')
# Verify role populated
ROLE=$(http_body "$RESP" | jq -er '.data.author.role')
[[ "$ROLE" == "customer" ]] || { echo "FAIL: role=$ROLE"; exit 1; }

echo "→ Assigned agent adds a comment"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"Investigating now"}')
expect_status "$(http_status "$RESP")" 201 "agent add"

echo "→ Other customer cannot list comments (404)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" -H "Authorization: Bearer ${OTHER_TOKEN}")
expect_status "$(http_status "$RESP")" 404 "other customer list"

echo "→ Unassigned agent2 cannot list comments (404)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" -H "Authorization: Bearer ${AGENT2_TOKEN}")
expect_status "$(http_status "$RESP")" 404 "agent2 list"

echo "→ Blank body rejected (422)"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"   "}')
# Validator triggers on min=1; trimming is server-side. Either 422 (validator) or 422 (use case).
expect_status "$(http_status "$RESP")" 422 "blank rejected"

echo "→ Author edits own comment"
RESP=$(req -X PUT "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments/${CUST_COMMENT_ID}" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"Updated comment"}')
expect_status "$(http_status "$RESP")" 200 "author edit"

echo "→ Non-author (agent) cannot edit foreign comment (403)"
RESP=$(req -X PUT "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments/${CUST_COMMENT_ID}" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"no"}')
expect_status "$(http_status "$RESP")" 403 "agent edit foreign"

echo "→ Admin can edit any comment"
RESP=$(req -X PUT "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments/${CUST_COMMENT_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"[moderated]"}')
expect_status "$(http_status "$RESP")" 200 "admin edit"

echo "→ Comment ID under wrong ticket -> 404"
# Create a second ticket for the same customer
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets" -H "Authorization: Bearer ${CUSTOMER_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg cat "$CATEGORY_ID" '{title:"Second ticket", description:"For path mismatch test.", category_id:$cat, priority:"low"}')")
T2_ID=$(http_body "$RESP" | jq -er '.data.id')
RESP=$(req -X PUT "${BASE_URL}/api/v1/tickets/${T2_ID}/comments/${CUST_COMMENT_ID}" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"wrong ticket"}')
expect_status "$(http_status "$RESP")" 404 "path mismatch"

echo "→ Timeline contains history + comment items, ascending"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/timeline" -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "timeline"
BODY=$(http_body "$RESP")
echo "$BODY" | jq -e '.data | any(.type == "history")' >/dev/null
echo "$BODY" | jq -e '.data | any(.type == "comment")' >/dev/null
OK=$(echo "$BODY" | jq '[.data[].occurred_at] == ([.data[].occurred_at] | sort)')
[[ "$OK" == "true" ]] || { echo "FAIL: timeline not ascending"; exit 1; }

echo "→ Customer deletes own comment"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"body":"to be deleted"}')
DEL_ID=$(http_body "$RESP" | jq -er '.data.id')
RESP=$(req -X DELETE "${BASE_URL}/api/v1/tickets/${TICKET_ID}/comments/${DEL_ID}" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 204 "author delete"

echo "→ Other customer cannot read timeline (404)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/timeline" -H "Authorization: Bearer ${OTHER_TOKEN}")
expect_status "$(http_status "$RESP")" 404 "other timeline"

echo
echo "Phase 4 comments + timeline verification passed"
