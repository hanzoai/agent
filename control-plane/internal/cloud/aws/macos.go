package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/agents/control-plane/internal/cloud"
	"github.com/hanzoai/agents/control-plane/internal/storage"
)

// launchMacOSInstance provisions a macOS EC2 instance on a Dedicated Host.
func (p *Provisioner) launchMacOSInstance(ctx context.Context, req *cloud.ProvisionRequest, instanceID string) (*cloud.CloudInstance, error) {
	cfg := p.awsCfg.MacOS

	// Acquire a dedicated host.
	host, err := p.acquireDedicatedHost(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	userData, err := RenderUserData("macos", UserDataParams{
		ControlPlaneURL: p.serverURL,
		APIKey:          p.apiKey,
		InstanceID:      instanceID,
		BotPackage:      req.BotPackage,
		BotVersion:      req.BotVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render macOS userdata: %w", err)
	}

	tags := []ec2types.Tag{
		{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("hanzo-mac-bot-%s", instanceID[:8]))},
		{Key: aws.String("hanzo.ai/cloud-instance"), Value: aws.String(instanceID)},
		{Key: aws.String("hanzo.ai/platform"), Value: aws.String("macos")},
		{Key: aws.String("hanzo.ai/team"), Value: aws.String(req.TeamID)},
		{Key: aws.String("hanzo.ai/bot-package"), Value: aws.String(req.BotPackage)},
		{Key: aws.String("hanzo.ai/dedicated-host"), Value: aws.String(host.HostID)},
	}
	for k, v := range req.Tags {
		tags = append(tags, ec2types.Tag{Key: aws.String("hanzo.ai/tag-" + k), Value: aws.String(v)})
	}

	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(cfg.AMIID),
		InstanceType: ec2types.InstanceType(cfg.InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		UserData:     aws.String(userData),
		Placement: &ec2types.Placement{
			HostId: aws.String(host.HostID),
		},
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags:         tags,
			},
		},
	}

	if len(p.awsCfg.SubnetIDs) > 0 {
		input.SubnetId = aws.String(p.awsCfg.SubnetIDs[0])
	}
	if p.awsCfg.SecurityGroupID != "" {
		input.SecurityGroupIds = []string{p.awsCfg.SecurityGroupID}
	}
	if p.awsCfg.IAMInstanceProfile != "" {
		input.IamInstanceProfile = &ec2types.IamInstanceProfileSpecification{
			Name: aws.String(p.awsCfg.IAMInstanceProfile),
		}
	}

	out, err := p.clients.EC2.RunInstances(ctx, input)
	if err != nil {
		// Release host on failure.
		_ = p.releaseDedicatedHost(ctx, host.ID)
		return nil, &cloud.ProvisionError{
			InstanceID: instanceID,
			Platform:   cloud.PlatformMacOS,
			Provider:   "aws",
			Err:        err,
		}
	}

	ec2ID := aws.ToString(out.Instances[0].InstanceId)
	log.Info().
		Str("ec2_id", ec2ID).
		Str("instance_id", instanceID).
		Str("host_id", host.HostID).
		Msg("macOS EC2 instance launched on Dedicated Host")

	// Update host with instance assignment.
	if p.store != nil {
		host.CurrentInstanceID = instanceID
		host.State = "allocated"
		_ = p.store.UpdateDedicatedHost(ctx, host)
	}

	now := time.Now().UTC()
	return &cloud.CloudInstance{
		ID:              instanceID,
		Platform:        cloud.PlatformMacOS,
		State:           cloud.InstanceStateProvisioning,
		Provider:        "aws",
		InstanceID:      ec2ID,
		InstanceType:    cfg.InstanceType,
		ImageID:         cfg.AMIID,
		Region:          p.awsCfg.Region,
		BotPackage:      req.BotPackage,
		BotVersion:      req.BotVersion,
		TeamID:          req.TeamID,
		DedicatedHostID: host.HostID,
		Tags:            req.Tags,
		RequestedAt:     now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

// acquireDedicatedHost finds or allocates an available Dedicated Host.
func (p *Provisioner) acquireDedicatedHost(ctx context.Context, instanceID string) (*cloud.DedicatedHost, error) {
	if p.store == nil {
		return nil, fmt.Errorf("storage required for dedicated host management")
	}

	// Try to find an available host in the DB.
	host, err := p.store.GetAvailableDedicatedHost(ctx)
	if err == nil && host != nil {
		now := time.Now().UTC()
		host.State = "allocated"
		host.CurrentInstanceID = instanceID
		host.AllocatedAt = &now
		if err := p.store.UpdateDedicatedHost(ctx, host); err != nil {
			return nil, fmt.Errorf("failed to allocate host %s: %w", host.HostID, err)
		}
		log.Info().Str("host_id", host.HostID).Msg("acquired existing dedicated host")
		return host, nil
	}

	return nil, cloud.ErrNoAvailableHost
}

// releaseDedicatedHost marks a Dedicated Host as available.
func (p *Provisioner) releaseDedicatedHost(ctx context.Context, hostDBID string) error {
	if p.store == nil {
		return nil
	}

	host, err := p.store.GetDedicatedHost(ctx, hostDBID)
	if err != nil {
		return err
	}

	// Check minimum allocation period.
	if host.AllocatedAt != nil {
		minEnd := host.AllocatedAt.Add(host.MinAllocation)
		if time.Now().Before(minEnd) {
			return cloud.ErrHostMinAllocation
		}
	}

	now := time.Now().UTC()
	host.State = "available"
	host.CurrentInstanceID = ""
	host.ReleasedAt = &now
	return p.store.UpdateDedicatedHost(ctx, host)
}

// getMacOSConnectionInfo returns VNC connection info for a macOS instance.
func (p *Provisioner) getMacOSConnectionInfo(ctx context.Context, ec2ID, publicIP string) (*cloud.ConnectionInfo, error) {
	extra := buildSSMConnectionExtra(ec2ID, p.awsCfg.Region)
	extra["vnc_url"] = fmt.Sprintf("vnc://%s:5900", publicIP)

	return &cloud.ConnectionInfo{
		Protocol: cloud.ConnectionProtocolVNC,
		Host:     publicIP,
		Port:     5900,
		Extra:    extra,
	}, nil
}

// SeedDedicatedHosts creates DB records for pre-allocated host IDs from config.
func (p *Provisioner) SeedDedicatedHosts(ctx context.Context, store storage.StorageProvider) error {
	for _, hostID := range p.awsCfg.MacOS.DedicatedHostIDs {
		existing, _ := store.ListDedicatedHosts(ctx)
		found := false
		for _, h := range existing {
			if h.HostID == hostID {
				found = true
				break
			}
		}
		if found {
			continue
		}

		now := time.Now().UTC()
		host := &cloud.DedicatedHost{
			ID:            fmt.Sprintf("dh-%s", hostID),
			HostID:        hostID,
			InstanceType:  p.awsCfg.MacOS.InstanceType,
			State:         "available",
			MinAllocation: p.awsCfg.MacOS.MinHostAllocation,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := store.CreateDedicatedHost(ctx, host); err != nil {
			log.Warn().Err(err).Str("host_id", hostID).Msg("failed to seed dedicated host")
		} else {
			log.Info().Str("host_id", hostID).Msg("seeded dedicated host record")
		}
	}
	return nil
}
