package cloud

import "context"

// CloudProvisioner defines the interface for cloud instance provisioners.
// Each provider (K8s, AWS) implements this interface.
type CloudProvisioner interface {
	// CreateInstance provisions a new cloud instance.
	CreateInstance(ctx context.Context, req *ProvisionRequest) (*CloudInstance, error)

	// GetInstance returns the current state of an instance.
	GetInstance(ctx context.Context, instanceID string) (*CloudInstance, error)

	// ListInstances returns instances matching the given filters.
	ListInstances(ctx context.Context, filters InstanceFilters) ([]*CloudInstance, error)

	// StartInstance starts a stopped instance.
	StartInstance(ctx context.Context, instanceID string) error

	// StopInstance stops a running instance.
	StopInstance(ctx context.Context, instanceID string) error

	// TerminateInstance permanently destroys an instance.
	TerminateInstance(ctx context.Context, instanceID string) error

	// GetConnectionInfo returns connection details for the instance.
	GetConnectionInfo(ctx context.Context, instanceID string) (*ConnectionInfo, error)

	// ExecuteCommand runs a command on the instance.
	ExecuteCommand(ctx context.Context, instanceID, command string) (*CommandResult, error)

	// GetLogs returns recent logs from the instance.
	GetLogs(ctx context.Context, instanceID string, lines int) (string, error)

	// ProviderName returns the name of this provisioner (e.g., "k8s", "aws").
	ProviderName() string
}
