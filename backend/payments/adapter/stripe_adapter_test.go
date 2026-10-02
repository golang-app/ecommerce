package adapter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

func TestStripeProvider_Charge_Succeeded(t *testing.T) {
	var (
		receivedMethod string
		receivedPath   string
		receivedAuth   string
		receivedIdemp  string
		receivedBody   string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")
		receivedIdemp = r.Header.Get("Idempotency-Key")
		bodyBytes, _ := io.ReadAll(r.Body)
		receivedBody = string(bodyBytes)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "pi_1234567890",
			"status": "succeeded",
			"amount": 2500,
			"currency": "usd"
		}`))
	}))
	defer server.Close()

	provider := NewStripeProvider(StripeConfig{
		SecretKey:  "sk_test_fake_secret_key",
		Endpoint:   server.URL,
		HTTPClient: server.Client(),
	})

	res, err := provider.Charge(context.Background(), app.ChargeRequest{
		Amount:         2500,
		Currency:       "USD",
		Source:         "pm_card_visa",
		IdempotencyKey: "order_id_42",
	})
	if err != nil {
		t.Fatalf("unexpected charge error: %v", err)
	}

	if receivedMethod != http.MethodPost {
		t.Errorf("got method %s, want POST", receivedMethod)
	}
	if receivedPath != "/v1/payment_intents" {
		t.Errorf("got path %s, want /v1/payment_intents", receivedPath)
	}
	if receivedAuth != "Bearer sk_test_fake_secret_key" {
		t.Errorf("got auth %s, want Bearer sk_test_fake_secret_key", receivedAuth)
	}
	if receivedIdemp != "order_id_42" {
		t.Errorf("got idempotency key %s, want order_id_42", receivedIdemp)
	}
	if !strings.Contains(receivedBody, "amount=2500") {
		t.Errorf("expected amount=2500 in body, got: %s", receivedBody)
	}
	if !strings.Contains(receivedBody, "currency=usd") {
		t.Errorf("expected currency=usd in body, got: %s", receivedBody)
	}
	if !strings.Contains(receivedBody, "payment_method=pm_card_visa") {
		t.Errorf("expected payment_method=pm_card_visa in body, got: %s", receivedBody)
	}

	if res.ProviderRef != "pi_1234567890" {
		t.Errorf("got provider ref %s, want pi_1234567890", res.ProviderRef)
	}
	if res.Status != domain.StatusSucceeded {
		t.Errorf("got status %v, want %v", res.Status, domain.StatusSucceeded)
	}
}

func TestStripeProvider_Charge_RequiresAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "pi_requires_action_123",
			"status": "requires_action"
		}`))
	}))
	defer server.Close()

	provider := NewStripeProvider(StripeConfig{
		SecretKey:  "sk_test",
		Endpoint:   server.URL,
		HTTPClient: server.Client(),
	})

	res, err := provider.Charge(context.Background(), app.ChargeRequest{
		Amount:         1000,
		Currency:       "EUR",
		Source:         "tok_123",
		IdempotencyKey: "idem_1",
	})
	if err != nil {
		t.Fatalf("unexpected charge error: %v", err)
	}

	if res.Status != domain.StatusPending {
		t.Errorf("got status %v, want %v", res.Status, domain.StatusPending)
	}
	if res.ProviderRef != "pi_requires_action_123" {
		t.Errorf("got provider ref %s, want pi_requires_action_123", res.ProviderRef)
	}
}

func TestStripeProvider_Charge_CardDeclined(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{
			"error": {
				"type": "card_error",
				"code": "card_declined",
				"message": "Your card was declined."
			}
		}`))
	}))
	defer server.Close()

	provider := NewStripeProvider(StripeConfig{
		SecretKey:  "sk_test",
		Endpoint:   server.URL,
		HTTPClient: server.Client(),
	})

	res, err := provider.Charge(context.Background(), app.ChargeRequest{
		Amount:         1000,
		Currency:       "USD",
		Source:         "pm_card_declined",
		IdempotencyKey: "idem_2",
	})
	if err == nil {
		t.Fatal("expected error for declined card, got nil")
	}
	if res.Status != domain.StatusFailed {
		t.Errorf("got status %v, want %v", res.Status, domain.StatusFailed)
	}
	if !strings.Contains(err.Error(), "Your card was declined.") {
		t.Errorf("expected error to mention decline message, got: %v", err)
	}
}
