<div align=center>

# MockPay

_Mock Payment Gateway Server_

</div>

A local mock server for **MTN MoMo** and **Airtel Africa Money** APIs built with Go + GoFiber.  
Transactions live entirely in memory. Random processing delays (300 ms to 3 s) and a 10 % failure
rate are simulated out of the box while both are tuneable at runtime via the admin API.

---

## Quick start

```bash
make tidy          # download deps (fetches directly from GitHub, no Go proxy needed)
make run           # start on :8080
make smoke         # run a quick end-to-end smoke test while the server is up
```

---

## Simulation behaviour

| Behaviour | Default | Change at runtime |
|-----------|---------|-------------------|
| Processing delay | 300 – 3000 ms random | `POST /admin/config` |
| Failure rate | 10 % | `POST /admin/config` |
| Force outcome per request | – | `?force=fail` or `?force=success` query param |

### Force a specific outcome
Append `?force=fail` or `?force=success` to any initiation endpoint:

```
POST /mtn/collection/v1_0/requesttopay?force=fail
POST /airtel/standard/v1/disbursements/?force=success
```

---

## MTN MoMo API  `/mtn/...`

### 1. Sandbox provisioning

**Create API user**
```
POST /mtn/v1_0/apiuser
Headers: X-Reference-Id: <uuid>
Body:    { "providerCallbackHost": "localhost" }
→ 201 (no body)
```

**Generate API key**
```
POST /mtn/v1_0/apiuser/:userId/apikey
→ 201 { "apiKey": "..." }
```

**Inspect API user**
```
GET /mtn/v1_0/apiuser/:userId
→ 200 { "providerCallbackHost": "...", "targetEnvironment": "sandbox" }
```

> A default user is pre-seeded: `mock-api-user` / `mock-api-key` — use these to skip provisioning.

---

### 2. Authentication

All token endpoints require **HTTP Basic auth**: `base64(userId:apiKey)`.

**Collection token**
```
POST /mtn/collection/token/
Authorization: Basic <base64(userId:apiKey)>
→ { "access_token": "...", "token_type": "access_token", "expires_in": 3600 }
```

**Disbursement token**
```
POST /mtn/disbursement/token/
Authorization: Basic <base64(userId:apiKey)>
→ { "access_token": "...", "token_type": "access_token", "expires_in": 3600 }
```

---

### 3. Collections

**Initiate payment request**
```
POST /mtn/collection/v1_0/requesttopay
Authorization: Bearer <token>
X-Reference-Id: <uuid>          (your idempotency key — generated if omitted)
X-Callback-Url: <url>           (optional; webhook fired after processing)
X-Target-Environment: sandbox

{
  "amount": "5000",
  "currency": "UGX",
  "externalId": "order-123",
  "payer": { "partyIdType": "MSISDN", "partyId": "256770000000" },
  "payerMessage": "Payment for order-123",
  "payeeNote": "Thank you"
}
→ 202 (empty body — poll or await callback)
```

**Query status**
```
GET /mtn/collection/v1_0/requesttopay/:referenceId
Authorization: Bearer <token>
→ {
    "amount": "5000", "currency": "UGX", "status": "PENDING|SUCCESSFUL|FAILED",
    "financialTransactionId": "FINxxxxxxxx",   (present when SUCCESSFUL)
    "reason": { "code": "...", "message": "..." }  (present when FAILED)
  }
```

**Account balance**
```
GET /mtn/collection/v1_0/account/balance
→ { "availableBalance": "...", "currency": "UGX" }
```

**Validate account holder**
```
GET /mtn/collection/v1_0/accountholder/:idType/:id/active
→ { "result": true|false }
```

---

### 4. Disbursements

**Initiate transfer**
```
POST /mtn/disbursement/v1_0/transfer
Authorization: Bearer <token>
X-Reference-Id: <uuid>
X-Callback-Url: <url>

{
  "amount": "10000",
  "currency": "UGX",
  "externalId": "payout-456",
  "payee": { "partyIdType": "MSISDN", "partyId": "256780000000" },
  "payerMessage": "Salary payment",
  "payeeNote": "August salary"
}
→ 202
```

**Query status**
```
GET /mtn/disbursement/v1_0/transfer/:referenceId
→ same shape as collection status query (payee instead of payer)
```

**Account balance**
```
GET /mtn/disbursement/v1_0/account/balance
→ { "availableBalance": "...", "currency": "UGX" }
```

---

### 5. MTN Webhook

Fires to `X-Callback-Url` with the following payload:

