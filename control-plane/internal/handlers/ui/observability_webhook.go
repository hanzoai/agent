package ui

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/hanzoai/agents/control-plane/internal/services"
	"github.com/hanzoai/agents/control-plane/internal/storage"
	"github.com/hanzoai/agents/control-plane/pkg/types"
	"github.com/gin-gonic/gin"
)

// ObservabilityWebhookHandler provides handlers for observability webhook management.
type ObservabilityWebhookHandler struct {
	storage   storage.StorageProvider
	forwarder services.ObservabilityForwarder
}

// NewObservabilityWebhookHandler creates a new ObservabilityWebhookHandler.
func NewObservabilityWebhookHandler(storage storage.StorageProvider, forwarder services.ObservabilityForwarder) *ObservabilityWebhookHandler {
	return &ObservabilityWebhookHandler{
		storage:   storage,
		forwarder: forwarder,
	}
}

// GetWebhookHandler retrieves the current observability webhook configuration.
// GET /api/v1/settings/observability-webhook
func (h *ObservabilityWebhookHandler) GetWebhookHandler(c *gin.Context) {
	ctx := c.Request.Context()

	config, err := h.storage.GetObservabilityWebhook(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to get observability webhook config"})
		return
	}

	response := types.ObservabilityWebhookConfigResponse{
		Configured: config != nil,
	}

	if config != nil {
		// Create a copy without the secret for the response
		configResponse := &types.ObservabilityWebhookConfig{
			ID:        config.ID,
			URL:       config.URL,
			HasSecret: config.Secret != nil && *config.Secret != "",
			Headers:   config.Headers,
			Enabled:   config.Enabled,
			CreatedAt: config.CreatedAt,
			UpdatedAt: config.UpdatedAt,
		}
		response.Config = configResponse
	}

	c.JSON(http.StatusOK, response)
}

// SetWebhookHandler creates or updates the observability webhook configuration.
// POST /api/v1/settings/observability-webhook
func (h *ObservabilityWebhookHandler) SetWebhookHandler(c *gin.Context) {
	ctx := c.Request.Context()

	var req types.ObservabilityWebhookConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	// Validate URL
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "url is required"})
		return
	}

	parsedURL, err := url.Parse(req.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid url: must be http or https"})
		return
	}

	// Build config
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Check if updating existing
	existing, _ := h.storage.GetObservabilityWebhook(ctx)

	secret := req.Secret
	if secret != nil && *secret == "" {
		secret = nil
	}
	if secret == nil && existing != nil {
		secret = existing.Secret
	}

	config := &types.ObservabilityWebhookConfig{
		ID:        "global",
		URL:       req.URL,
		Secret:    secret,
		Headers:   req.Headers,
		Enabled:   enabled,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if existing != nil {
		config.CreatedAt = existing.CreatedAt
	}

	// Store config
	if err := h.storage.SetObservabilityWebhook(ctx, config); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to save observability webhook config"})
		return
	}

	// Reload forwarder config
	if h.forwarder != nil {
		if err := h.forwarder.ReloadConfig(ctx); err != nil {
			// Log but don't fail - config is saved
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "observability webhook configured successfully (forwarder reload pending)",
				"config": types.ObservabilityWebhookConfig{
					ID:        config.ID,
					URL:       config.URL,
					Headers:   config.Headers,
					Enabled:   config.Enabled,
					CreatedAt: config.CreatedAt,
					UpdatedAt: config.UpdatedAt,
				},
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "observability webhook configured successfully",
		"config": types.ObservabilityWebhookConfig{
			ID:        config.ID,
			URL:       config.URL,
			Headers:   config.Headers,
			Enabled:   config.Enabled,
			CreatedAt: config.CreatedAt,
			UpdatedAt: config.UpdatedAt,
		},
	})
}

// DeleteWebhookHandler removes the observability webhook configuration.
// DELETE /api/v1/settings/observability-webhook
func (h *ObservabilityWebhookHandler) DeleteWebhookHandler(c *gin.Context) {
	ctx := c.Request.Context()

	if err := h.storage.DeleteObservabilityWebhook(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to delete observability webhook config"})
		return
	}

	// Reload forwarder config to disable forwarding
	if h.forwarder != nil {
		_ = h.forwarder.ReloadConfig(ctx) // Best effort
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "observability webhook configuration removed",
	})
}

