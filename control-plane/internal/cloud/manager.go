package cloud

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/hanzoai/agents/control-plane/internal/storage"
)

// CloudManager dispatches cloud operations to the correct provisioner by platform.
type CloudManager struct {
	mu           sync.RWMutex
	config       CloudConfig
	store        storage.StorageProvider
	provisioners map[Platform]CloudProvisioner
	eventBus     *EventBus
	billing      BillingAuthorizer
}

// NewCloudManager creates a new CloudManager.
func NewCloudManager(cfg CloudConfig, store storage.StorageProvider) *CloudManager {
	var billing BillingAuthorizer
	if cfg.Billing.Enabled && cfg.Billing.ServiceURL != "" {
		billing = NewHTTPBillingClient(cfg.Billing.ServiceURL, cfg.Billing.APIKey)
		log.Info().Str("url", cfg.Billing.ServiceURL).Msg("cloud billing enabled")
	} else {
		billing = &NoopBillingClient{}
		log.Info().Msg("cloud billing disabled, all provisioning allowed")
	}

	return &CloudManager{
		config:       cfg,
		store:        store,
		provisioners: make(map[Platform]CloudProvisioner),
		eventBus:     NewEventBus(200),
		billing:      billing,
	}
}

// Billing returns the billing authorizer.
func (m *CloudManager) Billing() BillingAuthorizer {
	return m.billing
}

// RegisterProvisioner registers a provisioner for one or more platforms.
func (m *CloudManager) RegisterProvisioner(platforms []Platform, p CloudProvisioner) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, platform := range platforms {
		m.provisioners[platform] = p
		log.Info().Str("platform", string(platform)).Str("provider", p.ProviderName()).Msg("registered cloud provisioner")
	}
}

// EventBus returns the cloud event bus for subscribers.
func (m *CloudManager) EventBus() *EventBus {
	return m.eventBus
}

// Config returns the cloud config.
func (m *CloudManager) Config() CloudConfig {
	return m.config
}

// CreateInstance provisions a new cloud instance.
func (m *CloudManager) CreateInstance(ctx context.Context, req *ProvisionRequest) (*CloudInstance, error) {
	if !m.config.Enabled {
		return nil, ErrCloudDisabled
	}

	// Check team instance limit.
	if req.TeamID != "" && m.store != nil {
		count, err := m.store.CountCloudInstancesByTeam(ctx, req.TeamID)
		if err == nil && count >= m.config.MaxInstancesPerTeam {
			return nil, ErrMaxInstancesReached
		}
	}

	// Billing authorization check.
	auth, err := m.billing.AuthorizeProvisioning(ctx, req.TeamID, req.Platform, req.InstanceType)
	if err != nil {
		log.Error().Err(err).Str("team", req.TeamID).Msg("billing authorization check failed")
		return nil, ErrBillingServiceUnavailable
	}
	if !auth.Authorized {
		log.Warn().Str("team", req.TeamID).Str("reason", auth.Reason).Msg("billing denied provisioning")
		return nil, fmt.Errorf("%w: %s", ErrBillingNotAuthorized, auth.Reason)
	}

	prov, err := m.getProvisioner(req.Platform)
	if err != nil {
		return nil, err
	}

	m.eventBus.EmitInstanceEvent(EventInstanceRequested, "", map[string]string{
		"platform":    string(req.Platform),
		"bot_package": req.BotPackage,
		"team_id":     req.TeamID,
	})

	inst, err := prov.CreateInstance(ctx, req)
	if err != nil {
		return nil, err
	}

	// Attach billing metadata to instance.
	inst.HourlyRateCents = auth.HourlyCents
	inst.BillingTier = auth.Tier

	// Persist to storage.
	if m.store != nil {
		if storeErr := m.store.CreateCloudInstance(ctx, inst); storeErr != nil {
			log.Error().Err(storeErr).Str("id", inst.ID).Msg("failed to persist cloud instance")
		}
	}

	m.eventBus.EmitInstanceEvent(EventInstanceProvisioning, inst.ID, inst)
	return inst, nil
}

// GetInstance returns the current state of an instance.
func (m *CloudManager) GetInstance(ctx context.Context, instanceID string) (*CloudInstance, error) {
	if !m.config.Enabled {
		return nil, ErrCloudDisabled
	}

	// Try storage first.
	if m.store != nil {
		inst, err := m.store.GetCloudInstance(ctx, instanceID)
		if err == nil {
			return inst, nil
		}
	}

	// Fall through to provisioners.
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, prov := range m.provisioners {
		inst, err := prov.GetInstance(ctx, instanceID)
		if err == nil {
			return inst, nil
		}
	}

	return nil, ErrInstanceNotFound
}

// ListInstances returns instances matching filters.
func (m *CloudManager) ListInstances(ctx context.Context, filters InstanceFilters) ([]*CloudInstance, error) {
	if !m.config.Enabled {
		return nil, ErrCloudDisabled
	}

	// Use storage if available.
	if m.store != nil {
		return m.store.ListCloudInstances(ctx, filters)
	}

	// Fall back to aggregating from all provisioners.
	m.mu.RLock()
	defer m.mu.RUnlock()
	var all []*CloudInstance
	for _, prov := range m.provisioners {
		instances, err := prov.ListInstances(ctx, filters)
		if err != nil {
			log.Warn().Err(err).Str("provider", prov.ProviderName()).Msg("failed to list instances")
			continue
		}
		all = append(all, instances...)
	}
	return all, nil
}

