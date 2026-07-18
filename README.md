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
go test ./internal/provider/telegram -run TestGatewaySendVerificationMessageLive -count=1
```

Use the phone number tied to the Telegram Gateway account when you want Telegram's free test delivery path. The number must be in E.164 format.

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
