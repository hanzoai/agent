package cloud

import (
	"errors"
	"fmt"
)

var (
	// ErrCloudDisabled is returned when cloud provisioning is disabled.
	ErrCloudDisabled = errors.New("cloud provisioning is disabled")

	// ErrProviderDisabled is returned when a specific provider is disabled.
	ErrProviderDisabled = errors.New("cloud provider is disabled")

	// ErrInstanceNotFound is returned when a cloud instance is not found.
	ErrInstanceNotFound = errors.New("cloud instance not found")

	// ErrInstanceAlreadyExists is returned when trying to create a duplicate instance.
	ErrInstanceAlreadyExists = errors.New("cloud instance already exists")

	// ErrInvalidPlatform is returned for unsupported platform types.
	ErrInvalidPlatform = errors.New("invalid platform")

	// ErrNoAvailableHost is returned when no macOS Dedicated Host is available.
	ErrNoAvailableHost = errors.New("no available dedicated host")

	// ErrProvisioningTimeout is returned when instance provisioning exceeds the timeout.
	ErrProvisioningTimeout = errors.New("instance provisioning timed out")

	// ErrMaxInstancesReached is returned when a team exceeds the instance limit.
	ErrMaxInstancesReached = errors.New("maximum instances per team reached")

	// ErrInvalidState is returned for invalid state transitions.
	ErrInvalidState = errors.New("invalid instance state for requested operation")

	// ErrHostMinAllocation is returned when trying to release a host before minimum allocation.
	ErrHostMinAllocation = errors.New("dedicated host minimum allocation period not met")

	// ErrBillingNotAuthorized is returned when billing check denies provisioning.
	ErrBillingNotAuthorized = errors.New("billing authorization denied")

	// ErrBillingQuotaExceeded is returned when a team exceeds their tier's cloud quota.
	ErrBillingQuotaExceeded = errors.New("cloud compute quota exceeded for billing tier")

	// ErrBillingServiceUnavailable is returned when the billing service is unreachable.
	ErrBillingServiceUnavailable = errors.New("billing service unavailable")
)

// ProvisionError wraps an error with provisioning context.
type ProvisionError struct {
	InstanceID string
	Platform   Platform
	Provider   string
	Err        error
}

func (e *ProvisionError) Error() string {
	return fmt.Sprintf("provisioning failed for %s instance %s on %s: %v",
		e.Platform, e.InstanceID, e.Provider, e.Err)
}

func (e *ProvisionError) Unwrap() error {
	return e.Err
}
