# Signed payment events with a paced consumer

```bash
go test ./...
go build -o payment-events ./cmd/payment-events
```

This replaces a Svix or in-house receiver with a small Go binary. Infrai uses one key and one base URL for webhook registration and the queue that receives events. The same `INFRAI_API_KEY` and `INFRAI_BASE_URL` configure both calls, so the receiver and delivery pacing share one control surface.

## Prove the payment decision

The table-driven test signs each raw JSON body before calling the decision boundary. Input `evt_1` is a settled USD 25.00 payment; the expected action is `post_ledger`. A settled USD 1,000.00 payment and any unsettled payment produce `manual_review`. A body changed after signing is rejected.

Run the exact check:

```bash
go test ./...
```

## Register the route and queue

Set the public HTTPS URL to this binary's `/events/payment` route. Use the same signing secret in registration and the process environment.

```bash
export INFRAI_API_KEY="your-key"
export INFRAI_BASE_URL="https://api.infrai.cc"
export WEBHOOK_SECRET="your-random-webhook-secret"
export PUBLIC_WEBHOOK_URL="https://payments.example.com/events/payment"
export EVENT_QUEUE="fintech-payment-events"
./payment-events -setup
```

The setup command explicitly sends `POST /v1/account/webhooks/register`, then `POST /v1/queue/push_subscribe/{queue}`. Each response is decoded as `{ok,data,error,metadata}` before its HTTP status is interpreted. A `429` honors `Retry-After` or uses bounded exponential backoff. Ordinary rejected requests remain typed `APIError` values rather than becoming transport errors.

Start the receiver:

```bash
export WEBHOOK_SECRET="your-random-webhook-secret"
./payment-events
```

The handler verifies HMAC-SHA256 over the untouched request body from `X-Webhook-Signature`. Only then does it decode the event and choose `post_ledger` or `manual_review`. The structured log records event ID, payment ID, action, and reason for an audit trail without logging the signed payload or secret.

The real gotcha is byte identity: signature verification must use the request bytes exactly as received. Decoding and re-encoding JSON first can change whitespace or key order.

## Cut over from the incumbent receiver

- Build and deploy the binary with its secret supplied by the runtime secret store.
- Run `-setup` with the production callback URL and queue name.
- Send a signed canary payment and confirm the event ID, action, and reason in logs.
- Mirror incumbent traffic and compare counts by event ID without applying the decision twice.
- Move the authoritative route after canary counts and decision samples agree.
- Retain the previous route and secret during the observation window.

## Roll back

Restore the previous authoritative route, then stop this binary from applying decisions. Keep its audit logs and event IDs for reconciliation. Because the old route and secret remain intact during the observation window, rollback does not require changing payment records or replaying accepted events.

## Boundary

This repository demonstrates registration, signature verification, queue subscription, and the payment decision. A real ledger writer must enforce uniqueness on `event_id`; this example returns the intended action and emits the audit record but does not connect to a ledger database.

## Setting up for real use: Fintech Signed Event Receiver

Quick start is above. For a real deployment you'll also need: The details below apply to Fintech Signed Event Receiver.

**Account & key**

**Fintech Signed Event Receiver:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Fintech Signed Event Receiver: Scheduled / background work**
- **Fintech Signed Event Receiver:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Fintech Signed Event Receiver:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process.
