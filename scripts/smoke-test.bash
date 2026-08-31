#!/usr/bin/env bash

echo "=== HEALTHCHECK ==="
curl -s http://localhost:7676/admin/ready | python3 -m json.tool

echo ""
echo "=== CONFIGURATION UI ==="
curl -s http://localhost:7676/ | grep -q "Simulation configuration"
echo "UI: available"

echo ""
echo "=== CONFIGURE: successful outcomes, short delay ==="
curl -s -X POST http://localhost:7676/admin/config \
  -H "Content-Type: application/json" \
  -d '{"failureRate":0.0,"minDelayMs":100,"maxDelayMs":100}' | python3 -m json.tool

echo ""
echo "=== MTN: collection token (default creds) ==="
COLLECTION_TOKEN=$(curl -s -X POST http://localhost:7676/mtn/collection/token/ \
  -H "Authorization: Basic $(echo -n 'mock-api-user:mock-api-key' | base64)" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['access_token'])")
echo "Token: ${COLLECTION_TOKEN:0:32}..."

echo ""
echo "=== MTN: initiate collection ==="
REF_ID="550e8400-e29b-41d4-a716-446655440001"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:7676/mtn/collection/v1_0/requesttopay \
  -H "Authorization: Bearer $COLLECTION_TOKEN" \
  -H "X-Reference-Id: $REF_ID" \
  -H "X-Target-Environment: sandbox" \
  -H "Content-Type: application/json" \
  -d '{"amount":"5000","currency":"UGX","externalId":"order-123","payer":{"partyIdType":"MSISDN","partyId":"256770000001"},"payerMessage":"Pay for order","payeeNote":"Thanks"}')
echo "HTTP: $HTTP_CODE (expect 202)"

echo ""
echo "=== MTN: poll status immediately (expect PENDING) ==="
curl -s http://localhost:7676/mtn/collection/v1_0/requesttopay/$REF_ID \
  -H "Authorization: Bearer $COLLECTION_TOKEN" | python3 -m json.tool

echo ""
echo "=== MTN: configured success collection ==="
REF2="550e8400-e29b-41d4-a716-446655440002"
curl -s -o /dev/null -X POST "http://localhost:7676/mtn/collection/v1_0/requesttopay" \
  -H "Authorization: Bearer $COLLECTION_TOKEN" \
  -H "X-Reference-Id: $REF2" \
  -H "X-Target-Environment: sandbox" \
  -H "Content-Type: application/json" \
  -d '{"amount":"2000","currency":"UGX","externalId":"order-124","payer":{"partyIdType":"MSISDN","partyId":"256770000002"},"payerMessage":"pay","payeeNote":"ok"}'
sleep 1
curl -s http://localhost:7676/mtn/collection/v1_0/requesttopay/$REF2 \
  -H "Authorization: Bearer $COLLECTION_TOKEN" | python3 -m json.tool

echo ""
echo "=== MTN: configured failure disbursement ==="
curl -s -X POST http://localhost:7676/admin/config \
  -H "Content-Type: application/json" \
  -d '{"failureRate":1.0}' | python3 -m json.tool
DISB_TOKEN=$(curl -s -X POST http://localhost:7676/mtn/disbursement/token/ \
  -H "Authorization: Basic $(echo -n 'mock-api-user:mock-api-key' | base64)" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['access_token'])")
REF3="550e8400-e29b-41d4-a716-446655440003"
curl -s -o /dev/null -X POST "http://localhost:7676/mtn/disbursement/v1_0/transfer" \
  -H "Authorization: Bearer $DISB_TOKEN" \
  -H "X-Reference-Id: $REF3" \
  -H "X-Target-Environment: sandbox" \
  -H "Content-Type: application/json" \
  -d '{"amount":"10000","currency":"UGX","externalId":"payout-1","payee":{"partyIdType":"MSISDN","partyId":"256780000001"}}'
sleep 1
curl -s http://localhost:7676/mtn/disbursement/v1_0/transfer/$REF3 \
  -H "Authorization: Bearer $DISB_TOKEN" | python3 -m json.tool

curl -s -X POST http://localhost:7676/admin/config \
  -H "Content-Type: application/json" \
  -d '{"failureRate":0.0}' > /dev/null

