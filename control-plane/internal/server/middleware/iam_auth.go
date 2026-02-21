package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// IAMConfig holds IAM authentication configuration for the middleware.
type IAMConfig struct {
	Enabled        bool
	Endpoint       string // Internal IAM endpoint for server-to-server calls
	PublicEndpoint string // Public IAM endpoint for browser redirects
	ClientID       string
	ClientSecret   string
	Organization   string
	Application    string
}

// IAMUserInfo represents the user identity returned by IAM userinfo endpoint.
type IAMUserInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Email        string `json:"email"`
	Avatar       string `json:"avatar"`
	Owner        string `json:"owner"` // organization
	Type         string `json:"type"`
	IsAdmin      bool   `json:"isAdmin"`
	IsGlobalAdmin bool  `json:"isGlobalAdmin"`
}

// tokenCacheEntry stores a validated token with its expiry time.
type tokenCacheEntry struct {
	user      *IAMUserInfo
	expiresAt time.Time
}

const (
	// tokenCacheTTL is how long validated tokens are cached.
	tokenCacheTTL = 60 * time.Second

	// sessionCookieName is the cookie used for browser-based OAuth sessions.
	SessionCookieName = "hanzo_agents_session"

	// Gin context keys for IAM user info.
	ContextKeyIAMUser     = "iam_user"
	ContextKeyIAMUserID   = "iam_user_id"
	ContextKeyIAMUserName = "iam_user_name"
	ContextKeyIAMEmail    = "iam_user_email"
	ContextKeyIAMOrg      = "iam_user_org"
	ContextKeyAuthMethod  = "auth_method"
)

// tokenCache is a concurrent-safe cache for validated IAM tokens.
var tokenCache sync.Map

// IAMAuth validates requests using IAM Bearer tokens or session cookies.
// If IAM validation succeeds, user identity is set in gin context.
// If IAM validation fails, the middleware does NOT abort - it allows the
// next middleware (API key auth) to try.
func IAMAuth(config IAMConfig) gin.HandlerFunc {
	client := &http.Client{Timeout: 5 * time.Second}

	return func(c *gin.Context) {
		if !config.Enabled {
			c.Next()
			return
		}

		// Extract token: try Authorization header first, then session cookie.
		token := extractBearerToken(c)
		if token == "" {
			token = extractSessionCookie(c)
		}

		if token == "" {
			// No IAM token found, let next middleware handle auth.
			c.Next()
			return
		}

		// Check token cache first.
		if entry, ok := tokenCache.Load(token); ok {
			cached := entry.(*tokenCacheEntry)
			if time.Now().Before(cached.expiresAt) {
				setIAMUserContext(c, cached.user)
				c.Next()
				return
			}
			// Expired entry, remove it.
			tokenCache.Delete(token)
		}

		// Validate token against IAM userinfo endpoint.
		userInfo, err := validateTokenWithIAM(client, config.Endpoint, token)
		if err != nil {
			// IAM validation failed. Do NOT abort - let API key middleware try.
			c.Next()
			return
		}

		// Cache the validated token.
		tokenCache.Store(token, &tokenCacheEntry{
			user:      userInfo,
			expiresAt: time.Now().Add(tokenCacheTTL),
		})

		setIAMUserContext(c, userInfo)
		c.Next()
	}
}

// extractBearerToken extracts a Bearer token from the Authorization header.
func extractBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

// extractSessionCookie extracts the access token from the session cookie.
func extractSessionCookie(c *gin.Context) string {
	cookie, err := c.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie
}

// validateTokenWithIAM calls the IAM userinfo endpoint to validate a token.
func validateTokenWithIAM(client *http.Client, iamEndpoint, token string) (*IAMUserInfo, error) {
	endpoint := strings.TrimRight(iamEndpoint, "/") + "/api/userinfo"

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &IAMValidationError{StatusCode: resp.StatusCode}
	}

	var userInfo IAMUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	// Require at minimum an ID or email to consider the token valid.
	if userInfo.ID == "" && userInfo.Email == "" {
		return nil, &IAMValidationError{StatusCode: http.StatusUnauthorized}
	}

	return &userInfo, nil
}

