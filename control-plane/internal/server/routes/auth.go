package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hanzoai/agents/control-plane/internal/config"
	"github.com/hanzoai/agents/control-plane/internal/server/middleware"
)

// IAMTokenResponse represents the response from IAM token endpoint.
type IAMTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
}

// AuthInfoResponse is returned by /api/v1/auth/userinfo and /api/v1/auth/info.
type AuthInfoResponse struct {
	Authenticated bool                  `json:"authenticated"`
	Method        string                `json:"method,omitempty"`    // "iam" or "api_key"
	IAMEnabled    bool                  `json:"iam_enabled"`
	User          *middleware.IAMUserInfo `json:"user,omitempty"`
}

// RegisterAuthRoutes registers OAuth/IAM authentication routes on the router.
// These routes must be registered BEFORE the auth middleware is applied.
func RegisterAuthRoutes(router *gin.Engine, authCfg config.AuthConfig) {
	if !authCfg.IAMEnabled {
		// Even if IAM is disabled, register the info endpoint so the frontend
		// can discover that IAM is not available.
		router.GET("/api/v1/auth/info", func(c *gin.Context) {
			c.JSON(http.StatusOK, AuthInfoResponse{
				Authenticated: false,
				IAMEnabled:    false,
			})
		})
		return
	}

	publicEndpoint := strings.TrimRight(authCfg.IAMPublicEndpoint, "/")
	internalEndpoint := strings.TrimRight(authCfg.IAMEndpoint, "/")
	if internalEndpoint == "" {
		internalEndpoint = publicEndpoint
	}

	// GET /auth/login - Redirect to IAM authorization page.
	router.GET("/auth/login", func(c *gin.Context) {
		// Build the Casdoor authorize URL.
		redirectURI := buildRedirectURI(c, authCfg)
		state := fmt.Sprintf("%d", time.Now().UnixNano()) // Simple state for CSRF protection

		authorizeURL := fmt.Sprintf(
			"%s/login/oauth/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&state=%s",
			publicEndpoint,
			url.QueryEscape(authCfg.IAMClientID),
			url.QueryEscape(redirectURI),
			url.QueryEscape("openid profile email"),
			url.QueryEscape(state),
		)

		c.Redirect(http.StatusFound, authorizeURL)
	})

	// GET /auth/callback - Exchange authorization code for tokens.
	router.GET("/auth/callback", func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "missing_code",
				"message": "authorization code not provided",
			})
			return
		}

		// Exchange code for tokens using the internal IAM endpoint.
		tokenURL := internalEndpoint + "/api/login/oauth/access_token"
		redirectURI := buildRedirectURI(c, authCfg)

		formData := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {code},
			"redirect_uri":  {redirectURI},
			"client_id":     {authCfg.IAMClientID},
			"client_secret": {authCfg.IAMClientSecret},
		}

		httpClient := &http.Client{Timeout: 10 * time.Second}
		resp, err := httpClient.PostForm(tokenURL, formData)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "token_exchange_failed",
				"message": fmt.Sprintf("failed to contact IAM: %v", err),
			})
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "token_read_failed",
				"message": "failed to read IAM response",
			})
			return
		}

		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "token_exchange_failed",
				"message": fmt.Sprintf("IAM returned status %d: %s", resp.StatusCode, string(body)),
			})
			return
		}

		var tokenResp IAMTokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "token_parse_failed",
				"message": "failed to parse IAM token response",
			})
			return
		}

		if tokenResp.AccessToken == "" {
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "no_access_token",
				"message": "IAM did not return an access token",
			})
			return
		}

		// Set the session cookie with the access token.
		maxAge := tokenResp.ExpiresIn
		if maxAge <= 0 {
			maxAge = 3600 // Default 1 hour
		}

		// Determine if we should set Secure flag based on the request.
		secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"

		c.SetCookie(
			middleware.SessionCookieName,
			tokenResp.AccessToken,
			maxAge,
			"/",
			"",   // Domain - let browser infer
			secure,
			true, // HttpOnly
		)

		// Redirect to the UI.
		c.Redirect(http.StatusFound, "/ui/")
	})

	// GET /api/v1/auth/userinfo - Return current user info from IAM token.
	router.GET("/api/v1/auth/userinfo", func(c *gin.Context) {
		// Try to get token from Authorization header or session cookie.
		token := ""
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
		if token == "" {
			if cookie, err := c.Cookie(middleware.SessionCookieName); err == nil {
				token = cookie
			}
		}

		// Also check API key auth.
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		if token == "" && apiKey == "" {
			c.JSON(http.StatusUnauthorized, AuthInfoResponse{
				Authenticated: false,
				IAMEnabled:    true,
			})
			return
		}

		// If we have an API key and it matches, return authenticated without user info.
		if apiKey != "" && apiKey == authCfg.APIKey {
			c.JSON(http.StatusOK, AuthInfoResponse{
				Authenticated: true,
				Method:        "api_key",
				IAMEnabled:    true,
			})
			return
		}

		// Validate IAM token.
		if token != "" {
			httpClient := &http.Client{Timeout: 5 * time.Second}
			userinfoURL := internalEndpoint + "/api/userinfo"

			req, err := http.NewRequest("GET", userinfoURL, nil)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
				return
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := httpClient.Do(req)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{
					"error":   "iam_unreachable",
					"message": "could not reach IAM service",
				})
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var userInfo middleware.IAMUserInfo
				if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil && (userInfo.ID != "" || userInfo.Email != "") {
					c.JSON(http.StatusOK, AuthInfoResponse{
						Authenticated: true,
						Method:        "iam",
						IAMEnabled:    true,
						User:          &userInfo,
					})
					return
				}
			}
		}

		// Token is invalid.
		c.JSON(http.StatusUnauthorized, AuthInfoResponse{
			Authenticated: false,
			IAMEnabled:    true,
		})
	})

	// GET /api/v1/auth/info - Return auth configuration (no auth required).
	router.GET("/api/v1/auth/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"iam_enabled": true,
			"login_url":   "/auth/login",
		})
	})

	// POST /auth/logout - Clear session cookie.
	router.POST("/auth/logout", func(c *gin.Context) {
		secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
		c.SetCookie(
			middleware.SessionCookieName,
			"",
			-1,   // Expire immediately
			"/",
			"",
			secure,
			true,
		)
		c.JSON(http.StatusOK, gin.H{"message": "logged out"})
	})
}

// buildRedirectURI constructs the OAuth redirect URI from the current request.
func buildRedirectURI(c *gin.Context, authCfg config.AuthConfig) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := c.Request.Host
	return fmt.Sprintf("%s://%s/auth/callback", scheme, host)
}
