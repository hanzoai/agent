package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/rs/zerolog/log"
)

// RunCommand executes a command on an EC2 instance via SSM.
func RunCommand(ctx context.Context, ssmClient *ssm.Client, instanceID, command, platform string) (string, error) {
	docName := "AWS-RunShellScript"
	if platform == "windows" {
		docName = "AWS-RunPowerShellScript"
	}

	out, err := ssmClient.SendCommand(ctx, &ssm.SendCommandInput{
		InstanceIds:  []string{instanceID},
		DocumentName: aws.String(docName),
		Parameters: map[string][]string{
			"commands": {command},
		},
		TimeoutSeconds: int32Ptr(120),
	})
	if err != nil {
		return "", fmt.Errorf("SSM send command failed: %w", err)
	}

	commandID := *out.Command.CommandId
	log.Debug().Str("command_id", commandID).Str("instance", instanceID).Msg("SSM command sent")

	// Poll for result.
	for i := 0; i < 60; i++ {
		time.Sleep(2 * time.Second)

		inv, err := ssmClient.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
			CommandId:  aws.String(commandID),
			InstanceId: aws.String(instanceID),
		})
		if err != nil {
			continue // may not be ready yet
		}

		switch inv.Status {
		case ssmtypes.CommandInvocationStatusSuccess:
			return aws.ToString(inv.StandardOutputContent), nil
		case ssmtypes.CommandInvocationStatusFailed,
			ssmtypes.CommandInvocationStatusTimedOut,
			ssmtypes.CommandInvocationStatusCancelled:
			stderr := aws.ToString(inv.StandardErrorContent)
			return "", fmt.Errorf("SSM command %s: %s", inv.Status, stderr)
		}
	}

	return "", fmt.Errorf("SSM command timed out waiting for result")
}

// WaitForSSMReady polls until the instance registers with SSM.
func WaitForSSMReady(ctx context.Context, ssmClient *ssm.Client, instanceID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		out, err := ssmClient.DescribeInstanceInformation(ctx, &ssm.DescribeInstanceInformationInput{
			Filters: []ssmtypes.InstanceInformationStringFilter{
				{
					Key:    aws.String("InstanceIds"),
					Values: []string{instanceID},
				},
			},
		})
		if err == nil && len(out.InstanceInformationList) > 0 {
			info := out.InstanceInformationList[0]
			if info.PingStatus == ssmtypes.PingStatusOnline {
				log.Debug().Str("instance", instanceID).Msg("SSM agent online")
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}

	return fmt.Errorf("timed out waiting for SSM agent on %s", instanceID)
}

func int32Ptr(v int32) *int32 { return &v }

// GetSSMSessionTarget returns a formatted SSM session target string.
func GetSSMSessionTarget(instanceID string) string {
	return instanceID
}

// IsSSMAvailable checks if an instance has an online SSM agent.
func IsSSMAvailable(ctx context.Context, ssmClient *ssm.Client, instanceID string) bool {
	out, err := ssmClient.DescribeInstanceInformation(ctx, &ssm.DescribeInstanceInformationInput{
		Filters: []ssmtypes.InstanceInformationStringFilter{
			{
				Key:    aws.String("InstanceIds"),
				Values: []string{instanceID},
			},
		},
	})
	if err != nil || len(out.InstanceInformationList) == 0 {
		return false
	}
	return out.InstanceInformationList[0].PingStatus == ssmtypes.PingStatusOnline
}

// buildSSMConnectionExtra returns extra connection info for SSM-capable instances.
func buildSSMConnectionExtra(instanceID, region string) map[string]string {
	return map[string]string{
		"ssm_target":  instanceID,
		"region":      region,
		"session_cmd": strings.Join([]string{"aws", "ssm", "start-session", "--target", instanceID, "--region", region}, " "),
	}
}
