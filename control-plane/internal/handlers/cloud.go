package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/agents/control-plane/internal/cloud"
	"github.com/hanzoai/agents/control-plane/pkg/types"
)

// CloudHandlers holds dependencies for cloud API handlers.
type CloudHandlers struct {
	manager *cloud.CloudManager
}

// NewCloudHandlers creates a new CloudHandlers instance.
func NewCloudHandlers(manager *cloud.CloudManager) *CloudHandlers {
	return &CloudHandlers{manager: manager}
}

// CreateInstanceHandler handles POST /api/v1/cloud/instances
func (h *CloudHandlers) CreateInstanceHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		var req types.ProvisionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
			return
		}

		inst, err := h.manager.CreateInstance(ctx, &req)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, cloud.ErrCloudDisabled):
				status = http.StatusServiceUnavailable
			case errors.Is(err, cloud.ErrMaxInstancesReached):
				status = http.StatusTooManyRequests
			case errors.Is(err, cloud.ErrNoAvailableHost):
				status = http.StatusServiceUnavailable
			case errors.Is(err, cloud.ErrInvalidPlatform):
				status = http.StatusBadRequest
			case errors.Is(err, cloud.ErrBillingNotAuthorized):
				status = http.StatusPaymentRequired
			case errors.Is(err, cloud.ErrBillingQuotaExceeded):
				status = http.StatusPaymentRequired
			case errors.Is(err, cloud.ErrBillingServiceUnavailable):
				status = http.StatusServiceUnavailable
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		log.Info().Str("id", inst.ID).Str("platform", string(inst.Platform)).Msg("cloud instance created")
		c.JSON(http.StatusCreated, inst)
	}
}

// ListInstancesHandler handles GET /api/v1/cloud/instances
func (h *CloudHandlers) ListInstancesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		filters := types.InstanceFilters{}
		if v := c.Query("platform"); v != "" {
			p := types.Platform(v)
			filters.Platform = &p
		}
		if v := c.Query("state"); v != "" {
			s := types.InstanceState(v)
			filters.State = &s
		}
		if v := c.Query("team_id"); v != "" {
			filters.TeamID = &v
		}
		if v := c.Query("provider"); v != "" {
			filters.Provider = &v
		}
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				filters.Limit = n
			}
		}
		if v := c.Query("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				filters.Offset = n
			}
		}

		instances, err := h.manager.ListInstances(ctx, filters)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"instances": instances,
			"count":     len(instances),
			"filters":   filters,
		})
	}
}

// GetInstanceHandler handles GET /api/v1/cloud/instances/:id
func (h *CloudHandlers) GetInstanceHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		inst, err := h.manager.GetInstance(ctx, id)
		if err != nil {
			if err == cloud.ErrInstanceNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, inst)
	}
}

// TerminateInstanceHandler handles DELETE /api/v1/cloud/instances/:id
func (h *CloudHandlers) TerminateInstanceHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		if err := h.manager.TerminateInstance(ctx, id); err != nil {
			if err == cloud.ErrInstanceNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "instance terminated"})
	}
}

// StartInstanceHandler handles POST /api/v1/cloud/instances/:id/start
func (h *CloudHandlers) StartInstanceHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		if err := h.manager.StartInstance(ctx, id); err != nil {
			if err == cloud.ErrInstanceNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "instance started"})
	}
}

// StopInstanceHandler handles POST /api/v1/cloud/instances/:id/stop
func (h *CloudHandlers) StopInstanceHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		if err := h.manager.StopInstance(ctx, id); err != nil {
			if err == cloud.ErrInstanceNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "instance stopped"})
	}
}

// GetConnectionInfoHandler handles GET /api/v1/cloud/instances/:id/connect
func (h *CloudHandlers) GetConnectionInfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		info, err := h.manager.GetConnectionInfo(ctx, id)
		if err != nil {
			if err == cloud.ErrInstanceNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, info)
	}
}

// GetLogsHandler handles GET /api/v1/cloud/instances/:id/logs
func (h *CloudHandlers) GetLogsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		lines := 100
		if v := c.Query("lines"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				lines = n
			}
		}

		logs, err := h.manager.GetLogs(ctx, id, lines)
		if err != nil {
			if err == cloud.ErrInstanceNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"instance_id": id,
			"lines":       lines,
			"logs":        logs,
		})
	}
}

// ExecuteCommandHandler handles POST /api/v1/cloud/instances/:id/exec
func (h *CloudHandlers) ExecuteCommandHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		var req struct {
			Command string `json:"command" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
			return
		}

		result, err := h.manager.ExecuteCommand(ctx, id, req.Command)
		if err != nil {
			if err == cloud.ErrInstanceNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// GetQuotaHandler handles GET /api/v1/cloud/quota
func (h *CloudHandlers) GetQuotaHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		teamID := c.Query("team_id")
		if teamID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "team_id is required"})
			return
		}

		quota, err := h.manager.Billing().GetTeamQuota(ctx, teamID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, quota)
	}
}
