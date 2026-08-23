<div align=center>

# MockPay

**Mock Payment Gateway Server**

[![Go](https://img.shields.io/badge/Go-1.26.2-blue.svg)](https://golang.org)
[![Fiber](https://img.shields.io/badge/Framework-Fiber%20v3-00aed6.svg)](https://gofiber.io)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](./LICENSE.txt)

A local mock server for **MTN MoMo** and **Airtel Africa Money** APIs built with Go + Fiber.

Perfect for testing payment integrations locally without hitting production APIs.

</div>


## Overview

MockPay replicates MTN MoMo and Airtel Africa Money payment gateway behavior with realistic characteristics:

- [x] **Async processing** – Configurable delays (300 ms – 3 s by default)
- [x] **Failure injection** – 10% default failure rate (tuneable)
- [x] **Webhook callbacks** – Delivers transaction completion events
- [x] **Configuration UI** – Tune simulation behavior at `http://localhost:8080/`
- [x] **Live activity** – Inspect in-memory transactions and pending webhooks
- [x] **Admin API** – Runtime configuration without restarts
- [x] **In-memory** – All data cleared on restart (perfect for testing)
- [x] **Pre-seeded credentials** – Ready to test immediately

>[!WARNING]
>
>**For local development only.** Do not use in production.


## Prerequisites

- **Go 1.26+**
- **make** (optional)
- **curl** and **jq** (optional, for testing)


## Installation

```bash
git clone https://github.com/momobasehq/mockpay.git
cd mockpay

make tidy    # Download dependencies
make build   # Build binary
make run     # Start on http://localhost:8080
```

## Quick Start

### Get a Token (MTN)

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/mtn/collection/token/ \
  -H "Authorization: Basic $(echo -n 'mock-api-user:mock-api-key' | base64)" \
  | jq -r .access_token)
```

### Initiate Payment

```bash
REF=$(uuidgen)
curl -X POST http://localhost:8080/mtn/collection/v1_0/requesttopay \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Reference-Id: $REF" \
  -H "X-Target-Environment: sandbox" \
  -H "Content-Type: application/json" \
  -d '{
    "amount": "5000",
    "currency": "UGX",
    "externalId": "order-123",
    "payer": {"partyIdType": "MSISDN", "partyId": "256770000001"},
    "payerMessage": "Payment for order",
    "payeeNote": "Thank you"
  }'
```

### Check Status

```bash
sleep 2  # Wait for async processing
curl -s http://localhost:8080/mtn/collection/v1_0/requesttopay/$REF \
  -H "Authorization: Bearer $TOKEN" | jq '.status, .financialTransactionId'
```


## Pre-Seeded Credentials

**MTN MoMo:**
```
UserID:                 mock-api-user
APIKey:                 mock-api-key
OcpApimSubscriptionKey: mock-oapi-subscription-key
```

**Airtel Africa Money:**
```
Any client_id and client_secret are accepted in sandbox mode
```


## Supported Endpoints

### 📌 MTN MoMo

Full endpoint reference: [**MTN MoMo API Documentation**](https://mtn-momo-api-documentation.readthedocs.io/en/latest/)

**Implemented endpoints:** `/mtn/*`
- `POST /collection/token/` – Get collection token (Bearer auth)
- `POST /collection/v1_0/requesttopay` – Initiate payment
- `GET /collection/v1_0/requesttopay/:referenceId` – Query payment status
- `GET /collection/v1_0/account/balance` – Account balance
- `POST /disbursement/token/` – Get disbursement token
- `POST /disbursement/v1_0/transfer` – Initiate transfer
- `GET /disbursement/v1_0/transfer/:referenceId` – Query transfer status
- `GET /disbursement/v1_0/account/balance` – Disbursement balance

### 📌 Airtel Africa Money

Full endpoint reference: [**Airtel Africa Money API**](https://developers.airtel.africa/)

**Implemented endpoints:** `/airtel/*`
- `POST /auth/oauth2/token` – OAuth2 client credentials flow
- `POST /merchant/v2/payments/` – Initiate payment
- `GET /standard/v1/payments/:id` – Query payment status
- `POST /standard/v1/disbursements/` – Initiate disbursement
- `GET /standard/v1/disbursements/:id` – Query disbursement status
- `POST /standard/v1/payments/refund` – Refund payment

## Simulation Behavior

Open [http://localhost:8080/](http://localhost:8080/) to configure the simulation. No login is required.

### Configuration

| Parameter | Default | Range |
|-----------|---------|-------|
| Failure Rate | 10% | 0–100% |
| Min Delay | 300 ms | 0–3000 ms |
| Max Delay | 3000 ms | 0–3000 ms |

All new transactions use the configured failure rate and a random delay within the configured range. Per-request outcome overrides are intentionally not supported, matching real provider environments more closely.

### Update through the API

```bash
curl -X POST http://localhost:8080/admin/config \
  -H "Content-Type: application/json" \
  -d '{
    "failureRate": 0.5,
    "minDelayMs": 100,
    "maxDelayMs": 500
  }'
```

## Admin API

### Get Full State

```bash
curl http://localhost:8080/admin/state | jq .
```

Response includes simulation config and all transactions (MTN + Airtel).

### Update Simulation Config

```bash
curl -X POST http://localhost:8080/admin/config \
  -H "Content-Type: application/json" \
  -d '{"failureRate": 0.25, "minDelayMs": 200, "maxDelayMs": 800}'
```

### Clear All Transactions

```bash
curl -X DELETE http://localhost:8080/admin/reset
```

Note: API users are preserved; only transactions and tokens are cleared.

### Health Check

```bash
curl http://localhost:8080/admin/ready
```


## Examples

### Airtel Payment with Webhook

```bash
BASE=http://localhost:8080
APP_WEBHOOK=http://localhost:3000/webhook

# 1. Get token
TOKEN=$(curl -s -X POST $BASE/airtel/auth/oauth2/token \
  -H "Content-Type: application/json" \
  -d '{"client_id":"myapp","client_secret":"secret","grant_type":"client_credentials"}' \
  | jq -r .access_token)

# 2. Initiate payment with callback
curl -X POST $BASE/airtel/merchant/v2/payments/ \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Callback-Url: $APP_WEBHOOK" \
  -H "Content-Type: application/json" \
  -d '{
    "reference": "order-789",
    "subscriber": {"country": "UG", "currency": "UGX", "msisdn": "256750000001"},
    "transaction": {"amount": 7500, "country": "UG", "currency": "UGX", "id": "txn-001"}
  }'
```

### Change Failure Rate to 100%

```bash
curl -X POST http://localhost:8080/admin/config \
  -H "Content-Type: application/json" \
  -d '{"failureRate": 1.0, "minDelayMs": 100, "maxDelayMs": 300}'
```

## Testing

```bash
# Terminal 1: Start server
make run

# Terminal 2: Run tests
make smoke
```

## Limitations

- **In-memory only** – All data lost on restart (by design)
- **No persistence** – Transactions not stored to disk
- **Webhook delivery** – Best-effort, no retry logic
- **Token expiry** – In-memory, not persistent across restarts
- **Single instance** – No clustering support


## Contributing

Contributions welcome! Please submit pull requests or open issues on GitHub.

## License

Released under [MIT License](./LICENSE.txt).

© 2026-present [Henry Hale](https://github.com/henryhale)
