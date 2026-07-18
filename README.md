# message-delivery

Generic message delivery worker for email and phone messages.

The service consumes `message.delivery.requested` events, renders templates, chooses a provider plan, sends the message through a provider adapter, and publishes `message.delivery.result`.

It is intentionally not tied to any product domain. Producers such as `auth-service` and application APIs should publish the same generic delivery contract with different templates and variables.

## Features

- RabbitMQ consumer and result publisher
- Generic delivery request/result event contract
- Email and phone recipient types
- Provider fallback chain
- User-selected provider support
- In-memory idempotency for local/test runs
- Template rendering with locale fallback
- Fake providers for local and integration tests
- Health endpoint
- Makefile, systemd unit, Dockerfile, `.deb` packaging

## Architecture

`message-delivery` is a worker service, not a domain API. It does not know how users register, reset passwords, create orders or receive product notifications. Producers send generic delivery events, and this service only handles message rendering, provider choice, delivery and result reporting.

Main components:

| Component | Responsibility |
|---|---|
| `cmd/main.go` | Loads config, builds dependencies, starts health server and worker loop. |
| `internal/config` | Reads JSON config, applies env overrides, validates required broker/provider/template settings. |
| `internal/broker` | Owns RabbitMQ connection, exchange/queue/DLQ declaration, consuming request events and publishing result events. |
| `internal/worker` | Reads RabbitMQ deliveries, decodes requests, acknowledges successful handling and dead-letters invalid messages. |
| `internal/delivery` | Contains request/result contracts and the orchestration algorithm. |
| `internal/template` | Renders configured templates using event variables and locale fallback. |
| `internal/provider` | Defines provider interfaces, registry, fake/unavailable providers and concrete adapters. |
| `tests/integration` | Runs real RabbitMQ consume/publish flow in Docker Compose. |

Processing flow:

```text
producer
  -> RabbitMQ exchange messages.events
  -> routing key message.delivery.requested
  -> queue message.delivery.requests
  -> worker
  -> validate event
  -> render template
  -> choose provider plan
  -> send via provider adapter
  -> publish message.delivery.result
```

Failure flow:

```text
invalid JSON / invalid contract
  -> message is rejected without requeue
  -> RabbitMQ routes it to message.delivery.requests.dlq

provider returns undeliverable
  -> orchestrator tries the next provider if fallback is enabled

provider returns failed
  -> orchestrator stops and publishes failed result

worker/orchestrator internal error
  -> message is nacked with requeue=true
```

Idempotency is currently in-memory and keyed by `event_id`. This is enough for local/test runs and protects against duplicate delivery while the process is alive. Production-grade idempotency should be moved to Redis/PostgreSQL before horizontal scaling.

Provider adapters are intentionally isolated behind:

```text
Send(ctx, message) -> Result
```

That keeps Telegram, WhatsApp, SMS, SMTP, SES, Mailgun or other providers replaceable without changing producer contracts. Auth codes and future notifications use the same delivery mechanism; only `template`, `purpose`, `recipient_type`, `recipient` and `variables` differ.

Configuration is split into:

- tracked JSON config for non-secret routing, templates and adapter shape;
- env/runtime secrets for tokens, passwords and external credentials;
- env overrides for deployment-specific broker/port settings.

This repository contains the service artifact and local/test runtime. Kubernetes or production infrastructure manifests should live in a separate deploy repository.

## Event Contract

### Request

```json
{
  "version": "v1",
  "event_id": "uuid-or-random-id",
  "type": "message.delivery.requested",
  "source": "auth-service",
  "template": "auth_verification_code",
  "purpose": "verification",
  "recipient_type": "phone",
  "recipient": "+10000000000",
  "variables": {
    "code": "123456",
    "ttl_sec": "300"
  },
  "user_id": 123,
  "created_at": "2026-07-18T18:00:00Z",
  "delivery": {
    "selected_provider": "",
    "provider_chain": ["telegram", "whatsapp", "sms"],
    "allow_fallback": true
  },
  "metadata": {
    "device_uid": "device-1",
    "locale": "en"
  }
}
```

### Result

```json
{
  "version": "v1",
  "event_id": "uuid",
  "type": "message.delivery.result",
  "request_event_id": "uuid-or-random-id",
  "status": "sent",
  "recipient_type": "phone",
  "recipient": "+10000000000",
  "provider": "sms",
  "attempt": 3,
  "created_at": "2026-07-18T18:00:01Z"
}
```

