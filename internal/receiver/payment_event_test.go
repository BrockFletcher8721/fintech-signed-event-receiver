package receiver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyAndDecide(t *testing.T) {
	secret := "local-test-secret"
	tests := []struct {
		name      string
		body      string
		signature func([]byte) string
		want      string
		wantErr   bool
	}{
		{"settled payment posts", `{"id":"evt_1","type":"payment.succeeded","payment":{"id":"pay_1","amount_cents":2500,"currency":"USD","status":"succeeded"},"occurred_at":"2026-09-13T10:00:00Z"}`, sign(secret), "post_ledger", false},
		{"high value payment pauses", `{"id":"evt_2","type":"payment.succeeded","payment":{"id":"pay_2","amount_cents":100000,"currency":"USD","status":"succeeded"},"occurred_at":"2026-09-13T10:01:00Z"}`, sign(secret), "manual_review", false},
		{"failed payment pauses", `{"id":"evt_3","type":"payment.failed","payment":{"id":"pay_3","amount_cents":2500,"currency":"USD","status":"failed"},"occurred_at":"2026-09-13T10:02:00Z"}`, sign(secret), "manual_review", false},
		{"altered payload rejected", `{"id":"evt_4","type":"payment.succeeded","payment":{"id":"pay_4","amount_cents":2500,"currency":"USD","status":"succeeded"},"occurred_at":"2026-09-13T10:03:00Z"}`, func([]byte) string { return sign(secret)([]byte("different")) }, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			got, err := VerifyAndDecide(body, tc.signature(body), secret)
			if (err != nil) != tc.wantErr {
				t.Fatalf("VerifyAndDecide() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got.Action != tc.want {
				t.Fatalf("action = %q, want %q", got.Action, tc.want)
			}
		})
	}
}

func sign(secret string) func([]byte) string {
	return func(body []byte) string {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		return hex.EncodeToString(mac.Sum(nil))
	}
}
