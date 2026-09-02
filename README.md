<div align="center">

# MockPay

**A local mock payment gateway for MTN MoMo and Airtel Africa Money.**

[![Release](https://img.shields.io/github/v/release/momobasehq/mockpay)](https://github.com/momobasehq/mockpay/releases)
[![Container](https://img.shields.io/badge/container-ghcr.io-2496ED?logo=docker&logoColor=white)](https://github.com/momobasehq/mockpay/pkgs/container/mockpay)
[![Go](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Fiber](https://img.shields.io/badge/Fiber-v3-00ACD7)](https://gofiber.io)
[![License](https://img.shields.io/badge/license-MIT-green)](./LICENSE.txt)

Test payment integrations locally without calling production APIs.

</div>

> [!WARNING]
> MockPay is for local development and testing only. Do not use it in production.

## Features

- MTN MoMo collection and disbursement endpoints
- Airtel Money payment, disbursement, and refund endpoints
- Configurable processing delays and failure rates
- Asynchronous transaction processing and webhook callbacks
- Browser UI for configuration, transactions, and pending webhooks
- Pre-seeded credentials and in-memory state for quick, repeatable tests

## Quick start

### Docker

Released versions are published to GitHub Container Registry. Image tags match Git release tags; `latest` is not published.

```bash
docker run --rm --name mockpay -p 7676:7676 \
  ghcr.io/momobasehq/mockpay:v1.0.0
```

Open the simulation console at [http://localhost:7676](http://localhost:7676), or verify the server:

```bash
curl http://localhost:7676/admin/ready/
```

To build the container locally instead:

```bash
git clone https://github.com/momobasehq/mockpay.git
cd mockpay
docker compose up --build
```

### From source

Requires Go 1.26 or newer.

```bash
git clone https://github.com/momobasehq/mockpay.git
cd mockpay
make run
```

The server listens on port `7676`. Override it with the `PORT` environment variable.

## Make your first payment

The examples below use `curl`, `jq`, and `uuidgen`.

Get an MTN collection token:

```bash
TOKEN=$(curl -s -X POST http://localhost:7676/mtn/collection/token/ \
  -H "Authorization: Basic $(printf 'mock-api-user:mock-api-key' | base64)" \
  | jq -r .access_token)
```

Create a request to pay:

```bash
REF=$(uuidgen)
curl -X POST http://localhost:7676/mtn/collection/v1_0/requesttopay \
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

Check the result after the configured processing delay:

```bash
sleep 3
curl -s http://localhost:7676/mtn/collection/v1_0/requesttopay/$REF \
  -H "Authorization: Bearer $TOKEN" | jq
```

## Credentials

| Provider | Credential | Value |
| --- | --- | --- |
| MTN | API user | `mock-api-user` |
| MTN | API key | `mock-api-key` |
| MTN | Subscription key | `mock-oapi-subscription-key` |
| Airtel | Client ID and secret | Any values are accepted |

Send the MTN subscription key in the `Ocp-Apim-Subscription-Key` header when the upstream integration requires it.

## Simulation

The console at [http://localhost:7676](http://localhost:7676) provides tabs for configuration, transactions, and pending webhooks. No login is required.

| Setting | Default | Range |
| --- | --- | --- |
| Failure rate | 10% | 0–100% |
| Minimum delay | 300 ms | 0–3000 ms |
| Maximum delay | 3000 ms | 0–3000 ms |

All settings and transactions are held in memory and reset when the server stops.

### Admin API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/admin/ready/` | Health check |
| `GET` | `/admin/state/` | Configuration, transactions, and pending webhooks |
| `POST` | `/admin/config/` | Update simulation settings |
| `DELETE` | `/admin/reset/` | Clear transactions, tokens, and pending webhooks |

Update the simulation without using the browser:

```bash
curl -X POST http://localhost:7676/admin/config/ \
  -H "Content-Type: application/json" \
  -d '{"failureRate":0.25,"minDelayMs":200,"maxDelayMs":800}'
```

## Supported endpoints

### MTN MoMo

See the [MTN MoMo API documentation](https://mtn-momo-api-documentation.readthedocs.io/en/latest/) for the upstream API.

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/mtn/collection/token/` | Get a collection token |
| `POST` | `/mtn/collection/v1_0/requesttopay` | Initiate a collection |
| `GET` | `/mtn/collection/v1_0/requesttopay/:referenceId` | Get collection status |
| `GET` | `/mtn/collection/v1_0/account/balance` | Get collection balance |
| `POST` | `/mtn/disbursement/token/` | Get a disbursement token |
| `POST` | `/mtn/disbursement/v1_0/transfer` | Initiate a disbursement |
| `GET` | `/mtn/disbursement/v1_0/transfer/:referenceId` | Get disbursement status |
| `GET` | `/mtn/disbursement/v1_0/account/balance` | Get disbursement balance |

### Airtel Africa Money

See the [Airtel Africa developer portal](https://developers.airtel.africa/) for the upstream API.

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/airtel/auth/oauth2/token` | Get an access token |
| `POST` | `/airtel/merchant/v2/payments/` | Initiate a payment |
| `GET` | `/airtel/standard/v1/payments/:id` | Get payment status |
| `POST` | `/airtel/standard/v1/disbursements/` | Initiate a disbursement |
| `GET` | `/airtel/standard/v1/disbursements/:id` | Get disbursement status |
| `POST` | `/airtel/standard/v1/payments/refund` | Refund a payment |

## Webhooks

Send an `X-Callback-Url` header when creating a transaction. MockPay posts the final transaction payload to that URL after processing completes.

Webhook delivery is best-effort with a 10-second timeout and no retries. In-flight deliveries appear in the simulation console.

## Development

```bash
make build     # Build build/mockpay
make run       # Run the server
make smoke     # Exercise the running server
go test ./...  # Run the Go tests
```

## Limitations

- State and tokens are not persisted across restarts.
- Webhook delivery is not retried.
- MockPay runs as a single instance with no shared state.

## Contributing

Issues and pull requests are welcome.

## License

Released under the [MIT License](./LICENSE.txt).

© 2026-present [Henry Hale](https://github.com/henryhale)