## Provider Policy

For phone recipients, the default provider chain is:

```text
telegram -> whatsapp -> sms
```

If `delivery.selected_provider` is set and `allow_fallback=false`, only that provider is tried. If `allow_fallback=true`, the selected provider is tried first and the chain continues on `undeliverable`.

The current implementation includes fake providers for deterministic local tests:

- `fake-email` sends successfully.
- `telegram` returns `undeliverable`.
- `whatsapp` returns `undeliverable`.
- `sms` sends successfully.

Supported adapter kinds:

- `fake` for local and integration tests.
- `smtp` for generic email delivery.
- `telegram-gateway` for Telegram Gateway verification messages.

WhatsApp and Twilio/SMS adapters are intentionally registered as explicit `not_configured` placeholders until provider credentials and exact contracts are selected.

## Configuration

Copy and edit the example config:

```bash
cp message-delivery.example.json message-delivery.json
```

Secrets must come from environment variables or the runtime secret mechanism, not from tracked config.

Useful environment overrides:

| Variable | Description |
|---|---|
| `MESSAGE_DELIVERY_HOST` | HTTP bind host |
| `MESSAGE_DELIVERY_PORT` | HTTP health port |
| `MESSAGE_DELIVERY_BROKER_HOST` | RabbitMQ host, for example `127.0.0.1:5673` |
| `MESSAGE_DELIVERY_BROKER_USER` | RabbitMQ username |
| `MESSAGE_DELIVERY_BROKER_PASSWORD` | RabbitMQ password |
| `MESSAGE_DELIVERY_BROKER_PREFETCH` | Consumer prefetch count |
| `RABBITMQ_PASSWORD` | Password used when `Broker.PasswordEnv` points to it |
| `TELEGRAM_GATEWAY_API_TOKEN` | Telegram Gateway adapter token |

For local tests, `message-delivery.example.json` uses fake phone providers. To send through Telegram Gateway, switch the `telegram` adapter:

```json
{
  "Enabled": true,
  "Kind": "telegram-gateway",
  "BaseURL": "https://gatewayapi.telegram.org",
  "ApiTokenEnv": "TELEGRAM_GATEWAY_API_TOKEN"
}
```

The token must be exported in the runtime environment, not committed to the config file.

## Usage

`message-delivery` has no public send HTTP API. Producers use RabbitMQ:

1. Publish a `message.delivery.requested` JSON event to the configured exchange.
2. Use routing key `message.delivery.requested`.
3. Read `message.delivery.result` events from a queue bound to routing key `message.delivery.result`.
4. Match the result with the original request by `request_event_id`.

Default broker settings from `message-delivery.example.json`:

| Setting | Value |
|---|---|
| Exchange | `messages.events` |
| Exchange type | `topic` |
| Request routing key | `message.delivery.requested` |
| Result routing key | `message.delivery.result` |
| Consumer queue | `message.delivery.requests` |
| Dead-letter queue | `message.delivery.requests.dlq` |

### Start with Isolated RabbitMQ

For a fully local smoke test:

```bash
docker compose -f docker-compose.integration.yml up --build rabbitmq message-delivery
```

RabbitMQ management UI is exposed on:

```text
http://localhost:15674
```

Default credentials in the integration compose are `guest` / `guest`.

### Publish a Phone Verification Code

Use the manual client instead of RabbitMQ UI:

```bash
go run ./cmd/send-test-message \
  --config message-delivery.example.json \
  --recipient-type phone \
  --recipient +15551234567 \
  --template auth_verification_code \
  --purpose registration_verification \
  --code 123456 \
  --ttl-sec 300 \
  --provider-chain telegram,whatsapp,sms \
  --allow-fallback=true \
  --wait-result=true
```

The command publishes `message.delivery.requested`, waits for the matching `message.delivery.result`, and prints the result JSON.

Example request for phone verification:

```json
{
  "version": "v1",
  "event_id": "manual-phone-1",
  "type": "message.delivery.requested",
  "source": "auth-service",
  "template": "auth_verification_code",
  "purpose": "registration_verification",
  "recipient_type": "phone",
  "recipient": "+15551234567",
  "variables": {
    "code": "123456",
    "ttl_sec": "300"
  },
  "user_id": 123,
  "created_at": "2026-07-18T18:00:00Z",
  "delivery": {
    "selected_provider": "",
    "provider_chain": ["telegram", "whatsapp", "sms"],
    "allow_fallback": true
  },
  "metadata": {
    "locale": "en",
    "device_uid": "device-1"
  }
}
```

