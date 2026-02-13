package aws

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/agents/control-plane/internal/cloud"
	"github.com/hanzoai/agents/control-plane/internal/storage"
)

// Provisioner implements cloud.CloudProvisioner for AWS EC2 instances.
type Provisioner struct {
	clients   *Clients
	awsCfg    cloud.AWSConfig
	store     storage.StorageProvider
	serverURL string
	apiKey    string
}

// NewProvisioner creates a new AWS provisioner.
func NewProvisioner(ctx context.Context, cfg cloud.AWSConfig, store storage.StorageProvider, serverURL, apiKey string) (*Provisioner, error) {
	clients, err := NewClients(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Provisioner{
		clients:   clients,
		awsCfg:    cfg,
		store:     store,
		serverURL: serverURL,
		apiKey:    apiKey,
	}, nil
}

func (p *Provisioner) ProviderName() string { return "aws" }

// CreateInstance provisions a Windows or macOS EC2 instance.
func (p *Provisioner) CreateInstance(ctx context.Context, req *cloud.ProvisionRequest) (*cloud.CloudInstance, error) {
	instanceID := uuid.New().String()

	switch req.Platform {
	case cloud.PlatformWindows:
		return p.launchWindowsInstance(ctx, req, instanceID)
	case cloud.PlatformMacOS:
		return p.launchMacOSInstance(ctx, req, instanceID)
	default:
		return nil, fmt.Errorf("AWS provisioner does not support platform: %s", req.Platform)
	}
}

// GetInstance returns the current state of an EC2 instance.
func (p *Provisioner) GetInstance(ctx context.Context, instanceID string) (*cloud.CloudInstance, error) {
	ec2Instance, err := p.describeInstanceByTag(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	state := ec2StateToInstanceState(ec2Instance.State.Name)
	platform := cloud.PlatformWindows
	for _, tag := range ec2Instance.Tags {
		if awssdk.ToString(tag.Key) == "hanzo.ai/platform" {
			platform = cloud.Platform(awssdk.ToString(tag.Value))
		}
	}

	inst := &cloud.CloudInstance{
		ID:           instanceID,
		Platform:     platform,
		State:        state,
		Provider:     "aws",
		InstanceID:   awssdk.ToString(ec2Instance.InstanceId),
		InstanceType: string(ec2Instance.InstanceType),
		Region:       p.awsCfg.Region,
		PrivateIP:    awssdk.ToString(ec2Instance.PrivateIpAddress),
		PublicIP:     awssdk.ToString(ec2Instance.PublicIpAddress),
	}

	for _, tag := range ec2Instance.Tags {
		switch awssdk.ToString(tag.Key) {
		case "hanzo.ai/team":
			inst.TeamID = awssdk.ToString(tag.Value)
		case "hanzo.ai/bot-package":
			inst.BotPackage = awssdk.ToString(tag.Value)
		case "hanzo.ai/dedicated-host":
			inst.DedicatedHostID = awssdk.ToString(tag.Value)
		}
	}

	return inst, nil
}

// ListInstances returns EC2 instances matching filters.
func (p *Provisioner) ListInstances(ctx context.Context, filters cloud.InstanceFilters) ([]*cloud.CloudInstance, error) {
	ec2Filters := []ec2types.Filter{
		{
			Name:   awssdk.String("tag:hanzo.ai/cloud-instance"),
			Values: []string{"*"},
		},
	}
	if filters.TeamID != nil {
		ec2Filters = append(ec2Filters, ec2types.Filter{
			Name:   awssdk.String("tag:hanzo.ai/team"),
			Values: []string{*filters.TeamID},
		})
	}
	if filters.Platform != nil {
		ec2Filters = append(ec2Filters, ec2types.Filter{
			Name:   awssdk.String("tag:hanzo.ai/platform"),
			Values: []string{string(*filters.Platform)},
		})
	}

	out, err := p.clients.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: ec2Filters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe instances: %w", err)
	}

	var instances []*cloud.CloudInstance
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			state := ec2StateToInstanceState(inst.State.Name)
			if filters.State != nil && state != *filters.State {
				continue
			}

			ci := &cloud.CloudInstance{
				Provider:     "aws",
				State:        state,
				InstanceID:   awssdk.ToString(inst.InstanceId),
				InstanceType: string(inst.InstanceType),
				Region:       p.awsCfg.Region,
				PrivateIP:    awssdk.ToString(inst.PrivateIpAddress),
				PublicIP:     awssdk.ToString(inst.PublicIpAddress),
			}

			for _, tag := range inst.Tags {
				switch awssdk.ToString(tag.Key) {
				case "hanzo.ai/cloud-instance":
					ci.ID = awssdk.ToString(tag.Value)
				case "hanzo.ai/platform":
					ci.Platform = cloud.Platform(awssdk.ToString(tag.Value))
				case "hanzo.ai/team":
					ci.TeamID = awssdk.ToString(tag.Value)
				case "hanzo.ai/bot-package":
					ci.BotPackage = awssdk.ToString(tag.Value)
				case "hanzo.ai/dedicated-host":
					ci.DedicatedHostID = awssdk.ToString(tag.Value)
				}
			}

			instances = append(instances, ci)
		}
	}

	return instances, nil
}

