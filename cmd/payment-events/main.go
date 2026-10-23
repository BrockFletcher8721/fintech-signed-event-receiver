package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/infrai-examples/fintech-signed-event-receiver/internal/infrai"
	"github.com/infrai-examples/fintech-signed-event-receiver/internal/receiver"
)

func main() {
	setup := flag.Bool("setup", false, "register webhook and queue push subscription")
	flag.Parse()
	secret := requireEnv("WEBHOOK_SECRET")
	if *setup {
		runSetup(secret)
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /events/payment", paymentHandler(secret))
	server := &http.Server{Addr: envOr("LISTEN_ADDR", ":8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	slog.Info("payment event receiver listening", "address", server.Addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("receiver stopped", "error", err)
		os.Exit(1)
	}
}

func runSetup(secret string) {
	client := &infrai.Client{
		BaseURL: envOr("INFRAI_BASE_URL", infrai.DefaultBaseURL),
		APIKey:  requireEnv("INFRAI_API_KEY"),
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	webhook, err := client.RegisterWebhook(ctx, requireEnv("PUBLIC_WEBHOOK_URL"), []string{"payment.succeeded", "payment.failed"}, secret)
	if err != nil {
		fatalAPI(err)
	}
	subscription, err := client.SubscribeQueue(ctx, envOr("EVENT_QUEUE", "fintech-payment-events"), requireEnv("PUBLIC_WEBHOOK_URL"))
	if err != nil {
		fatalAPI(err)
	}
	fmt.Printf("webhook=%s\nqueue_subscription=%s\n", webhook, subscription)
}

func paymentHandler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		decision, err := receiver.VerifyAndDecide(body, r.Header.Get("X-Webhook-Signature"), secret)
		if err != nil {
			slog.Warn("payment event rejected", "reason", err.Error())
			http.Error(w, "invalid signed event", http.StatusUnauthorized)
			return
		}
		slog.Info("payment event accepted", "event_id", decision.EventID, "payment_id", decision.PaymentID, "action", decision.Action, "reason", decision.Reason)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(decision)
	}
}

func fatalAPI(err error) {
	var apiErr *infrai.APIError
	if errors.As(err, &apiErr) {
		slog.Error("setup rejected", "code", apiErr.Code, "status", apiErr.HTTPStatus, "message", apiErr.Message)
	} else {
		slog.Error("setup failed", "error", err)
	}
	os.Exit(1)
}

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", name)
		os.Exit(2)
	}
	return value
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
