#!/usr/bin/env bash
#
# End-to-end verification covering Phase 2 ticket management AND the Phase 3
# assignment + status-workflow + histories rules.
#
# Requires: curl, jq, psql; the API running on $BASE_URL.

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DATABASE_URL="${DATABASE_URL:-postgres://ticketing_user:ticketing_password@localhost:5432/ticketing_db?sslmode=disable}"
TS="$(date +%s)$$"
PW='password123'

CUSTOMER_EMAIL="p3-cust+${TS}@example.com"
OTHER_EMAIL="p3-other+${TS}@example.com"
AGENT_EMAIL="p3-agent+${TS}@example.com"
AGENT2_EMAIL="p3-agent2+${TS}@example.com"
ADMIN_EMAIL="p3-admin+${TS}@example.com"

trap 'echo "verify_tickets.sh failed on line $LINENO" >&2' ERR

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
  local name="$1" email="$2" r
  r=$(req -X POST "${BASE_URL}/api/v1/auth/register" -H 'Content-Type: application/json' \
       -d "$(jq -n --arg name "$name" --arg email "$email" --arg pw "$PW" \
              '{name:$name,email:$email,password:$pw}')")
  expect_status "$(http_status "$r")" 201 "register $email"
  http_body "$r" | jq -er '.data.access_token'
}

login() {
  local email="$1" r
  r=$(req -X POST "${BASE_URL}/api/v1/auth/login" -H 'Content-Type: application/json' \
       -d "$(jq -n --arg email "$email" --arg pw "$PW" '{email:$email,password:$pw}')")
  expect_status "$(http_status "$r")" 200 "login $email"
  http_body "$r" | jq -er '.data.access_token'
}

user_id_for() {
  psql "$DATABASE_URL" -tAc "SELECT id FROM users WHERE email = '$1'"
}

set_role() {
  psql "$DATABASE_URL" -c "UPDATE users SET role='$2' WHERE email='$1';" >/dev/null
}

echo "→ Register customer / other / agents / admin"
CUSTOMER_TOKEN_REG=$(register "Customer ${TS}" "$CUSTOMER_EMAIL")
OTHER_TOKEN_REG=$(register "Other ${TS}" "$OTHER_EMAIL")
AGENT_TOKEN_REG=$(register "Agent ${TS}" "$AGENT_EMAIL")
AGENT2_TOKEN_REG=$(register "Agent2 ${TS}" "$AGENT2_EMAIL")
ADMIN_TOKEN_REG=$(register "Admin ${TS}" "$ADMIN_EMAIL")

echo "→ Promote roles via SQL"
set_role "$AGENT_EMAIL"  agent
set_role "$AGENT2_EMAIL" agent
set_role "$ADMIN_EMAIL"  admin

CUSTOMER_TOKEN=$(login "$CUSTOMER_EMAIL")
OTHER_TOKEN=$(login "$OTHER_EMAIL")
AGENT_TOKEN=$(login "$AGENT_EMAIL")
AGENT2_TOKEN=$(login "$AGENT2_EMAIL")
ADMIN_TOKEN=$(login "$ADMIN_EMAIL")

AGENT_ID=$(user_id_for "$AGENT_EMAIL")
AGENT2_ID=$(user_id_for "$AGENT2_EMAIL")
CUSTOMER_ID=$(user_id_for "$CUSTOMER_EMAIL")

# Touch the registration tokens once so set -u doesn't trip later.
: "${CUSTOMER_TOKEN_REG:?}" "${OTHER_TOKEN_REG:?}" "${AGENT_TOKEN_REG:?}" "${AGENT2_TOKEN_REG:?}" "${ADMIN_TOKEN_REG:?}"

echo "→ Customer fetches categories"
RESP=$(req "${BASE_URL}/api/v1/ticket-categories" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "list categories"
CATEGORY_ID=$(http_body "$RESP" | jq -er '.data[] | select(.slug=="technical-issue") | .id')

echo "→ Customer creates ticket"
RESP=$(req -X POST "${BASE_URL}/api/v1/tickets" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg cat "$CATEGORY_ID" '{title:"Cannot log in", description:"App rejects valid credentials.", category_id:$cat, priority:"high"}')")
expect_status "$(http_status "$RESP")" 201 "create ticket"
TICKET_ID=$(http_body "$RESP" | jq -er '.data.id')

echo "→ Histories include 'created'"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/histories" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "histories created"
http_body "$RESP" | jq -e '.data | any(.action == "created")' >/dev/null

echo "→ Unassigned open -> in_progress is rejected (422)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"in_progress"}')
expect_status "$(http_status "$RESP")" 422 "unassigned cannot in_progress"

echo "→ Customer cannot view another customer's ticket (404)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}" -H "Authorization: Bearer ${OTHER_TOKEN}")
expect_status "$(http_status "$RESP")" 404 "other customer 404"