// StartInstance starts a stopped EC2 instance.
func (p *Provisioner) StartInstance(ctx context.Context, instanceID string) error {
	ec2Instance, err := p.describeInstanceByTag(ctx, instanceID)
	if err != nil {
		return err
	}

	ec2ID := awssdk.ToString(ec2Instance.InstanceId)
	_, err = p.clients.EC2.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{ec2ID},
	})
	if err != nil {
		return fmt.Errorf("failed to start instance %s: %w", ec2ID, err)
	}

	log.Info().Str("ec2_id", ec2ID).Str("instance_id", instanceID).Msg("EC2 instance started")
	return nil
}

// StopInstance stops a running EC2 instance.
func (p *Provisioner) StopInstance(ctx context.Context, instanceID string) error {
	ec2Instance, err := p.describeInstanceByTag(ctx, instanceID)
	if err != nil {
		return err
	}

	ec2ID := awssdk.ToString(ec2Instance.InstanceId)
	_, err = p.clients.EC2.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{ec2ID},
	})
	if err != nil {
		return fmt.Errorf("failed to stop instance %s: %w", ec2ID, err)
	}

	log.Info().Str("ec2_id", ec2ID).Str("instance_id", instanceID).Msg("EC2 instance stopped")
	return nil
}

// TerminateInstance permanently terminates an EC2 instance.
func (p *Provisioner) TerminateInstance(ctx context.Context, instanceID string) error {
	ec2Instance, err := p.describeInstanceByTag(ctx, instanceID)
	if err != nil {
		return err
	}

	ec2ID := awssdk.ToString(ec2Instance.InstanceId)
	_, err = p.clients.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{ec2ID},
	})
	if err != nil {
		return fmt.Errorf("failed to terminate instance %s: %w", ec2ID, err)
	}

	// Release dedicated host if macOS.
	for _, tag := range ec2Instance.Tags {
		if awssdk.ToString(tag.Key) == "hanzo.ai/dedicated-host" {
			hostID := awssdk.ToString(tag.Value)
			if p.store != nil {
				hosts, _ := p.store.ListDedicatedHosts(ctx)
				for _, h := range hosts {
					if h.HostID == hostID {
						_ = p.releaseDedicatedHost(ctx, h.ID)
						break
					}
				}
			}
		}
	}

	log.Info().Str("ec2_id", ec2ID).Str("instance_id", instanceID).Msg("EC2 instance terminated")
	return nil
}