// StartInstance starts a stopped instance.
func (m *CloudManager) StartInstance(ctx context.Context, instanceID string) error {
	inst, prov, err := m.resolveInstance(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := prov.StartInstance(ctx, instanceID); err != nil {
		return err
	}

	if m.store != nil {
		inst.State = InstanceStateRunning
		_ = m.store.UpdateCloudInstance(ctx, inst)
	}
	return nil
}

// StopInstance stops a running instance.
func (m *CloudManager) StopInstance(ctx context.Context, instanceID string) error {
	inst, prov, err := m.resolveInstance(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := prov.StopInstance(ctx, instanceID); err != nil {
		return err
	}

	if m.store != nil {
		inst.State = InstanceStateStopped
		_ = m.store.UpdateCloudInstance(ctx, inst)
	}

	m.eventBus.EmitInstanceEvent(EventInstanceStopped, instanceID, nil)
	return nil
}

// TerminateInstance permanently destroys an instance.
func (m *CloudManager) TerminateInstance(ctx context.Context, instanceID string) error {
	inst, prov, err := m.resolveInstance(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := prov.TerminateInstance(ctx, instanceID); err != nil {
		return err
	}

	if m.store != nil {
		inst.State = InstanceStateTerminated
		_ = m.store.UpdateCloudInstance(ctx, inst)
	}

	m.eventBus.EmitInstanceEvent(EventInstanceTerminated, instanceID, nil)
	return nil
}

// GetConnectionInfo returns connection details for the instance.
func (m *CloudManager) GetConnectionInfo(ctx context.Context, instanceID string) (*ConnectionInfo, error) {
	_, prov, err := m.resolveInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return prov.GetConnectionInfo(ctx, instanceID)
}

// ExecuteCommand runs a command on an instance.
func (m *CloudManager) ExecuteCommand(ctx context.Context, instanceID, command string) (*CommandResult, error) {
	_, prov, err := m.resolveInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return prov.ExecuteCommand(ctx, instanceID, command)
}

// GetLogs returns recent logs from an instance.
func (m *CloudManager) GetLogs(ctx context.Context, instanceID string, lines int) (string, error) {
	_, prov, err := m.resolveInstance(ctx, instanceID)
	if err != nil {
		return "", err
	}
	return prov.GetLogs(ctx, instanceID, lines)
}

// GetSummary returns a dashboard summary of cloud instances.
func (m *CloudManager) GetSummary(ctx context.Context) (*CloudSummary, error) {
	if !m.config.Enabled {
		return nil, ErrCloudDisabled
	}

	instances, err := m.ListInstances(ctx, InstanceFilters{})
	if err != nil {
		return nil, err
	}

	summary := &CloudSummary{
		ByPlatform: make(map[Platform]int),
		ByState:    make(map[InstanceState]int),
	}

	for _, inst := range instances {
		summary.TotalInstances++
		summary.ByPlatform[inst.Platform]++
		summary.ByState[inst.State]++
		summary.TotalAccruedCents += inst.AccruedCostCents

		// Rough hourly cost estimate for active instances.
		if inst.State == InstanceStateRunning || inst.State == InstanceStateProvisioning {
			if inst.HourlyRateCents > 0 {
				summary.EstimatedCostUSD += float64(inst.HourlyRateCents) / 100.0
			} else {
				switch inst.Platform {
				case PlatformMacOS:
					summary.EstimatedCostUSD += 1.20
				case PlatformWindows:
					summary.EstimatedCostUSD += 0.10
				case PlatformLinux:
					summary.EstimatedCostUSD += 0.01
				}
			}
		}
	}

	// Count active dedicated hosts.
	if m.store != nil {
		hosts, err := m.store.ListDedicatedHosts(ctx)
		if err == nil {
			for _, h := range hosts {
				if h.State == "allocated" {
					summary.ActiveHosts++
				}
			}
		}
	}

	return summary, nil
}

// getProvisioner returns the provisioner for the given platform.
func (m *CloudManager) getProvisioner(platform Platform) (CloudProvisioner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prov, ok := m.provisioners[platform]
	if !ok {
		return nil, fmt.Errorf("%w: no provisioner for platform %s", ErrInvalidPlatform, platform)
	}
	return prov, nil
}

// resolveInstance finds an instance and its provisioner.
func (m *CloudManager) resolveInstance(ctx context.Context, instanceID string) (*CloudInstance, CloudProvisioner, error) {
	if !m.config.Enabled {
		return nil, nil, ErrCloudDisabled
	}

	inst, err := m.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, nil, err
	}

	prov, err := m.getProvisioner(inst.Platform)
	if err != nil {
		return nil, nil, err
	}

	return inst, prov, nil
}
