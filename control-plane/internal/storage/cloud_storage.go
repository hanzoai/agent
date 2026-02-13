package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanzoai/agents/control-plane/pkg/types"
	"gorm.io/gorm"
)

// --- CloudInstance CRUD ---

func (ls *LocalStorage) CreateCloudInstance(ctx context.Context, instance *types.CloudInstance) error {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to prepare gorm: %w", err)
	}

	model, err := cloudInstanceToModel(instance)
	if err != nil {
		return err
	}

	if result := gormDB.Create(model); result.Error != nil {
		return fmt.Errorf("failed to create cloud instance: %w", result.Error)
	}
	return nil
}

func (ls *LocalStorage) GetCloudInstance(ctx context.Context, id string) (*types.CloudInstance, error) {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare gorm: %w", err)
	}

	var model CloudInstanceModel
	if err := gormDB.Where("id = ?", id).Take(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("cloud instance not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get cloud instance: %w", err)
	}

	return modelToCloudInstance(&model)
}

func (ls *LocalStorage) UpdateCloudInstance(ctx context.Context, instance *types.CloudInstance) error {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to prepare gorm: %w", err)
	}

	model, err := cloudInstanceToModel(instance)
	if err != nil {
		return err
	}

	if result := gormDB.Save(model); result.Error != nil {
		return fmt.Errorf("failed to update cloud instance: %w", result.Error)
	}
	return nil
}

func (ls *LocalStorage) ListCloudInstances(ctx context.Context, filters types.InstanceFilters) ([]*types.CloudInstance, error) {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare gorm: %w", err)
	}

	query := gormDB.Model(&CloudInstanceModel{})

	if filters.Platform != nil {
		query = query.Where("platform = ?", string(*filters.Platform))
	}
	if filters.State != nil {
		query = query.Where("state = ?", string(*filters.State))
	}
	if filters.TeamID != nil {
		query = query.Where("team_id = ?", *filters.TeamID)
	}
	if filters.Provider != nil {
		query = query.Where("provider = ?", *filters.Provider)
	}

	query = query.Order("created_at DESC")

	if filters.Limit > 0 {
		query = query.Limit(filters.Limit)
	}
	if filters.Offset > 0 {
		query = query.Offset(filters.Offset)
	}

	var models []CloudInstanceModel
	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to list cloud instances: %w", err)
	}

	instances := make([]*types.CloudInstance, 0, len(models))
	for _, m := range models {
		inst, err := modelToCloudInstance(&m)
		if err != nil {
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

func (ls *LocalStorage) DeleteCloudInstance(ctx context.Context, id string) error {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to prepare gorm: %w", err)
	}

	if result := gormDB.Where("id = ?", id).Delete(&CloudInstanceModel{}); result.Error != nil {
		return fmt.Errorf("failed to delete cloud instance: %w", result.Error)
	}
	return nil
}

func (ls *LocalStorage) GetCloudInstanceByAgentNodeID(ctx context.Context, agentNodeID string) (*types.CloudInstance, error) {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare gorm: %w", err)
	}

	var model CloudInstanceModel
	if err := gormDB.Where("agent_node_id = ?", agentNodeID).Take(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("cloud instance not found for agent node: %s", agentNodeID)
		}
		return nil, fmt.Errorf("failed to get cloud instance by agent node: %w", err)
	}

	return modelToCloudInstance(&model)
}

func (ls *LocalStorage) CountCloudInstancesByTeam(ctx context.Context, teamID string) (int, error) {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare gorm: %w", err)
	}

	var count int64
	if err := gormDB.Model(&CloudInstanceModel{}).
		Where("team_id = ? AND state NOT IN (?)", teamID, []string{"terminated", "failed"}).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count cloud instances: %w", err)
	}
	return int(count), nil
}

// --- DedicatedHost CRUD ---

func (ls *LocalStorage) CreateDedicatedHost(ctx context.Context, host *types.DedicatedHost) error {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to prepare gorm: %w", err)
	}

	model := dedicatedHostToModel(host)
	if result := gormDB.Create(model); result.Error != nil {
		return fmt.Errorf("failed to create dedicated host: %w", result.Error)
	}
	return nil
}

func (ls *LocalStorage) GetDedicatedHost(ctx context.Context, id string) (*types.DedicatedHost, error) {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare gorm: %w", err)
	}

	var model DedicatedHostModel
	if err := gormDB.Where("id = ?", id).Take(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("dedicated host not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get dedicated host: %w", err)
	}

	return modelToDedicatedHost(&model), nil
}

func (ls *LocalStorage) UpdateDedicatedHost(ctx context.Context, host *types.DedicatedHost) error {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to prepare gorm: %w", err)
	}

	model := dedicatedHostToModel(host)
	if result := gormDB.Save(model); result.Error != nil {
		return fmt.Errorf("failed to update dedicated host: %w", result.Error)
	}
	return nil
}

