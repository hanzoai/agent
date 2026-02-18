package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/hanzoai/agents/control-plane/internal/logger"
)

// Sentinel errors for billing operations.
var (
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrCommerceUnavailable = errors.New("commerce service unavailable")
)

// BillingConfig holds billing configuration.
type BillingConfig struct {
	CommerceURL string // BILLING_COMMERCE_URL
	AdminToken  string // BILLING_ADMIN_TOKEN
	Currency    string // default "usd"
}

// DebitParams describes a billing debit (withdraw) request.
type DebitParams struct {
	User        string
	AmountCents int64
	Currency    string
	Model       string
	Provider    string
	Tokens      int
	ExecutionID string
	BotID       string
	Notes       string
}

// BalanceResponse represents the Commerce API balance response.
type BalanceResponse struct {
	User      string `json:"user"`
	Currency  string `json:"currency"`
	Balance   int64  `json:"balance"`
	Holds     int64  `json:"holds"`
	Available int64  `json:"available"`
}

// UsageResponse represents the Commerce API usage recording response.
type UsageResponse struct {
	TransactionID string `json:"transactionId"`
	User          string `json:"user"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	Type          string `json:"type"`
}

// RefundResponse represents the Commerce API refund response.
type RefundResponse struct {
	TransactionID string `json:"transactionId"`
	User          string `json:"user"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
}

// BillingService provides billing operations via the Commerce API.
// Billing is always enabled — every execution is balance-gated and charged.
type BillingService struct {
	commerceURL string
	adminToken  string
	client      *http.Client
	currency    string
}

// NewBillingService creates a new BillingService from config.
func NewBillingService(cfg BillingConfig) *BillingService {
	cur := cfg.Currency
	if cur == "" {
		cur = "usd"
	}
	return &BillingService{
		commerceURL: cfg.CommerceURL,
		adminToken:  cfg.AdminToken,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		currency: cur,
	}
}

// CheckBalance queries Commerce for the user's available balance (in cents).
// Returns ErrCommerceUnavailable if Commerce is down (fail-safe: blocks execution).
func (b *BillingService) CheckBalance(ctx context.Context, userID string) (int64, error) {
	u, err := url.Parse(b.commerceURL + "/api/v1/billing/balance")
	if err != nil {
		return 0, fmt.Errorf("parse commerce URL: %w", err)
	}
	q := u.Query()
	q.Set("user", userID)
	q.Set("currency", b.currency)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("create balance request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.adminToken)
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).Str("user", userID).Msg("commerce balance check failed")
		return 0, ErrCommerceUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return 0, ErrCommerceUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("commerce balance error (%d): %s", resp.StatusCode, string(body))
	}

	var bal BalanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&bal); err != nil {
		return 0, fmt.Errorf("decode balance response: %w", err)
	}
	return bal.Available, nil
}

// DebitActualCost charges the user for actual usage after execution completes.
// Calls Commerce POST /billing/usage to create a Withdraw transaction.
func (b *BillingService) DebitActualCost(ctx context.Context, params DebitParams) (string, error) {
	return b.recordUsage(ctx, params)
}

// DebitUpfront charges a known cost before execution (e.g. Mac VM 24hr minimum).
// Uses the same usage endpoint — the cost is known in advance.
func (b *BillingService) DebitUpfront(ctx context.Context, params DebitParams) (string, error) {
	return b.recordUsage(ctx, params)
}

// Refund creates a correction deposit for an overcharge.
func (b *BillingService) Refund(ctx context.Context, userID string, amountCents int64, originalTxID, notes string) error {
	cur := b.currency

	payload := map[string]interface{}{
		"user":                  userID,
		"currency":              cur,
		"amount":                amountCents,
		"originalTransactionId": originalTxID,
		"notes":                 notes,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal refund request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.commerceURL+"/api/v1/billing/refund", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create refund request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("refund request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refund error (%d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// recordUsage creates a Withdraw transaction in Commerce for usage billing.
func (b *BillingService) recordUsage(ctx context.Context, params DebitParams) (string, error) {
	cur := params.Currency
	if cur == "" {
		cur = b.currency
	}

	payload := map[string]interface{}{
		"user":     params.User,
		"currency": cur,
		"amount":   params.AmountCents,
		"model":    params.Model,
		"provider": params.Provider,
	}
	if params.Tokens > 0 {
		payload["totalTokens"] = params.Tokens
	}
	if params.ExecutionID != "" {
		payload["requestId"] = params.ExecutionID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal usage request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.commerceURL+"/api/v1/billing/usage", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).
			Str("user", params.User).
			Str("execution_id", params.ExecutionID).
			Int64("amount_cents", params.AmountCents).
			Msg("billing debit failed — will retry")
		return "", fmt.Errorf("usage request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("usage error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result UsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode usage response: %w", err)
	}
	return result.TransactionID, nil
}
