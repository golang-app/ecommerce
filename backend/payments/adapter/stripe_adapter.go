package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

// StripeConfig configures the Stripe payment gateway provider.
type StripeConfig struct {
	SecretKey  string
	Endpoint   string // Defaults to "https://api.stripe.com" if empty
	HTTPClient *http.Client
}

// StripeProvider integrates with Stripe's PaymentIntents API satisfying app.Provider.
type StripeProvider struct {
	secretKey  string
	endpoint   string
	httpClient *http.Client
}

var _ app.Provider = (*StripeProvider)(nil)

// NewStripeProvider creates a new Stripe payment gateway adapter.
func NewStripeProvider(cfg StripeConfig) *StripeProvider {
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.stripe.com"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &StripeProvider{
		secretKey:  cfg.SecretKey,
		endpoint:   endpoint,
		httpClient: client,
	}
}

type stripeErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

type stripePaymentIntentResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Charge creates and confirms a Stripe PaymentIntent with client-side tokenized payment method.
func (p *StripeProvider) Charge(ctx context.Context, req app.ChargeRequest) (app.ChargeResult, error) {
	data := url.Values{}
	data.Set("amount", strconv.FormatInt(req.Amount, 10))
	data.Set("currency", strings.ToLower(req.Currency))
	data.Set("confirm", "true")

	// In Stripe, tokenized methods starting with "pm_" are payment_method;
	// "tok_" or legacy tokens are passed via source or payment_method.
	if strings.HasPrefix(req.Source, "pm_") {
		data.Set("payment_method", req.Source)
	} else if req.Source != "" {
		data.Set("payment_method_data[type]", "card")
		data.Set("payment_method_data[card][token]", req.Source)
	}

	reqURL := p.endpoint + "/v1/payment_intents"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return app.ChargeResult{}, fmt.Errorf("create stripe request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if req.IdempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return app.ChargeResult{}, fmt.Errorf("stripe request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return app.ChargeResult{}, fmt.Errorf("read stripe response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp stripeErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
			return app.ChargeResult{Status: domain.StatusFailed}, fmt.Errorf("stripe error: %s (%s)", errResp.Error.Message, errResp.Error.Code)
		}
		return app.ChargeResult{Status: domain.StatusFailed}, fmt.Errorf("stripe returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var piResp stripePaymentIntentResponse
	if err := json.Unmarshal(body, &piResp); err != nil {
		return app.ChargeResult{}, fmt.Errorf("decode stripe payment intent: %w", err)
	}

	status, err := translateStripeStatus(piResp.Status)
	if err != nil {
		return app.ChargeResult{}, err
	}

	return app.ChargeResult{
		Status:      status,
		ProviderRef: piResp.ID,
	}, nil
}

func translateStripeStatus(status string) (domain.Status, error) {
	switch status {
	case "succeeded":
		return domain.StatusSucceeded, nil
	case "requires_action", "requires_confirmation", "requires_capture", "processing":
		return domain.StatusPending, nil
	case "requires_payment_method", "canceled":
		return domain.StatusFailed, nil
	default:
		return "", fmt.Errorf("stripe adapter: unknown status %q", status)
	}
}
