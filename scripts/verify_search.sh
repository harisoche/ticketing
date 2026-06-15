#!/usr/bin/env bash
#
# End-to-end verification for Phase 6 search/pagination/dashboard.
# Requires: curl, jq, psql; the API running on $BASE_URL.

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DATABASE_URL="${DATABASE_URL:-postgres://ticketing_user:ticketing_password@localhost:5432/ticketing_db?sslmode=disable}"
TS="$(date +%s)$$"
PW='password123'

CUSTOMER_EMAIL="p6-cust+${TS}@example.com"
OTHER_EMAIL="p6-other+${TS}@example.com"
AGENT_EMAIL="p6-agent+${TS}@example.com"
ADMIN_EMAIL="p6-admin+${TS}@example.com"

trap 'echo "verify_search.sh failed on line $LINENO" >&2' ERR

req() { curl -sS -w '\n%{http_code}' "$@"; }
http_status() { echo "$1" | tail -n1; }
http_body()   { echo "$1" | sed '$d'; }

expect_status() {
  if [[ "$1" != "$2" ]]; then
    echo "FAIL: $3: expected HTTP $2, got HTTP $1"
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

echo "→ Register + promote roles"
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
CUSTOMER_ID=$(user_id_for "$CUSTOMER_EMAIL")

RESP=$(req "${BASE_URL}/api/v1/ticket-categories" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
CATEGORY_ID=$(http_body "$RESP" | jq -er '.data[] | select(.slug=="technical-issue") | .id')

echo "→ Create three tickets with searchable titles"
create_ticket() {
  local title="$1" desc="$2" prio="$3"
  RESP=$(req -X POST "${BASE_URL}/api/v1/tickets" \
    -H "Authorization: Bearer ${CUSTOMER_TOKEN}" -H 'Content-Type: application/json' \
    -d "$(jq -n --arg cat "$CATEGORY_ID" --arg t "$title" --arg d "$desc" --arg p "$prio" \
            '{title:$t, description:$d, category_id:$cat, priority:$p}')")
  expect_status "$(http_status "$RESP")" 201 "create $title"
  http_body "$RESP" | jq -er '.data.id'
}
T1=$(create_ticket "Cannot connect to office wifi network" "Wi-Fi drops every minute in meeting room A" "high")
T2=$(create_ticket "Email client crashes on launch" "After updating to v3 the client crashes immediately" "medium")
T3=$(create_ticket "VPN disconnects intermittently" "VPN session ends after 5 minutes - network issue" "low")

echo "→ Default page returns per_page=20"
RESP=$(req "${BASE_URL}/api/v1/tickets" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "default list"
PP=$(http_body "$RESP" | jq -r '.meta.per_page')
[[ "$PP" == "20" ]] || { echo "FAIL: default per_page=$PP"; exit 1; }

echo "→ per_page=500 is clamped to 100"
RESP=$(req "${BASE_URL}/api/v1/tickets?per_page=500" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "per_page clamp"
PP=$(http_body "$RESP" | jq -r '.meta.per_page')
[[ "$PP" == "100" ]] || { echo "FAIL: clamped per_page=$PP"; exit 1; }

echo "→ page=-1 returns 422"
RESP=$(req "${BASE_URL}/api/v1/tickets?page=-1" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 422 "page=-1"

echo "→ q matches title — wifi"
RESP=$(req --get "${BASE_URL}/api/v1/tickets" \
  --data-urlencode "q=wifi" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "q=wifi"
http_body "$RESP" | jq -e --arg id "$T1" '.data | any(.id == $id)' >/dev/null
COUNT=$(http_body "$RESP" | jq -r '.data | length')
[[ "$COUNT" -ge 1 ]] || { echo "FAIL: q=wifi count=$COUNT"; exit 1; }

echo "→ q matches description — meeting room"
RESP=$(req --get "${BASE_URL}/api/v1/tickets" \
  --data-urlencode "q=meeting room" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "q=meeting room"
http_body "$RESP" | jq -e --arg id "$T1" '.data | any(.id == $id)' >/dev/null

echo "→ sort_by=garbage returns 422"
RESP=$(req "${BASE_URL}/api/v1/tickets?sort_by=garbage" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 422 "bad sort"

echo "→ sort_by=status returns 200"
RESP=$(req "${BASE_URL}/api/v1/tickets?sort_by=status&sort_order=asc" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "sort=status"

echo "→ Customer cannot view=all (422)"
RESP=$(req "${BASE_URL}/api/v1/tickets?view=all" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 422 "customer view=all"

echo "→ Customer cannot widen via created_by/assigned_to"
RESP=$(req "${BASE_URL}/api/v1/tickets?created_by=${AGENT_ID}&assigned_to=${AGENT_ID}" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "customer with admin filters"
TOTAL=$(http_body "$RESP" | jq -r '.meta.total')
[[ "$TOTAL" == "3" ]] || { echo "FAIL: customer should still see own 3 tickets, got $TOTAL"; exit 1; }

echo "→ Admin uses created_by filter"
RESP=$(req "${BASE_URL}/api/v1/tickets?created_by=${CUSTOMER_ID}" -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "admin created_by"
TOTAL=$(http_body "$RESP" | jq -r '.meta.total')
[[ "$TOTAL" == "3" ]] || { echo "FAIL: admin created_by=cust expected 3, got $TOTAL"; exit 1; }

echo "→ Agent view=all is forbidden (403)"
RESP=$(req "${BASE_URL}/api/v1/tickets?view=all" -H "Authorization: Bearer ${AGENT_TOKEN}")
expect_status "$(http_status "$RESP")" 403 "agent view=all"

echo "→ Date range from > to is rejected (422)"
RESP=$(req "${BASE_URL}/api/v1/tickets?created_from=2026-06-30T00:00:00Z&created_to=2026-06-01T00:00:00Z" -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 422 "bad date range"

echo "→ Total independent of per_page"
A=$(req "${BASE_URL}/api/v1/tickets?per_page=1"  -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
B=$(req "${BASE_URL}/api/v1/tickets?per_page=100" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
TA=$(http_body "$A" | jq -r '.meta.total')
TB=$(http_body "$B" | jq -r '.meta.total')
[[ "$TA" == "$TB" ]] || { echo "FAIL: total differs $TA vs $TB"; exit 1; }

echo "→ Empty list returns valid meta"
RESP=$(req "${BASE_URL}/api/v1/tickets" -H "Authorization: Bearer ${OTHER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "empty list"
EMPTY=$(http_body "$RESP" | jq -r '.data | length')
META_TOTAL=$(http_body "$RESP" | jq -r '.meta.total')
[[ "$EMPTY" == "0" && "$META_TOTAL" == "0" ]] || { echo "FAIL: empty list shape"; exit 1; }

echo "→ Dashboard summary as customer"
RESP=$(req "${BASE_URL}/api/v1/dashboard/summary" -H "Authorization: Bearer ${CUSTOMER_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "dashboard cust"
BODY=$(http_body "$RESP")
echo "$BODY" | jq -e '.data.total_tickets == 3' >/dev/null
echo "$BODY" | jq -e '.data.by_status.open // 0 >= 3' >/dev/null
echo "$BODY" | jq -e '.data.by_status | has("reopened")' >/dev/null
echo "$BODY" | jq -e '.data.by_priority | has("urgent")' >/dev/null

echo "→ Dashboard summary as agent (no assignments yet → 0)"
RESP=$(req "${BASE_URL}/api/v1/dashboard/summary" -H "Authorization: Bearer ${AGENT_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "dashboard agent"
http_body "$RESP" | jq -e '.data.total_tickets == 0' >/dev/null

echo "→ Assign one ticket to agent, agent dashboard becomes 1"
RESP=$(req -X PATCH "${BASE_URL}/api/v1/tickets/${T1}/assign" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' \
  -d "$(jq -n --argjson uid $AGENT_ID '{agent_id:$uid}')")
expect_status "$(http_status "$RESP")" 200 "assign"
RESP=$(req "${BASE_URL}/api/v1/dashboard/summary" -H "Authorization: Bearer ${AGENT_TOKEN}")
http_body "$RESP" | jq -e '.data.total_tickets == 1' >/dev/null

echo "→ Dashboard summary as admin (>= 3)"
RESP=$(req "${BASE_URL}/api/v1/dashboard/summary" -H "Authorization: Bearer ${ADMIN_TOKEN}")
expect_status "$(http_status "$RESP")" 200 "dashboard admin"
TOTAL=$(http_body "$RESP" | jq -r '.data.total_tickets')
[[ "$TOTAL" -ge 3 ]] || { echo "FAIL: admin total < 3"; exit 1; }

# touch unused so set -u doesn't trip on the last variable
: "${T2:?}" "${T3:?}"

echo
echo "Phase 6 search + pagination + dashboard verification passed"
