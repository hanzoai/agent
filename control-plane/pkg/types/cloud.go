package types

import (
	"encoding/json"
	"time"
)

// Platform represents the operating system platform for a cloud instance.
type Platform string

const (
	PlatformLinux   Platform = "linux"
	PlatformMacOS   Platform = "macos"
	PlatformWindows Platform = "windows"
)

// InstanceState represents the lifecycle state of a cloud instance.
type InstanceState string

const (
	InstanceStateRequested    InstanceState = "requested"
	InstanceStateProvisioning InstanceState = "provisioning"
	InstanceStateRunning      InstanceState = "running"
	InstanceStateStopped      InstanceState = "stopped"
	InstanceStateTerminated   InstanceState = "terminated"
	InstanceStateFailed       InstanceState = "failed"
)

// ConnectionProtocol represents the protocol used to connect to an instance.
type ConnectionProtocol string

const (
	ConnectionProtocolRDP  ConnectionProtocol = "rdp"
	ConnectionProtocolVNC  ConnectionProtocol = "vnc"
	ConnectionProtocolSSH  ConnectionProtocol = "ssh"
	ConnectionProtocolExec ConnectionProtocol = "exec"
	ConnectionProtocolSSM  ConnectionProtocol = "ssm"
)

// CloudInstance represents a provisioned cloud instance.
type CloudInstance struct {
	ID         string        `json:"id"`
	Platform   Platform      `json:"platform"`
	State      InstanceState `json:"state"`
	Provider   string        `json:"provider"` // "k8s" or "aws"
	InstanceID string        `json:"instance_id"`

	// Instance details
	InstanceType string `json:"instance_type,omitempty"`
	ImageID      string `json:"image_id,omitempty"`
	Region       string `json:"region,omitempty"`

	// Bot configuration
	BotPackage string `json:"bot_package"`
	BotVersion string `json:"bot_version,omitempty"`

	// Network
	PublicIP  string `json:"public_ip,omitempty"`
	PrivateIP string `json:"private_ip,omitempty"`

	// Agent correlation
	AgentNodeID string `json:"agent_node_id,omitempty"`
	TeamID      string `json:"team_id"`

	// Resource tracking
	DedicatedHostID string `json:"dedicated_host_id,omitempty"`

	// Billing
	HourlyRateCents int    `json:"hourly_rate_cents,omitempty"`
	AccruedCostCents int   `json:"accrued_cost_cents,omitempty"`
	BillingTier     string `json:"billing_tier,omitempty"`

	// Connection
	ConnectionInfo *ConnectionInfo `json:"connection_info,omitempty"`

	// Metadata
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`

	// Error tracking
	ErrorMessage string `json:"error_message,omitempty"`

	// Timestamps
	RequestedAt   time.Time  `json:"requested_at"`
	ProvisionedAt *time.Time `json:"provisioned_at,omitempty"`
	TerminatedAt  *time.Time `json:"terminated_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ConnectionInfo holds the connection details for a cloud instance.
type ConnectionInfo struct {
	Protocol ConnectionProtocol `json:"protocol"`
	Host     string             `json:"host"`
	Port     int                `json:"port"`
	Username string             `json:"username,omitempty"`
	Password string             `json:"password,omitempty"`
	KeyData  string             `json:"key_data,omitempty"`
	Extra    map[string]string  `json:"extra,omitempty"`
}

// ProvisionRequest represents a request to provision a new cloud instance.
type ProvisionRequest struct {
	Platform     Platform          `json:"platform" binding:"required"`
	BotPackage   string            `json:"bot_package" binding:"required"`
	BotVersion   string            `json:"bot_version,omitempty"`
	InstanceType string            `json:"instance_type,omitempty"`
	TeamID       string            `json:"team_id" binding:"required"`
	Tags         map[string]string `json:"tags,omitempty"`
	Metadata     json.RawMessage   `json:"metadata,omitempty"`
}

// CommandResult represents the result of a command executed on an instance.
type CommandResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// DedicatedHost represents an AWS Dedicated Host for macOS instances.
type DedicatedHost struct {
	ID             string        `json:"id"`
	HostID         string        `json:"host_id"`
	InstanceType   string        `json:"instance_type"`
	State          string        `json:"state"` // "available", "allocated", "released"
	CurrentInstanceID string     `json:"current_instance_id,omitempty"`
	AllocatedAt    *time.Time    `json:"allocated_at,omitempty"`
	ReleasedAt     *time.Time    `json:"released_at,omitempty"`
	MinAllocation  time.Duration `json:"min_allocation"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// InstanceFilters holds filters for querying cloud instances.
type InstanceFilters struct {
	Platform *Platform      `json:"platform,omitempty"`
	State    *InstanceState `json:"state,omitempty"`
	TeamID   *string        `json:"team_id,omitempty"`
	Provider *string        `json:"provider,omitempty"`
	Limit    int            `json:"limit,omitempty"`
	Offset   int            `json:"offset,omitempty"`
}

// CloudEvent represents a cloud infrastructure event.
type CloudEvent struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"` // "instance.created", "instance.running", etc.
	InstanceID string          `json:"instance_id"`
	Timestamp  time.Time       `json:"timestamp"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// CloudSummary holds dashboard summary data.
type CloudSummary struct {
	TotalInstances    int                        `json:"total_instances"`
	ByPlatform        map[Platform]int           `json:"by_platform"`
	ByState           map[InstanceState]int      `json:"by_state"`
	ActiveHosts       int                        `json:"active_hosts"`
	EstimatedCostUSD  float64                    `json:"estimated_cost_usd"`
	TotalAccruedCents int                        `json:"total_accrued_cents"`
}
