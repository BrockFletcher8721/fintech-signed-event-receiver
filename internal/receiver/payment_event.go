package receiver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type PaymentEvent struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Payment  Payment `json:"payment"`
	Occurred string  `json:"occurred_at"`
}

type Payment struct {
	ID          string `json:"id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
}

type Decision struct {
	EventID   string `json:"event_id"`
	PaymentID string `json:"payment_id"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
}

func VerifyAndDecide(body []byte, signature, secret string) (Decision, error) {
	if secret == "" {
		return Decision{}, errors.New("webhook secret is empty")
	}
	want, err := hex.DecodeString(signature)
	if err != nil {
		return Decision{}, errors.New("signature is not hexadecimal")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	if !hmac.Equal(want, mac.Sum(nil)) {
		return Decision{}, errors.New("signature does not match")
	}

	var event PaymentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return Decision{}, fmt.Errorf("decode payment event: %w", err)
	}
	if event.ID == "" || event.Payment.ID == "" {
		return Decision{}, errors.New("event id and payment id are required")
	}
	return Decide(event), nil
}

func Decide(event PaymentEvent) Decision {
	d := Decision{EventID: event.ID, PaymentID: event.Payment.ID}
	switch {
	case event.Payment.Status != "succeeded":
		d.Action, d.Reason = "manual_review", "payment_not_settled"
	case event.Payment.AmountCents >= 100000:
		d.Action, d.Reason = "manual_review", "high_value_payment"
	default:
		d.Action, d.Reason = "post_ledger", "settled_within_policy"
	}
	return d
}