Publish it with `rabbitmqadmin` against the integration RabbitMQ:

```bash
rabbitmqadmin \
  --host localhost \
  --port 15674 \
  --username guest \
  --password guest \
  publish \
  exchange=messages.events \
  routing_key=message.delivery.requested \
  payload='{"version":"v1","event_id":"manual-phone-1","type":"message.delivery.requested","source":"auth-service","template":"auth_verification_code","purpose":"registration_verification","recipient_type":"phone","recipient":"+15551234567","variables":{"code":"123456","ttl_sec":"300"},"user_id":123,"created_at":"2026-07-18T18:00:00Z","delivery":{"selected_provider":"","provider_chain":["telegram","whatsapp","sms"],"allow_fallback":true},"metadata":{"locale":"en","device_uid":"device-1"}}' \
  properties='{"content_type":"application/json","delivery_mode":2}'
```

With the default example config this uses fake providers: Telegram and WhatsApp return `undeliverable`, then SMS returns `sent`.

After `make build`, the same command is available as:

```bash
bin/message-delivery-send \
  --config message-delivery.example.json \
  --recipient-type phone \
  --recipient +15551234567 \
  --code 123456
```

This default example does not send a real Telegram message. It is a local smoke test for RabbitMQ wiring and orchestration.

### Send a Real Telegram Gateway Code

Use `message-delivery.telegram.example.json` when you want real Telegram Gateway delivery through the full service path.

Start RabbitMQ:

```bash
docker compose -f docker-compose.integration.yml up -d rabbitmq
```

Start `message-delivery` with the Telegram Gateway config:

```bash
export RABBITMQ_PASSWORD=guest
export MESSAGE_DELIVERY_BROKER_HOST=127.0.0.1:5674
export TELEGRAM_GATEWAY_API_TOKEN=...

go run ./cmd/main.go --config message-delivery.telegram.example.json
```

In another terminal, publish a real Telegram request through the manual client:

```bash
export RABBITMQ_PASSWORD=guest
export MESSAGE_DELIVERY_BROKER_HOST=127.0.0.1:5674

go run ./cmd/send-test-message \
  --config message-delivery.telegram.example.json \
  --recipient-type phone \
  --recipient +15551234567 \
  --event-id manual-telegram-live-1 \
  --code 123456 \
  --provider telegram \
  --allow-fallback=false \
  --wait-result=true \
  --timeout=20s
```

Expected result:

```json
{
  "type": "message.delivery.result",
  "request_event_id": "manual-telegram-live-1",
  "status": "sent",
  "recipient_type": "phone",
  "provider": "telegram",
  "attempt": 1
}
```

If the result is `sent`, Telegram Gateway accepted the send request. Delivery to the phone still depends on Telegram Gateway rules, account binding and provider limits.

### Publish an Email Message

Use the manual client:

```bash
go run ./cmd/send-test-message \
  --config message-delivery.example.json \
  --recipient-type email \
  --recipient user@example.com \
  --template auth_password_reset \
  --purpose password_reset \
  --code 654321 \
  --wait-result=true
```

Example request for email delivery:

```json
{
  "version": "v1",
  "event_id": "manual-email-1",
  "type": "message.delivery.requested",
  "source": "auth-service",
  "template": "auth_password_reset",
  "purpose": "password_reset",
  "recipient_type": "email",
  "recipient": "user@example.com",
  "variables": {
    "code": "654321",
    "ttl_sec": "300"
  },
  "created_at": "2026-07-18T18:00:00Z",
  "delivery": {
    "selected_provider": "",
    "provider_chain": [],
    "allow_fallback": true
  },
  "metadata": {
    "locale": "en"
  }
}
```

For email, the service uses `Providers.Email.DefaultProvider` unless `delivery.selected_provider` is set.

### Manual Client Flags

