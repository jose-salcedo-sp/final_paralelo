#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"

echo "1) Login"
TOKEN="$(curl -s -X POST -u user:password "$API_URL/login" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
if [[ -z "$TOKEN" ]]; then
  echo "Failed to obtain token"
  exit 1
fi
echo "TOKEN acquired"

echo "2) Create workload"
WORKLOAD_JSON="$(curl -s -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" -X POST -d '{"filter":"grayscale","workload_name":"test-workload"}' "$API_URL/workloads")"
WORKLOAD_ID="$(echo "$WORKLOAD_JSON" | sed -n 's/.*"workload_id":"\([^"]*\)".*/\1/p')"
if [[ -z "$WORKLOAD_ID" ]]; then
  echo "Failed to create workload"
  echo "$WORKLOAD_JSON"
  exit 1
fi

echo "3) Upload sample image"
if [[ ! -f tests/sample.png ]]; then
  echo "tests/sample.png not found (add any PNG image to run this script)"
  exit 1
fi
UPLOAD_JSON="$(curl -s -H "Authorization: Bearer $TOKEN" -F "data=@tests/sample.png" -F "workload_id=$WORKLOAD_ID" -F "type=original" -X POST "$API_URL/images")"
echo "$UPLOAD_JSON"

echo "4) Poll workload status"
for _ in {1..15}; do
  STATUS_JSON="$(curl -s -H "Authorization: Bearer $TOKEN" "$API_URL/workloads/$WORKLOAD_ID")"
  echo "$STATUS_JSON"
  if echo "$STATUS_JSON" | grep -q '"status":"completed"'; then
    break
  fi
  sleep 1
done

echo "5) System status"
curl -s -H "Authorization: Bearer $TOKEN" "$API_URL/status"
echo

echo "6) Logout"
curl -s -X DELETE -H "Authorization: Bearer $TOKEN" "$API_URL/logout"
echo

echo "Console test flow finished"