// GetConnectionInfo returns connection details for the instance.
func (p *Provisioner) GetConnectionInfo(ctx context.Context, instanceID string) (*cloud.ConnectionInfo, error) {
	ec2Instance, err := p.describeInstanceByTag(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	ec2ID := awssdk.ToString(ec2Instance.InstanceId)
	publicIP := awssdk.ToString(ec2Instance.PublicIpAddress)
	if publicIP == "" {
		publicIP = awssdk.ToString(ec2Instance.PrivateIpAddress)
	}

	platform := cloud.PlatformWindows
	for _, tag := range ec2Instance.Tags {
		if awssdk.ToString(tag.Key) == "hanzo.ai/platform" {
			platform = cloud.Platform(awssdk.ToString(tag.Value))
		}
	}

	switch platform {
	case cloud.PlatformWindows:
		return p.getWindowsConnectionInfo(ctx, ec2ID, publicIP)
	case cloud.PlatformMacOS:
		return p.getMacOSConnectionInfo(ctx, ec2ID, publicIP)
	default:
		return &cloud.ConnectionInfo{
			Protocol: cloud.ConnectionProtocolSSM,
			Host:     publicIP,
			Extra:    buildSSMConnectionExtra(ec2ID, p.awsCfg.Region),
		}, nil
	}
}

// ExecuteCommand runs a command on the instance via SSM.
func (p *Provisioner) ExecuteCommand(ctx context.Context, instanceID, command string) (*cloud.CommandResult, error) {
	ec2Instance, err := p.describeInstanceByTag(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	ec2ID := awssdk.ToString(ec2Instance.InstanceId)
	platform := "linux"
	for _, tag := range ec2Instance.Tags {
		if awssdk.ToString(tag.Key) == "hanzo.ai/platform" {
			platform = awssdk.ToString(tag.Value)
		}
	}

	stdout, err := RunCommand(ctx, p.clients.SSM, ec2ID, command, platform)
	if err != nil {
		return &cloud.CommandResult{
			ExitCode: 1,
			Stderr:   err.Error(),
		}, nil
	}

	return &cloud.CommandResult{
		ExitCode: 0,
		Stdout:   stdout,
	}, nil
}

// GetLogs returns recent logs from the instance via SSM.
func (p *Provisioner) GetLogs(ctx context.Context, instanceID string, lines int) (string, error) {
	ec2Instance, err := p.describeInstanceByTag(ctx, instanceID)
	if err != nil {
		return "", err
	}

	ec2ID := awssdk.ToString(ec2Instance.InstanceId)
	cmd := fmt.Sprintf("tail -n %d /var/log/hanzo-agent.log 2>/dev/null || journalctl -n %d -u hanzo-agent 2>/dev/null || echo 'No logs found'", lines, lines)

	// Check platform for Windows.
	for _, tag := range ec2Instance.Tags {
		if awssdk.ToString(tag.Key) == "hanzo.ai/platform" && awssdk.ToString(tag.Value) == "windows" {
			cmd = fmt.Sprintf("Get-Content 'C:\\ProgramData\\hanzo-agent\\agent.log' -Tail %d -ErrorAction SilentlyContinue", lines)
		}
	}

	platform := "linux"
	for _, tag := range ec2Instance.Tags {
		if awssdk.ToString(tag.Key) == "hanzo.ai/platform" {
			platform = awssdk.ToString(tag.Value)
		}
	}

	return RunCommand(ctx, p.clients.SSM, ec2ID, cmd, platform)
}

// describeInstanceByTag finds an EC2 instance by the cloud instance ID tag.
func (p *Provisioner) describeInstanceByTag(ctx context.Context, instanceID string) (*ec2types.Instance, error) {
	out, err := p.clients.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   awssdk.String("tag:hanzo.ai/cloud-instance"),
				Values: []string{instanceID},
			},
			{
				Name:   awssdk.String("instance-state-name"),
				Values: []string{"pending", "running", "stopping", "stopped"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe instances: %w", err)
	}

	for _, res := range out.Reservations {
		if len(res.Instances) > 0 {
			return &res.Instances[0], nil
		}
	}

	return nil, cloud.ErrInstanceNotFound
}

// ec2StateToInstanceState maps EC2 instance state to cloud InstanceState.
func ec2StateToInstanceState(state ec2types.InstanceStateName) cloud.InstanceState {
	switch state {
	case ec2types.InstanceStateNamePending:
		return cloud.InstanceStateProvisioning
	case ec2types.InstanceStateNameRunning:
		return cloud.InstanceStateRunning
	case ec2types.InstanceStateNameStopping, ec2types.InstanceStateNameStopped:
		return cloud.InstanceStateStopped
	case ec2types.InstanceStateNameShuttingDown, ec2types.InstanceStateNameTerminated:
		return cloud.InstanceStateTerminated
	default:
		return cloud.InstanceStateFailed
	}
}

// SyncInstanceState checks actual EC2 state and updates the cloud instance.
func (p *Provisioner) SyncInstanceState(ctx context.Context, inst *cloud.CloudInstance) (*cloud.CloudInstance, error) {
	updated, err := p.GetInstance(ctx, inst.ID)
	if err != nil {
		return inst, err
	}

	now := time.Now().UTC()
	inst.State = updated.State
	inst.PublicIP = updated.PublicIP
	inst.PrivateIP = updated.PrivateIP
	inst.UpdatedAt = now

	if updated.State == cloud.InstanceStateRunning && inst.ProvisionedAt == nil {
		inst.ProvisionedAt = &now
	}
	if updated.State == cloud.InstanceStateTerminated && inst.TerminatedAt == nil {
		inst.TerminatedAt = &now
	}

	return inst, nil
}
