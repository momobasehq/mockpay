build:
	mkdir -p build
	go build -o build/mockpay ./cmd/mockpay

smoke:
	bash ./scripts/smoke-test.bash

## Force 100 % failure rate (useful for testing error paths)
force-fail:
	curl -s -X POST http://localhost:8080/admin/config \
	  -H "Content-Type: application/json" \
	  -d '{"failureRate":1.0}'

## Restore default 10 % failure rate, 300–3000 ms delay
restore:
	curl -s -X POST http://localhost:8080/admin/config \
	  -H "Content-Type: application/json" \
	  -d '{"failureRate":0.1,"minDelayMs":300,"maxDelayMs":3000}'

## Wipe all transactions
reset-state:
	curl -s -X DELETE http://localhost:8080/admin/reset
