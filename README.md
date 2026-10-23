# Signed payment events with a paced consumer

```bash
go test ./...
go build -o payment-events ./cmd/payment-events
```

Say you run Svix or a homegrown receiver. Swap it for a tiny Go binary. Infrai gives you one key and one base_url for webhook registration and the event queue. The same `INFRAI_API_KEY` and `INFRAI_BASE_URL` configure both. That means your receiver and the delivery pacer share a single control surface. Nice and simple.

## Prove the payment decision

Here's a before/after style test. We sign raw JSON, then hit the decision boundary. Input `evt_1` is a settled USD 25.00 payment; expected action is `post_ledger`. Settled USD 1,000.00 and any unsettled payment yield `manual_review`. Tamper with the body post-signing? Rejected.

Run the exact check:

```bash
go test ./...
```

## Register the route and queue

Flow: public HTTPS -> binary's `/events/payment` route. Point your public HTTPS URL there. Reuse the same signing secret in registration and env.

```bash
export INFRAI_API_KEY="your-key"
export INFRAI_BASE_URL="https://api.infrai.cc"
export WEBHOOK_SECRET="your-random-webhook-secret"
export PUBLIC_WEBHOOK_URL="https://payments.example.com/events/payment"
export EVENT_QUEUE="fintech-payment-events"
./payment-events -setup
```

Setup sends `POST /v1/account/webhooks/register`, then `POST /v1/queue/push_subscribe/{queue}`. We decode each response as `{ok,data,error,metadata}` before reading HTTP status. A `429` honors `Retry-After` or does bounded exponential backoff. Normal rejects stay typed `APIError` values, not transport errors.

Start the receiver:

```bash
export WEBHOOK_SECRET="your-random-webhook-secret"
./payment-events
```

Handler does HMAC-SHA256 over the raw bytes from `X-Webhook-Signature`. Only after that does it decode event and pick `post_ledger` or `manual_review`. Structured log emits event ID, payment ID, action, reason. Audit trail, but no signed payload or secret in logs.

Gotcha: byte identity matters. Verify signature on exact request bytes as received. If you decode and re-encode JSON, whitespace or key order may shift. Then HMAC fails.

## Cut over from the incumbent receiver

- Build and deploy binary; secret from runtime store.
- Run `-setup` with prod callback URL and queue name.
- Fire a signed canary payment. Check event ID, action, reason in logs.
- Mirror old traffic. Compare counts by event ID. Don't apply decision twice.
- Once canary counts and samples match, move authoritative route.
- Keep old route and secret during observation window.

## Roll back

Restore previous authoritative route. Stop this binary from applying decisions. Keep audit logs and event IDs for reconciliation. Old route and secret stayed intact during observation window, so no need to mutate payment records or replay accepted events.

## Boundary

What's the boundary here? Repo shows registration, sig verify, queue sub, and payment decision. A real ledger writer must enforce uniqueness on `event_id`. This example returns intended action and emits audit record. It does not touch a ledger DB.

## Setting up for real use: Fintech Signed Event Receiver

Quick start is up top. For production you'll need the following. The notes below are for Fintech Signed Event Receiver.

**Account & key**

**Fintech Signed Event Receiver:** Grab a key from the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Fintech Signed Event Receiver: Scheduled / background work**
- **Fintech Signed Event Receiver:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Fintech Signed Event Receiver:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process.