func (ls *LocalStorage) ListDedicatedHosts(ctx context.Context) ([]*types.DedicatedHost, error) {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare gorm: %w", err)
	}

	var models []DedicatedHostModel
	if err := gormDB.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to list dedicated hosts: %w", err)
	}

	hosts := make([]*types.DedicatedHost, 0, len(models))
	for _, m := range models {
		hosts = append(hosts, modelToDedicatedHost(&m))
	}
	return hosts, nil
}

func (ls *LocalStorage) GetAvailableDedicatedHost(ctx context.Context) (*types.DedicatedHost, error) {
	gormDB, err := ls.gormWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare gorm: %w", err)
	}

	var model DedicatedHostModel
	if err := gormDB.Where("state = ? AND current_instance_id = ?", "available", "").
		Order("updated_at ASC").
		Take(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no available dedicated host")
		}
		return nil, fmt.Errorf("failed to get available host: %w", err)
	}

	return modelToDedicatedHost(&model), nil
}

// --- Model ↔ Type conversion ---

func cloudInstanceToModel(inst *types.CloudInstance) (*CloudInstanceModel, error) {
	var connInfoBytes []byte
	if inst.ConnectionInfo != nil {
		var err error
		connInfoBytes, err = json.Marshal(inst.ConnectionInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal connection info: %w", err)
		}
	}

	var tagsBytes []byte
	if inst.Tags != nil {
		var err error
		tagsBytes, err = json.Marshal(inst.Tags)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tags: %w", err)
		}
	}

	return &CloudInstanceModel{
		ID:               inst.ID,
		Platform:         string(inst.Platform),
		State:            string(inst.State),
		Provider:         inst.Provider,
		InstanceID:       inst.InstanceID,
		InstanceType:     inst.InstanceType,
		ImageID:          inst.ImageID,
		Region:           inst.Region,
		BotPackage:       inst.BotPackage,
		BotVersion:       inst.BotVersion,
		PublicIP:         inst.PublicIP,
		PrivateIP:        inst.PrivateIP,
		AgentNodeID:      inst.AgentNodeID,
		TeamID:           inst.TeamID,
		DedicatedHostID:  inst.DedicatedHostID,
		HourlyRateCents:  inst.HourlyRateCents,
		AccruedCostCents: inst.AccruedCostCents,
		BillingTier:      inst.BillingTier,
		ConnectionInfo:   connInfoBytes,
		Metadata:         inst.Metadata,
		Tags:             tagsBytes,
		ErrorMessage:     inst.ErrorMessage,
		RequestedAt:      inst.RequestedAt,
		ProvisionedAt:    inst.ProvisionedAt,
		TerminatedAt:     inst.TerminatedAt,
	}, nil
}

func modelToCloudInstance(m *CloudInstanceModel) (*types.CloudInstance, error) {
	inst := &types.CloudInstance{
		ID:               m.ID,
		Platform:         types.Platform(m.Platform),
		State:            types.InstanceState(m.State),
		Provider:         m.Provider,
		InstanceID:       m.InstanceID,
		InstanceType:     m.InstanceType,
		ImageID:          m.ImageID,
		Region:           m.Region,
		BotPackage:       m.BotPackage,
		BotVersion:       m.BotVersion,
		PublicIP:         m.PublicIP,
		PrivateIP:        m.PrivateIP,
		AgentNodeID:      m.AgentNodeID,
		TeamID:           m.TeamID,
		DedicatedHostID:  m.DedicatedHostID,
		HourlyRateCents:  m.HourlyRateCents,
		AccruedCostCents: m.AccruedCostCents,
		BillingTier:      m.BillingTier,
		Metadata:         m.Metadata,
		ErrorMessage:     m.ErrorMessage,
		RequestedAt:      m.RequestedAt,
		ProvisionedAt:    m.ProvisionedAt,
		TerminatedAt:     m.TerminatedAt,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}

	if len(m.ConnectionInfo) > 0 {
		var ci types.ConnectionInfo
		if err := json.Unmarshal(m.ConnectionInfo, &ci); err == nil {
			inst.ConnectionInfo = &ci
		}
	}

	if len(m.Tags) > 0 {
		var tags map[string]string
		if err := json.Unmarshal(m.Tags, &tags); err == nil {
			inst.Tags = tags
		}
	}

	return inst, nil
}

func dedicatedHostToModel(h *types.DedicatedHost) *DedicatedHostModel {
	return &DedicatedHostModel{
		ID:                h.ID,
		HostID:            h.HostID,
		InstanceType:      h.InstanceType,
		State:             h.State,
		CurrentInstanceID: h.CurrentInstanceID,
		AllocatedAt:       h.AllocatedAt,
		ReleasedAt:        h.ReleasedAt,
		MinAllocationSec:  int64(h.MinAllocation / time.Second),
	}
}

func modelToDedicatedHost(m *DedicatedHostModel) *types.DedicatedHost {
	return &types.DedicatedHost{
		ID:                m.ID,
		HostID:            m.HostID,
		InstanceType:      m.InstanceType,
		State:             m.State,
		CurrentInstanceID: m.CurrentInstanceID,
		AllocatedAt:       m.AllocatedAt,
		ReleasedAt:        m.ReleasedAt,
		MinAllocation:     time.Duration(m.MinAllocationSec) * time.Second,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}
