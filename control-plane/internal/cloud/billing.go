package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// BillingAuthorizer checks billing authorization before provisioning.
type BillingAuthorizer interface {
	// AuthorizeProvisioning checks if a team can provision an instance.
	AuthorizeProvisioning(ctx context.Context, teamID string, platform Platform, instanceType string) (*BillingAuth, error)
	// ReportUsage reports compute hours for billing.
	ReportUsage(ctx context.Context, instanceID string, platform Platform, hours float64, hourlyCents int) error
	// GetTeamQuota returns the cloud compute quota for a team.
	GetTeamQuota(ctx context.Context, teamID string) (*CloudQuota, error)
}

// BillingAuth is the result of an authorization check.
type BillingAuth struct {
	Authorized     bool   `json:"authorized"`
	Tier           string `json:"tier"`
	HourlyCents    int    `json:"hourly_rate_cents"`
	Reason         string `json:"reason,omitempty"`
	BillingAccount string `json:"billing_account_id,omitempty"`
}

// CloudQuota holds the cloud compute quota for a team/tier.
type CloudQuota struct {
	Tier               string `json:"tier"`
	MaxLinuxInstances  int    `json:"max_linux_instances"`
	MaxWindowsInstances int   `json:"max_windows_instances"`
	MaxMacOSInstances  int    `json:"max_macos_instances"`
	MaxComputeHours    int    `json:"max_compute_hours_monthly"` // 0 = unlimited
	UsedLinux          int    `json:"used_linux"`
	UsedWindows        int    `json:"used_windows"`
	UsedMacOS          int    `json:"used_macos"`
	UsedComputeHours   float64 `json:"used_compute_hours"`
	MonthlyBudgetCents int    `json:"monthly_budget_cents"` // 0 = unlimited
	UsedBudgetCents    int    `json:"used_budget_cents"`
}

// BillingConfig holds configuration for billing integration.
type BillingConfig struct {
	Enabled    bool   `yaml:"enabled" mapstructure:"enabled"`
	ServiceURL string `yaml:"service_url" mapstructure:"service_url"` // bootnode API URL
	APIKey     string `yaml:"api_key" mapstructure:"api_key"`
}

// HTTPBillingClient calls the bootnode billing API over HTTP.
type HTTPBillingClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewHTTPBillingClient creates a billing client that calls the bootnode API.
func NewHTTPBillingClient(baseURL, apiKey string) *HTTPBillingClient {
	return &HTTPBillingClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *HTTPBillingClient) AuthorizeProvisioning(ctx context.Context, teamID string, platform Platform, instanceType string) (*BillingAuth, error) {
	body, _ := json.Marshal(map[string]string{
		"team_id":       teamID,
		"platform":      string(platform),
		"instance_type": instanceType,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/billing/cloud/authorize", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("billing request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("billing service unreachable: %w", err)
	}
	defer resp.Body.Close()

	var auth BillingAuth
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return nil, fmt.Errorf("billing response decode failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return &auth, nil
	}

	return &auth, nil
}

func (c *HTTPBillingClient) ReportUsage(ctx context.Context, instanceID string, platform Platform, hours float64, hourlyCents int) error {
	body, _ := json.Marshal(map[string]interface{}{
		"instance_id":      instanceID,
		"platform":         string(platform),
		"compute_hours":    hours,
		"hourly_rate_cents": hourlyCents,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/billing/cloud/usage", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("usage report request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Warn().Err(err).Str("instance", instanceID).Msg("failed to report usage to billing")
		return nil // non-fatal: don't block provisioning
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Int("status", resp.StatusCode).Str("instance", instanceID).Msg("billing usage report returned error")
	}

	return nil
}

func (c *HTTPBillingClient) GetTeamQuota(ctx context.Context, teamID string) (*CloudQuota, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/billing/cloud/quota?team_id="+teamID, nil)
	if err != nil {
		return nil, fmt.Errorf("quota request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("billing service unreachable: %w", err)
	}
	defer resp.Body.Close()

	var quota CloudQuota
	if err := json.NewDecoder(resp.Body).Decode(&quota); err != nil {
		return nil, fmt.Errorf("quota response decode failed: %w", err)
	}

	return &quota, nil
}

// NoopBillingClient allows all provisioning without billing checks (for dev/testing).
type NoopBillingClient struct{}

func (n *NoopBillingClient) AuthorizeProvisioning(_ context.Context, _ string, platform Platform, _ string) (*BillingAuth, error) {
	rate := platformHourlyCents(platform)
	return &BillingAuth{
		Authorized:  true,
		Tier:        "unlimited",
		HourlyCents: rate,
	}, nil
}

func (n *NoopBillingClient) ReportUsage(_ context.Context, _ string, _ Platform, _ float64, _ int) error {
	return nil
}

func (n *NoopBillingClient) GetTeamQuota(_ context.Context, _ string) (*CloudQuota, error) {
	return &CloudQuota{
		Tier:               "unlimited",
		MaxLinuxInstances:  100,
		MaxWindowsInstances: 10,
		MaxMacOSInstances:  5,
		MaxComputeHours:    0, // unlimited
	}, nil
}

// platformHourlyCents returns the default hourly rate in cents for a platform.
func platformHourlyCents(p Platform) int {
	switch p {
	case PlatformMacOS:
		return 120 // $1.20/hr mac2.metal
	case PlatformWindows:
		return 10 // $0.10/hr t3.large
	case PlatformLinux:
		return 1 // $0.01/hr pod
	default:
		return 10
	}
}
