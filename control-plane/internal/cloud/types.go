package cloud

import (
	"github.com/hanzoai/agents/control-plane/pkg/types"
)

// Re-export public types for internal use.
type (
	CloudInstance      = types.CloudInstance
	ProvisionRequest   = types.ProvisionRequest
	ConnectionInfo     = types.ConnectionInfo
	CommandResult      = types.CommandResult
	DedicatedHost      = types.DedicatedHost
	InstanceFilters    = types.InstanceFilters
	CloudEvent         = types.CloudEvent
	CloudSummary       = types.CloudSummary
	Platform           = types.Platform
	InstanceState      = types.InstanceState
	ConnectionProtocol = types.ConnectionProtocol
)

// Re-export constants.
const (
	PlatformLinux   = types.PlatformLinux
	PlatformMacOS   = types.PlatformMacOS
	PlatformWindows = types.PlatformWindows

	InstanceStateRequested    = types.InstanceStateRequested
	InstanceStateProvisioning = types.InstanceStateProvisioning
	InstanceStateRunning      = types.InstanceStateRunning
	InstanceStateStopped      = types.InstanceStateStopped
	InstanceStateTerminated   = types.InstanceStateTerminated
	InstanceStateFailed       = types.InstanceStateFailed

	ConnectionProtocolRDP  = types.ConnectionProtocolRDP
	ConnectionProtocolVNC  = types.ConnectionProtocolVNC
	ConnectionProtocolSSH  = types.ConnectionProtocolSSH
	ConnectionProtocolExec = types.ConnectionProtocolExec
	ConnectionProtocolSSM  = types.ConnectionProtocolSSM
)