echo "→ Admin can list scope=created_by_me — sees none"
RESP=$(req "${BASE_URL}/api/v1/tickets?scope=created_by_me" -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "admin scope=created_by_me"
http_body "$RESP" | jq -e --arg tid "$TICKET_ID" '.data | all(.id != $tid)' >/dev/null

echo "→ Admin assigns the agent (Phase 3 endpoint /assign)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/assign" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid $AGENT_ID '{agent_id:$uid, note:"Assigned for investigation"}')")
expect_status "$(http_status "$RESP")" 200 "admin assigns"
http_body "$RESP" | jq -e '.data.assignee.id' >/dev/null

echo "→ Admin tries to 'assign' a customer (422)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/assign" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid $CUSTOMER_ID '{agent_id:$uid}')")
expect_status "$(http_status "$RESP")" 422 "non-agent rejected"

echo "→ Customer cannot assign (403)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/assign" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid $AGENT_ID '{agent_id:$uid}')")
expect_status "$(http_status "$RESP")" 403 "customer cannot assign"

echo "→ Another agent (not assigned) cannot update status (404 — hidden)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${AGENT2_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"in_progress"}')
expect_status "$(http_status "$RESP")" 404 "other agent hidden"

echo "→ Assigned agent moves open -> in_progress (200)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"in_progress", "note":"Investigation started"}')
expect_status "$(http_status "$RESP")" 200 "open->in_progress"

echo "→ Illegal open transition rejected (in_progress -> open, 422)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"open"}')
expect_status "$(http_status "$RESP")" 422 "illegal in_progress->open"

echo "→ Agent resolves (200)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${AGENT_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"resolved"}')
expect_status "$(http_status "$RESP")" 200 "in_progress->resolved"

echo "→ Customer cannot mark as closed directly (resolved->closed by them = 403)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"closed"}')
expect_status "$(http_status "$RESP")" 403 "customer cannot close"

echo "→ Admin closes the resolved ticket"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"closed"}')
expect_status "$(http_status "$RESP")" 200 "admin close"

echo "→ Customer reopens own closed ticket (200)"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/status" \
  -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
  -d '{"status":"reopened","note":"Issue is back"}')
expect_status "$(http_status "$RESP")" 200 "customer reopens"

echo "→ Admin reassigns to second agent"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${TICKET_ID}/assign" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid $AGENT2_ID '{agent_id:$uid}')")
expect_status "$(http_status "$RESP")" 200 "admin reassign"

echo "→ Histories contain assigned + reassigned + status_changed (chronological)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/histories" -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "histories list"
BODY=$(http_body "$RESP")
echo "$BODY" | jq -e '.data | any(.action == "assigned")' >/dev/null
echo "$BODY" | jq -e '.data | any(.action == "reassigned")' >/dev/null
echo "$BODY" | jq -e '.data | any(.action == "status_changed")' >/dev/null
SORTED=$(echo "$BODY" | jq '[.data[].created_at]')
SORTED_OK=$(echo "$BODY" | jq '[.data[].created_at] == ([.data[].created_at] | sort)')
if [[ "$SORTED_OK" != "true" ]]; then
  echo "FAIL: histories not chronological: $SORTED"
  exit 1
fi

echo "→ Agent2 (now assigned) can read histories"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/histories" -H "Authorization: Bearer ${AGENT2_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "agent2 histories"

echo "→ Other customer cannot read histories (404)"
RESP=$(req "${BASE_URL}/api/v1/tickets/${TICKET_ID}/histories" -H "Authorization: Bearer ${OTHER_TOKEN}")
expect_status "$(http_status "$RESP")" 404 "other customer histories 404"

echo "→ Agent2 list scope=assigned_to_me sees the ticket"
RESP=$(req "${BASE_URL}/api/v1/tickets?scope=assigned_to_me" -H "Authorization: Bearer ${AGENT2_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "agent2 assigned_to_me"
http_body "$RESP" | jq -e --arg tid "$TICKET_ID" '.data | any(.id == $tid)' >/dev/null

echo "→ Customer list scope=assigned_to_me forbidden (403)"
RESP=$(req "${BASE_URL}/api/v1/tickets?scope=assigned_to_me" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 403 "customer assigned_to_me"

echo "→ Agent1 (no longer assigned) defaults to creator-or-assignee — sees nothing"
RESP=$(req "${BASE_URL}/api/v1/tickets" -H "Authorization: Bearer ${AGENT_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "agent1 default list"
http_body "$RESP" | jq -e --arg tid "$TICKET_ID" '.data | all(.id != $tid)' >/dev/null

echo "→ Status filter narrows agent2 list to reopened"
RESP=$(req "${BASE_URL}/api/v1/tickets?scope=assigned_to_me&status=reopened" -H "Authorization: Bearer ${AGENT2_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "status=reopened"
http_body "$RESP" | jq -e --arg tid "$TICKET_ID" '.data | any(.id == $tid)' >/dev/null

echo
echo "Phase 2 + Phase 3 verification passed"
