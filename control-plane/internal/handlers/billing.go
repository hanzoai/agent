package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hanzoai/agents/control-plane/internal/services"
)

// BillingBalanceHandler proxies balance queries to Commerce, scoped to
// the authenticated user. The control-plane adds the admin token so
// the frontend never needs direct Commerce credentials.
//
//	GET /api/v1/billing/balance?user=<iam-user-id>&currency=usd
func BillingBalanceHandler(billing *services.BillingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if billing == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "billing not configured"})
			return
		}

		userID := c.Query("user")
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user query parameter is required"})
			return
		}

		available, err := billing.CheckBalance(c.Request.Context(), userID)
		if err != nil {
			if err == services.ErrCommerceUnavailable {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "billing service unavailable"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"available": available,
			"currency":  "usd",
		})
	}
}
