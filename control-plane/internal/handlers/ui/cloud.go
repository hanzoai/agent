package ui

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/agents/control-plane/internal/cloud"
	"github.com/hanzoai/agents/control-plane/pkg/types"
)

// CloudHandler provides UI-friendly cloud instance endpoints.
type CloudHandler struct {
	manager *cloud.CloudManager
}

// NewCloudHandler creates a new CloudHandler.
func NewCloudHandler(manager *cloud.CloudManager) *CloudHandler {
	return &CloudHandler{manager: manager}
}

// ListInstancesHandler handles GET /api/ui/v1/cloud/instances
func (h *CloudHandler) ListInstancesHandler(c *gin.Context) {
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

	// Build UI-friendly response.
	items := make([]gin.H, 0, len(instances))
	for _, inst := range instances {
		item := gin.H{
			"id":            inst.ID,
			"platform":      inst.Platform,
			"state":         inst.State,
			"provider":      inst.Provider,
			"instance_type": inst.InstanceType,
			"bot_package":   inst.BotPackage,
			"bot_version":   inst.BotVersion,
			"team_id":       inst.TeamID,
			"public_ip":     inst.PublicIP,
			"private_ip":    inst.PrivateIP,
			"agent_node_id": inst.AgentNodeID,
			"created_at":    inst.CreatedAt,
			"updated_at":    inst.UpdatedAt,
		}
		if inst.ProvisionedAt != nil {
			item["provisioned_at"] = inst.ProvisionedAt
		}
		if inst.ErrorMessage != "" {
			item["error_message"] = inst.ErrorMessage
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"instances": items,
		"count":     len(items),
	})
}

// GetInstanceDetailsHandler handles GET /api/ui/v1/cloud/instances/:id/details
func (h *CloudHandler) GetInstanceDetailsHandler(c *gin.Context) {
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

	// Include connection info if running.
	var connInfo *types.ConnectionInfo
	if inst.State == types.InstanceStateRunning {
		connInfo, _ = h.manager.GetConnectionInfo(ctx, id)
	}

	c.JSON(http.StatusOK, gin.H{
		"instance":        inst,
		"connection_info": connInfo,
	})
}

// GetSummaryHandler handles GET /api/ui/v1/cloud/summary
func (h *CloudHandler) GetSummaryHandler(c *gin.Context) {
	ctx := c.Request.Context()

	summary, err := h.manager.GetSummary(ctx)
	if err != nil {
		if err == cloud.ErrCloudDisabled {
			c.JSON(http.StatusOK, gin.H{
				"enabled": false,
				"message": "cloud provisioning is disabled",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled": true,
		"summary": summary,
	})
}

// StreamEventsHandler handles GET /api/ui/v1/cloud/events (SSE)
func (h *CloudHandler) StreamEventsHandler(c *gin.Context) {
	eventBus := h.manager.EventBus()

	subID, ch := eventBus.Subscribe()
	defer eventBus.Unsubscribe(subID)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Send recent events as initial batch.
	recent := eventBus.Recent(20)
	for _, event := range recent {
		writeSSEEvent(c.Writer, event)
	}
	c.Writer.Flush()

	// Stream new events.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			writeSSEEvent(c.Writer, event)
			c.Writer.Flush()
		case <-ticker.C:
			// Keep-alive ping.
			fmt.Fprintf(c.Writer, ": ping\n\n")
			c.Writer.Flush()
		}
	}
}

func writeSSEEvent(w io.Writer, event types.CloudEvent) {
	fmt.Fprintf(w, "event: %s\n", event.Type)
	fmt.Fprintf(w, "id: %s\n", event.ID)
	if event.Data != nil {
		fmt.Fprintf(w, "data: %s\n", string(event.Data))
	} else {
		fmt.Fprintf(w, "data: {\"instance_id\":\"%s\"}\n", event.InstanceID)
	}
	fmt.Fprintf(w, "\n")

	log.Debug().Str("event", event.Type).Str("instance", event.InstanceID).Msg("SSE cloud event sent")
}
