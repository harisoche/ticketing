#!/usr/bin/env bash
#
# End-to-end verification for Phase 01 authentication.
# Requires: curl, jq, and the API running on $BASE_URL (default localhost:8080).

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TIMESTAMP="$(date +%s)"
EMAIL="verify+${TIMESTAMP}@example.com"
PASSWORD="password123"
NAME="Verify User ${TIMESTAMP}"
UPDATED_NAME="Verify Updated ${TIMESTAMP}"

trap 'echo "verify_auth.sh failed on line $LINENO" >&2' ERR

req() {
  local method="$1"
  local path="$2"
  shift 2
  curl -sS -w '\n%{http_code}' -X "$method" "${BASE_URL}${path}" "$@"
}

extract_token() {
  jq -er '.data.access_token'
}

echo "→ Register"
RESPONSE=$(req POST /api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg name "$NAME" --arg email "$EMAIL" --arg pw "$PASSWORD" \
    '{name:$name, email:$email, password:$pw}')")
STATUS=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')
if [[ "$STATUS" != "201" ]]; then
  echo "Register failed: HTTP $STATUS"
  echo "$BODY"
  exit 1
fi
TOKEN=$(echo "$BODY" | extract_token)

echo "→ GET /api/v1/me"
RESPONSE=$(req GET /api/v1/me -H "Authorization: Bearer ${TOKEN}")
STATUS=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')
if [[ "$STATUS" != "200" ]]; then
  echo "GET /me failed: HTTP $STATUS"
  echo "$BODY"
  exit 1
fi

echo "→ PATCH /api/v1/me"
RESPONSE=$(req PATCH /api/v1/me \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg name "$UPDATED_NAME" '{name:$name}')")
STATUS=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')
if [[ "$STATUS" != "200" ]]; then
  echo "PATCH /me failed: HTTP $STATUS"
  echo "$BODY"
  exit 1
fi
NEW_NAME=$(echo "$BODY" | jq -er '.data.name')
if [[ "$NEW_NAME" != "$UPDATED_NAME" ]]; then
  echo "Name not updated: $NEW_NAME"
  exit 1
fi

echo "→ Logout"
RESPONSE=$(req POST /api/v1/auth/logout -H "Authorization: Bearer ${TOKEN}")
STATUS=$(echo "$RESPONSE" | tail -n1)
if [[ "$STATUS" != "200" ]]; then
  echo "Logout failed: HTTP $STATUS"
  exit 1
fi

echo "→ Confirm revoked token returns 401"
RESPONSE=$(req GET /api/v1/me -H "Authorization: Bearer ${TOKEN}")
STATUS=$(echo "$RESPONSE" | tail -n1)
if [[ "$STATUS" != "401" ]]; then
  echo "Expected 401 after logout, got HTTP $STATUS"
  exit 1
fi

echo "→ Login again"
RESPONSE=$(req POST /api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg email "$EMAIL" --arg pw "$PASSWORD" '{email:$email, password:$pw}')")
STATUS=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')
if [[ "$STATUS" != "200" ]]; then
  echo "Login failed: HTTP $STATUS"
  echo "$BODY"
  exit 1
fi
NEW_TOKEN=$(echo "$BODY" | extract_token)

echo "→ Verify new token works"
RESPONSE=$(req GET /api/v1/me -H "Authorization: Bearer ${NEW_TOKEN}")
STATUS=$(echo "$RESPONSE" | tail -n1)
if [[ "$STATUS" != "200" ]]; then
  echo "GET /me with new token failed: HTTP $STATUS"
  exit 1
fi

echo
echo "Authentication verification passed"