```json
{
  "referenceId": "...",
  "financialTransactionId": "FINxxxxxxxx",
  "externalId": "order-123",
  "amount": "5000",
  "currency": "UGX",
  "payer": { "partyIdType": "MSISDN", "partyId": "256770000000" },
  "payerMessage": "...",
  "payeeNote": "...",
  "status": "SUCCESSFUL",
  "reason": null
}
```

---

## Airtel Africa Money API  `/airtel/...`

### 1. Authentication

```
POST /airtel/auth/oauth2/token
Content-Type: application/json

{ "client_id": "any", "client_secret": "any", "grant_type": "client_credentials" }
→ { "access_token": "...", "expires_in": 3600, "token_type": "Bearer" }
```

All other endpoints require `Authorization: Bearer <token>`.

---

### 2. Collections

**Initiate payment**
```
POST /airtel/merchant/v2/payments/
Authorization: Bearer <token>
X-Country: UG
X-Currency: UGX
X-Callback-Url: <url>           (optional)

{
  "reference": "order-789",
  "subscriber": { "country": "UG", "currency": "UGX", "msisdn": "256750000000" },
  "transaction": { "amount": 7500, "country": "UG", "currency": "UGX", "id": "txn-unique-001" }
}
→ {
    "data": { "transaction": { "id": "...", "status": "DP", "message": "Waiting for customer confirmation" } },
    "status": { "code": "200", "message": "SUCCESS", "result_code": "ESB000010", "success": true }
  }
```

**Query payment status**
```
GET /airtel/standard/v1/payments/:id
→ {
    "data": { "transaction": { "id": "...", "airtel_money_id": "CIxxxxxxxxxx", "status": "TS|TF|DP", "message": "..." } },
    "status": { ... }
  }
```

Status codes: `DP` = pending, `TS` = successful, `TF` = failed.

---

### 3. Disbursements

**Initiate disbursement**
```
POST /airtel/standard/v1/disbursements/
Authorization: Bearer <token>
X-Callback-Url: <url>

{
  "payee": { "msisdn": "256760000000" },
  "reference": "payout-ref-001",
  "pin": "encrypted-pin",
  "transaction": { "amount": 25000, "currency": "UGX", "id": "disb-unique-001" }
}
→ { "data": { "transaction": { "id": "...", "status": "DP", "message": "Disbursement in progress" } }, ... }
```

**Query disbursement status**
```
GET /airtel/standard/v1/disbursements/:id
→ same shape as payment status
```

---

### 4. Refunds

```
POST /airtel/standard/v1/payments/refund
Authorization: Bearer <token>

{ "transaction": { "airtel_money_id": "CIxxxxxxxxxx" } }
→ { "data": { "transaction": { "airtel_money_id": "...", "status": "TS", "message": "Refund Successful" } }, ... }
```

---

### 5. Airtel Webhook 

Fires to `X-Callback-Url` with the following payload:

```json
{
  "transaction": {
    "id": "txn-unique-001",
    "airtel_money_id": "CIxxxxxxxxxx",
    "msisdn": "256750000000",
    "amount": "7500",
    "currency": "UGX",
    "status": "TS",
    "message": "Transaction Successful"
  }
}
```

---

## Admin API  `/admin/...`

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/state` | Dump all transactions + current sim config |
| `POST` | `/admin/config` | Update sim config |
| `DELETE` | `/admin/reset` | Wipe transactions and tokens |
| `GET` | `/admin/ready` | Healthcheck |

**Update sim config**
```json
POST /admin/config
{ "failureRate": 0.5, "minDelayMs": 100, "maxDelayMs": 1000 }
```

---

## Example: MTN curl cheatsheet

```bash
BASE=http://localhost:8080

# 1. Get token using the pre-seeded default user
TOKEN=$(curl -s -X POST $BASE/mtn/collection/token/ \
  -H "Authorization: Basic $(echo -n 'mock-api-user:mock-api-key' | base64)" \
  | jq -r .access_token)

# 2. Initiate a collection
REF=$(uuidgen)
curl -s -X POST $BASE/mtn/collection/v1_0/requesttopay \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Reference-Id: $REF" \
  -H "X-Target-Environment: sandbox" \
  -H "Content-Type: application/json" \
  -d "{\"amount\":\"5000\",\"currency\":\"UGX\",\"externalId\":\"ord-1\",
       \"payer\":{\"partyIdType\":\"MSISDN\",\"partyId\":\"256770000001\"},
       \"payerMessage\":\"pay\",\"payeeNote\":\"thanks\"}"

# 3. Poll for status (retry after 2–3 s)
sleep 3
curl -s $BASE/mtn/collection/v1_0/requesttopay/$REF \
  -H "Authorization: Bearer $TOKEN" | jq .
```

## License

Released under [MIT License](./LICENSE.txt) - check file for details

&copy; 2026-present MomobaseHQ