// setIAMUserContext sets the validated IAM user info into the gin context.
func setIAMUserContext(c *gin.Context, user *IAMUserInfo) {
	c.Set(ContextKeyIAMUser, user)
	c.Set(ContextKeyIAMUserID, user.ID)
	c.Set(ContextKeyIAMUserName, user.Name)
	c.Set(ContextKeyIAMEmail, user.Email)
	c.Set(ContextKeyIAMOrg, user.Owner)
	c.Set(ContextKeyAuthMethod, "iam")
}

// GetIAMUser retrieves the IAM user from the gin context.
// Returns nil if no IAM user is set (e.g., API key auth was used).
func GetIAMUser(c *gin.Context) *IAMUserInfo {
	val, exists := c.Get(ContextKeyIAMUser)
	if !exists {
		return nil
	}
	user, ok := val.(*IAMUserInfo)
	if !ok {
		return nil
	}
	return user
}

// IsIAMAuthenticated returns true if the request was authenticated via IAM.
func IsIAMAuthenticated(c *gin.Context) bool {
	method, exists := c.Get(ContextKeyAuthMethod)
	return exists && method == "iam"
}

// IAMValidationError represents an error from IAM token validation.
type IAMValidationError struct {
	StatusCode int
}

func (e *IAMValidationError) Error() string {
	return "IAM token validation failed"
}

// CombinedAuth creates a middleware that tries IAM auth first, then falls back
// to API key authentication. A request must pass at least one method.
func CombinedAuth(iamConfig IAMConfig, apiKeyConfig AuthConfig) gin.HandlerFunc {
	skipPathSet := make(map[string]struct{}, len(apiKeyConfig.SkipPaths))
	for _, p := range apiKeyConfig.SkipPaths {
		skipPathSet[p] = struct{}{}
	}

	client := &http.Client{Timeout: 5 * time.Second}

	return func(c *gin.Context) {
		// No auth configured at all, allow everything.
		if !iamConfig.Enabled && apiKeyConfig.APIKey == "" {
			c.Next()
			return
		}

		// Skip explicit paths.
		if _, ok := skipPathSet[c.Request.URL.Path]; ok {
			c.Next()
			return
		}

		// Always allow health, metrics, auth routes, and UI static files.
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/v1/health") ||
			path == "/health" ||
			path == "/metrics" ||
			strings.HasPrefix(path, "/auth/") ||
			strings.HasPrefix(path, "/ui") ||
			path == "/" {
			c.Next()
			return
		}

		// --- Try IAM auth first ---
		if iamConfig.Enabled {
			token := extractBearerToken(c)
			if token == "" {
				token = extractSessionCookie(c)
			}

			if token != "" {
				// Check cache.
				if entry, ok := tokenCache.Load(token); ok {
					cached := entry.(*tokenCacheEntry)
					if time.Now().Before(cached.expiresAt) {
						setIAMUserContext(c, cached.user)
						c.Next()
						return
					}
					tokenCache.Delete(token)
				}

				// Validate against IAM.
				userInfo, err := validateTokenWithIAM(client, iamConfig.Endpoint, token)
				if err == nil {
					tokenCache.Store(token, &tokenCacheEntry{
						user:      userInfo,
						expiresAt: time.Now().Add(tokenCacheTTL),
					})
					setIAMUserContext(c, userInfo)
					c.Next()
					return
				}
			}
		}

		// --- Fall back to API key auth ---
		if apiKeyConfig.APIKey == "" {
			// No API key configured and IAM didn't authenticate.
			// If IAM is enabled but no token was provided, require auth.
			if iamConfig.Enabled {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error":       "unauthorized",
					"message":     "authentication required",
					"iam_enabled": true,
				})
				return
			}
			// Neither IAM nor API key configured - allow through.
			c.Next()
			return
		}

		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				apiKey = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		if apiKey == apiKeyConfig.APIKey {
			c.Set(ContextKeyAuthMethod, "api_key")
			c.Next()
			return
		}

		// Both methods failed.
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":       "unauthorized",
			"message":     "invalid or missing credentials",
			"iam_enabled": iamConfig.Enabled,
		})
	}
}
