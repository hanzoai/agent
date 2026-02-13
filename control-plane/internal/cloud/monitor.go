package cloud

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/hanzoai/agents/control-plane/internal/storage"
)

// CloudInstanceMonitor runs background checks on cloud instances.
type CloudInstanceMonitor struct {
	manager  *CloudManager
	store    storage.StorageProvider
	config   CloudConfig
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewCloudInstanceMonitor creates a new monitor.
func NewCloudInstanceMonitor(manager *CloudManager, store storage.StorageProvider, cfg CloudConfig) *CloudInstanceMonitor {
	return &CloudInstanceMonitor{
		manager: manager,
		store:   store,
		config:  cfg,
		stopCh:  make(chan struct{}),
	}
}

// Start begins the monitor loop.
func (m *CloudInstanceMonitor) Start() {
	ticker := time.NewTicker(m.config.MonitorInterval)
	defer ticker.Stop()

	log.Info().Dur("interval", m.config.MonitorInterval).Msg("cloud instance monitor started")

	for {
		select {
		case <-m.stopCh:
			log.Info().Msg("cloud instance monitor stopped")
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

// Stop terminates the monitor loop.
func (m *CloudInstanceMonitor) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

func (m *CloudInstanceMonitor) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	m.cleanupStaleProvisioning(ctx)
	m.syncInstanceStates(ctx)
	m.accrueUsageCosts(ctx)
	m.releaseIdleHosts(ctx)
}

// accrueUsageCosts calculates and reports compute costs for running instances.
func (m *CloudInstanceMonitor) accrueUsageCosts(ctx context.Context) {
	runningState := InstanceStateRunning
	instances, err := m.store.ListCloudInstances(ctx, InstanceFilters{
		State: &runningState,
	})
	if err != nil {
		log.Error().Err(err).Msg("monitor: failed to list running instances for billing")
		return
	}

	intervalHours := m.config.MonitorInterval.Hours()

	for _, inst := range instances {
		if inst.HourlyRateCents <= 0 {
			continue
		}

		// Accrue cost for this interval.
		costCents := int(float64(inst.HourlyRateCents) * intervalHours)
		if costCents < 1 {
			costCents = 1 // minimum 1 cent per interval to avoid rounding to zero
		}

		inst.AccruedCostCents += costCents
		if err := m.store.UpdateCloudInstance(ctx, inst); err != nil {
			log.Error().Err(err).Str("id", inst.ID).Msg("monitor: failed to update accrued cost")
			continue
		}

		// Report usage to billing service.
		if err := m.manager.billing.ReportUsage(ctx, inst.ID, inst.Platform, intervalHours, inst.HourlyRateCents); err != nil {
			log.Warn().Err(err).Str("id", inst.ID).Msg("monitor: failed to report usage to billing")
		}
	}
}

// cleanupStaleProvisioning terminates instances stuck in "provisioning" state.
func (m *CloudInstanceMonitor) cleanupStaleProvisioning(ctx context.Context) {
	provState := InstanceStateProvisioning
	instances, err := m.store.ListCloudInstances(ctx, InstanceFilters{
		State: &provState,
	})
	if err != nil {
		log.Error().Err(err).Msg("monitor: failed to list provisioning instances")
		return
	}

	cutoff := time.Now().UTC().Add(-m.config.ProvisioningTimeout)
	for _, inst := range instances {
		if inst.CreatedAt.Before(cutoff) {
			log.Warn().
				Str("id", inst.ID).
				Str("platform", string(inst.Platform)).
				Time("created", inst.CreatedAt).
				Msg("terminating stale provisioning instance")

			if err := m.manager.TerminateInstance(ctx, inst.ID); err != nil {
				log.Error().Err(err).Str("id", inst.ID).Msg("failed to terminate stale instance")
				// Mark as failed.
				inst.State = InstanceStateFailed
				inst.ErrorMessage = "provisioning timeout"
				_ = m.store.UpdateCloudInstance(ctx, inst)
			}

			m.manager.EventBus().EmitInstanceEvent(EventInstanceFailed, inst.ID, map[string]string{
				"reason": "provisioning_timeout",
			})
		}
	}
}

// syncInstanceStates refreshes cloud state from provisioners and updates storage.
func (m *CloudInstanceMonitor) syncInstanceStates(ctx context.Context) {
	runningState := InstanceStateRunning
	instances, err := m.store.ListCloudInstances(ctx, InstanceFilters{
		State: &runningState,
	})
	if err != nil {
		log.Error().Err(err).Msg("monitor: failed to list running instances")
		return
	}

	for _, inst := range instances {
		prov, err := m.manager.getProvisioner(inst.Platform)
		if err != nil {
			continue
		}

		live, err := prov.GetInstance(ctx, inst.ID)
		if err != nil {
			log.Warn().Err(err).Str("id", inst.ID).Msg("monitor: could not sync instance")
			continue
		}

		if live.State != inst.State {
			log.Info().
				Str("id", inst.ID).
				Str("old_state", string(inst.State)).
				Str("new_state", string(live.State)).
				Msg("monitor: instance state changed")

			inst.State = live.State
			inst.PublicIP = live.PublicIP
			inst.PrivateIP = live.PrivateIP
			inst.UpdatedAt = time.Now().UTC()

			if live.State == InstanceStateTerminated {
				now := time.Now().UTC()
				inst.TerminatedAt = &now
			}
			if live.State == InstanceStateRunning && inst.ProvisionedAt == nil {
				now := time.Now().UTC()
				inst.ProvisionedAt = &now
			}

			_ = m.store.UpdateCloudInstance(ctx, inst)

			// Emit appropriate event.
			switch live.State {
			case InstanceStateRunning:
				m.manager.EventBus().EmitInstanceEvent(EventInstanceRunning, inst.ID, inst)
			case InstanceStateTerminated:
				m.manager.EventBus().EmitInstanceEvent(EventInstanceTerminated, inst.ID, nil)
			case InstanceStateFailed:
				m.manager.EventBus().EmitInstanceEvent(EventInstanceFailed, inst.ID, nil)
			}
		}
	}
}

// releaseIdleHosts releases macOS Dedicated Hosts past the idle release threshold.
func (m *CloudInstanceMonitor) releaseIdleHosts(ctx context.Context) {
	if !m.config.AWS.Enabled {
		return
	}

	hosts, err := m.store.ListDedicatedHosts(ctx)
	if err != nil {
		log.Error().Err(err).Msg("monitor: failed to list dedicated hosts")
		return
	}

	for _, host := range hosts {
		if host.State != "allocated" || host.CurrentInstanceID != "" {
			continue
		}

		if host.AllocatedAt == nil {
			continue
		}

		idleSince := *host.AllocatedAt
		if time.Since(idleSince) > m.config.AWS.MacOS.IdleHostRelease {
			log.Info().
				Str("host_id", host.HostID).
				Time("allocated_at", idleSince).
				Msg("releasing idle dedicated host")

			now := time.Now().UTC()
			host.State = "available"
			host.ReleasedAt = &now
			if err := m.store.UpdateDedicatedHost(ctx, host); err != nil {
				log.Error().Err(err).Str("host_id", host.HostID).Msg("failed to release host")
			} else {
				m.manager.EventBus().EmitInstanceEvent(EventHostReleased, host.HostID, nil)
			}
		}
	}
}
