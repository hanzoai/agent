package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/agents/control-plane/internal/cloud"
)

// launchWindowsInstance provisions a Windows EC2 instance.
func (p *Provisioner) launchWindowsInstance(ctx context.Context, req *cloud.ProvisionRequest, instanceID string) (*cloud.CloudInstance, error) {
	cfg := p.awsCfg.Windows

	instanceType := ec2types.InstanceType(cfg.DefaultInstanceType)
	if req.InstanceType != "" {
		instanceType = ec2types.InstanceType(req.InstanceType)
	}

	userData, err := RenderUserData("windows", UserDataParams{
		ControlPlaneURL: p.serverURL,
		APIKey:          p.apiKey,
		InstanceID:      instanceID,
		BotPackage:      req.BotPackage,
		BotVersion:      req.BotVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render Windows userdata: %w", err)
	}

	tags := []ec2types.Tag{
		{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("hanzo-bot-%s", instanceID[:8]))},
		{Key: aws.String("hanzo.ai/cloud-instance"), Value: aws.String(instanceID)},
		{Key: aws.String("hanzo.ai/platform"), Value: aws.String("windows")},
		{Key: aws.String("hanzo.ai/team"), Value: aws.String(req.TeamID)},
		{Key: aws.String("hanzo.ai/bot-package"), Value: aws.String(req.BotPackage)},
	}
	for k, v := range req.Tags {
		tags = append(tags, ec2types.Tag{Key: aws.String("hanzo.ai/tag-" + k), Value: aws.String(v)})
	}

	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(cfg.AMIID),
		InstanceType: instanceType,
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		UserData:     aws.String(userData),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags:         tags,
			},
		},
	}

	// Network config
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

	// Enable password retrieval via GetPasswordData
	input.KeyName = aws.String("hanzo-agent-windows")

	out, err := p.clients.EC2.RunInstances(ctx, input)
	if err != nil {
		return nil, &cloud.ProvisionError{
			InstanceID: instanceID,
			Platform:   cloud.PlatformWindows,
			Provider:   "aws",
			Err:        err,
		}
	}

	ec2ID := aws.ToString(out.Instances[0].InstanceId)
	log.Info().
		Str("ec2_id", ec2ID).
		Str("instance_id", instanceID).
		Str("type", string(instanceType)).
		Msg("Windows EC2 instance launched")

	now := time.Now().UTC()
	return &cloud.CloudInstance{
		ID:           instanceID,
		Platform:     cloud.PlatformWindows,
		State:        cloud.InstanceStateProvisioning,
		Provider:     "aws",
		InstanceID:   ec2ID,
		InstanceType: string(instanceType),
		ImageID:      cfg.AMIID,
		Region:       p.awsCfg.Region,
		BotPackage:   req.BotPackage,
		BotVersion:   req.BotVersion,
		TeamID:       req.TeamID,
		Tags:         req.Tags,
		RequestedAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// getWindowsConnectionInfo returns RDP connection info for a Windows instance.
func (p *Provisioner) getWindowsConnectionInfo(ctx context.Context, ec2ID, publicIP string) (*cloud.ConnectionInfo, error) {
	conn := &cloud.ConnectionInfo{
		Protocol: cloud.ConnectionProtocolRDP,
		Host:     publicIP,
		Port:     3389,
		Username: "Administrator",
		Extra:    buildSSMConnectionExtra(ec2ID, p.awsCfg.Region),
	}

	// Try to retrieve the password.
	passOut, err := p.clients.EC2.GetPasswordData(ctx, &ec2.GetPasswordDataInput{
		InstanceId: aws.String(ec2ID),
	})
	if err == nil && passOut.PasswordData != nil && *passOut.PasswordData != "" {
		// Password is base64-encoded and RSA-encrypted with the key pair.
		// We store the encrypted blob; client needs the private key to decrypt.
		decoded, err := base64.StdEncoding.DecodeString(*passOut.PasswordData)
		if err == nil {
			conn.Extra["encrypted_password"] = base64.StdEncoding.EncodeToString(decoded)
		}
	}

	return conn, nil
}