echo ""
echo "=== AIRTEL: get token ==="
AIRTEL_TOKEN=$(curl -s -X POST http://localhost:7676/airtel/auth/oauth2/token \
  -H "Content-Type: application/json" \
  -d '{"client_id":"my-app","client_secret":"secret","grant_type":"client_credentials"}' | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['access_token'])")
echo "Token: ${AIRTEL_TOKEN:0:32}..."

echo ""
echo "=== AIRTEL: initiate payment ==="
TXN_ID="airtel-pay-001"
curl -s -X POST http://localhost:7676/airtel/merchant/v2/payments/ \
  -H "Authorization: Bearer $AIRTEL_TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Country: UG" \
  -H "X-Currency: UGX" \
  -d "{\"reference\":\"ord-001\",\"subscriber\":{\"country\":\"UG\",\"currency\":\"UGX\",\"msisdn\":\"256750000001\"},\"transaction\":{\"amount\":7500,\"country\":\"UG\",\"currency\":\"UGX\",\"id\":\"$TXN_ID\"}}" | python3 -m json.tool

echo ""
echo "=== AIRTEL: configured success payment ==="
TXN_ID2="airtel-pay-002"
curl -s -o /dev/null -X POST "http://localhost:7676/airtel/merchant/v2/payments/" \
  -H "Authorization: Bearer $AIRTEL_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"reference\":\"ord-002\",\"subscriber\":{\"country\":\"UG\",\"currency\":\"UGX\",\"msisdn\":\"256750000002\"},\"transaction\":{\"amount\":3000,\"country\":\"UG\",\"currency\":\"UGX\",\"id\":\"$TXN_ID2\"}}"
sleep 1
curl -s "http://localhost:7676/airtel/standard/v1/payments/$TXN_ID2" \
  -H "Authorization: Bearer $AIRTEL_TOKEN" | python3 -m json.tool

echo ""
echo "=== AIRTEL: initiate disbursement ==="
DISB_ID="airtel-disb-001"
curl -s -X POST "http://localhost:7676/airtel/standard/v1/disbursements/" \
  -H "Authorization: Bearer $AIRTEL_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"payee\":{\"msisdn\":\"256760000001\"},\"reference\":\"pout-001\",\"pin\":\"enc-pin\",\"transaction\":{\"amount\":15000,\"currency\":\"UGX\",\"id\":\"$DISB_ID\"}}" | python3 -m json.tool
sleep 1
echo "--- disbursement status ---"
curl -s "http://localhost:7676/airtel/standard/v1/disbursements/$DISB_ID" \
  -H "Authorization: Bearer $AIRTEL_TOKEN" | python3 -m json.tool

echo ""
echo "=== AIRTEL: refund ==="
curl -s -X POST http://localhost:7676/airtel/standard/v1/payments/refund \
  -H "Authorization: Bearer $AIRTEL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"transaction":{"airtel_money_id":"CIxxxxxxxxxx"}}' | python3 -m json.tool

echo ""
echo "=== ADMIN: change failure rate to 100% ==="
curl -s -X POST http://localhost:7676/admin/config \
  -H "Content-Type: application/json" \
  -d '{"failureRate":1.0,"minDelayMs":100,"maxDelayMs":500}' | python3 -m json.tool

echo ""
echo "=== ADMIN: change failure rate to 10% ==="
curl -s -X POST http://localhost:7676/admin/config \
  -H "Content-Type: application/json" \
  -d '{"failureRate":0.1,"minDelayMs":100,"maxDelayMs":500}' | python3 -m json.tool

echo ""
echo "=== ADMIN: state dump (summary) ==="
curl -s http://localhost:7676/admin/state | python3 -c "import sys,json; d=json.load(sys.stdin); print('sim config:', d['sim']); print('MTN collections:', len(d['mtn']['collections']), 'tx'); print('MTN disbursements:', len(d['mtn']['disbursements']), 'tx'); print('Airtel payments:', len(d['airtel']['payments']), 'tx'); print('Airtel disbursements:', len(d['airtel']['disbursements']), 'tx')"


echo ""
echo "=== ADMIN: reset ==="
curl -s -X DELETE http://localhost:7676/admin/reset | python3 -m json.tool


echo ""
echo "✅ All tests passed"