| Flag | Description |
|---|---|
| `--config` | Path to service config JSON. Defaults to `message-delivery.example.json`. |
| `--recipient-type` | `phone` or `email`. |
| `--recipient` | Phone in E.164 format or email address. Required. |
| `--template` | Template key from config, for example `auth_verification_code`. |
| `--purpose` | Producer-defined delivery purpose. |
| `--source` | Producer name. Defaults to `manual-client`. |
| `--event-id` | Optional idempotency key. Generated when empty. |
| `--code` | Sets `variables.code`. |
| `--ttl-sec` | Sets `variables.ttl_sec`. |
| `--variables` | Additional variables as JSON object, for example `{\"name\":\"Anna\"}`. |
| `--metadata` | Additional metadata as JSON object, for example `{\"device_uid\":\"dev-1\"}`. |
| `--locale` | Sets `metadata.locale`. |
| `--provider` | Selected provider, for example `telegram`. |
| `--provider-chain` | Comma-separated chain, for example `telegram,whatsapp,sms`. |
| `--allow-fallback` | Whether to continue after `undeliverable`. |
| `--wait-result` | Wait for matching result event and print it. Defaults to `true`. |
| `--timeout` | Publish/result timeout. Defaults to `15s`. |

### Read Delivery Results

The manual client creates a temporary result queue automatically when `--wait-result=true`.

If you need to inspect results manually, create a temporary result queue and bind it to the result routing key:

```bash
rabbitmqadmin \
  --host localhost \
  --port 15674 \
  --username guest \
  --password guest \
  declare queue name=message.delivery.results.manual durable=false

rabbitmqadmin \
  --host localhost \
  --port 15674 \
  --username guest \
  --password guest \
  declare binding \
  source=messages.events \
  destination_type=queue \
  destination=message.delivery.results.manual \
  routing_key=message.delivery.result
```

Read one result:

```bash
rabbitmqadmin \
  --host localhost \
  --port 15674 \
  --username guest \
  --password guest \
  get queue=message.delivery.results.manual count=1 ackmode=ack_requeue_false
```

Expected result shape:

```json
{
  "version": "v1",
  "event_id": "generated-result-id",
  "type": "message.delivery.result",
  "request_event_id": "manual-phone-1",
  "status": "sent",
  "recipient_type": "phone",
  "recipient": "+15551234567",
  "provider": "sms",
  "attempt": 3,
  "created_at": "2026-07-18T18:00:01Z"
}
```

### Provider Selection

To force Telegram only:

```json
{
  "delivery": {
    "selected_provider": "telegram",
    "provider_chain": ["telegram", "whatsapp", "sms"],
    "allow_fallback": false
  }
}
```

To prefer WhatsApp but allow fallback:

```json
{
  "delivery": {
    "selected_provider": "whatsapp",
    "provider_chain": ["telegram", "whatsapp", "sms"],
    "allow_fallback": true
  }
}
```

The selected provider is tried first. If it returns `undeliverable` and fallback is enabled, the service continues through the configured chain without retrying the same provider twice.

## Local Run

Use an already running RabbitMQ, for example the shared local stand:

```bash
make run
```

The health endpoint is:

```text
http://localhost:8090/health
```

## Docker

Build the local/test image:

```bash
make docker-build
```

Run with an external RabbitMQ from config/env:

```bash
docker compose -f docker-compose.test.yml up --build
```

Run isolated integration tests with a dedicated RabbitMQ:

```bash
make docker-test
```

Kubernetes manifests are intentionally out of scope for this repository. External deployment repositories can consume the Docker image and the documented env/config contract.

## Testing

Run all tests:

```bash
make test
```

The tests cover:

- request event validation;
- provider plan calculation;
- template rendering;
- provider registry creation from config;
- fallback from Telegram to WhatsApp to SMS;
- selected provider without fallback;
- invalid provider handling;
- idempotency;
- RabbitMQ result publish;
- config loading and env overrides.
- isolated RabbitMQ consume/publish integration flow via `make docker-test`.

## Live Provider Tests

Live provider tests are disabled by default. They call real external APIs and may send real messages or spend provider balance.

Telegram Gateway live test:

```bash
export TELEGRAM_GATEWAY_LIVE_TEST=1
export TELEGRAM_GATEWAY_API_TOKEN=...
export TELEGRAM_GATEWAY_TEST_PHONE=+15551234567
# Optional: force an exact verification code instead of a random one.
export TELEGRAM_GATEWAY_TEST_CODE=123456
go test ./internal/provider/telegram -run TestGatewaySendVerificationMessageLive -count=1
```

Use the phone number tied to the Telegram Gateway account when you want Telegram's free test delivery path. The number must be in E.164 format.
Run with `-v` to see the exact code sent by the test.

## Build

Build Linux binaries:

```bash
make build
```

Build Debian packages:

```bash
make deb
```

## Install

```bash
make install
```

This installs:

- `/usr/bin/message-delivery`
- `/etc/systemd/system/message-delivery.service`
- `/etc/message-delivery/config.json`