// GetStatusHandler retrieves the current observability forwarder status.
// GET /api/v1/settings/observability-webhook/status
func (h *ObservabilityWebhookHandler) GetStatusHandler(c *gin.Context) {
	if h.forwarder == nil {
		c.JSON(http.StatusOK, types.ObservabilityForwarderStatus{
			Enabled: false,
		})
		return
	}

	status := h.forwarder.GetStatus()
	c.JSON(http.StatusOK, status)
}

// RedriveHandler attempts to resend all events in the dead letter queue.
// POST /api/v1/settings/observability-webhook/redrive
func (h *ObservabilityWebhookHandler) RedriveHandler(c *gin.Context) {
	if h.forwarder == nil {
		c.JSON(http.StatusServiceUnavailable, types.ObservabilityRedriveResponse{
			Success: false,
			Message: "forwarder not available",
		})
		return
	}

	ctx := c.Request.Context()
	response := h.forwarder.Redrive(ctx)

	if response.Success {
		c.JSON(http.StatusOK, response)
	} else {
		c.JSON(http.StatusOK, response) // Still 200 as the operation completed, just with failures
	}
}

// GetDeadLetterQueueHandler retrieves entries from the dead letter queue.
// GET /api/v1/settings/observability-webhook/dlq
func (h *ObservabilityWebhookHandler) GetDeadLetterQueueHandler(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse query params
	limit := 100
	offset := 0
	if l := c.Query("limit"); l != "" {
		if parsed, err := parseIntParam(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := parseIntParam(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	entries, err := h.storage.GetDeadLetterQueue(ctx, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to get dead letter queue"})
		return
	}

	count, err := h.storage.GetDeadLetterQueueCount(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to get dead letter queue count"})
		return
	}

	c.JSON(http.StatusOK, types.ObservabilityDeadLetterListResponse{
		Entries:    entries,
		TotalCount: count,
	})
}

// ClearDeadLetterQueueHandler clears all entries from the dead letter queue.
// DELETE /api/v1/settings/observability-webhook/dlq
func (h *ObservabilityWebhookHandler) ClearDeadLetterQueueHandler(c *gin.Context) {
	ctx := c.Request.Context()

	if err := h.storage.ClearDeadLetterQueue(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to clear dead letter queue"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "dead letter queue cleared",
	})
}

// ConsolePresetRequest is the request for auto-configuring Console as the observability provider.
type ConsolePresetRequest struct {
	PublicKey string `json:"public_key" binding:"required"`
	SecretKey string `json:"secret_key" binding:"required"`
	Host      string `json:"host,omitempty"` // Defaults to https://console.hanzo.ai
}

// SetConsolePresetHandler auto-configures observability webhook for Console.
// POST /api/v1/settings/observability-webhook/presets/console
func (h *ObservabilityWebhookHandler) SetConsolePresetHandler(c *gin.Context) {
	ctx := c.Request.Context()

	var req ConsolePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	host := req.Host
	if host == "" {
		host = "https://console.hanzo.ai"
	}

	// Configure webhook to point at Console ingestion API
	webhookURL := fmt.Sprintf("%s/api/public/ingestion", host)

	config := &types.ObservabilityWebhookConfig{
		ID:  "global",
		URL: webhookURL,
		Headers: map[string]string{
			"X-Console-Public-Key": req.PublicKey,
			"Authorization":         "Basic " + basicAuth(req.PublicKey, req.SecretKey),
			"Content-Type":          "application/json",
		},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	existing, _ := h.storage.GetObservabilityWebhook(ctx)
	if existing != nil {
		config.CreatedAt = existing.CreatedAt
	}

	if err := h.storage.SetObservabilityWebhook(ctx, config); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to save Console config"})
		return
	}

	if h.forwarder != nil {
		_ = h.forwarder.ReloadConfig(ctx)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Console observability configured successfully",
		"provider": "console",
		"host":     host,
	})
}

func basicAuth(publicKey, secretKey string) string {
	return base64.StdEncoding.EncodeToString([]byte(publicKey + ":" + secretKey))
}

func parseIntParam(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
