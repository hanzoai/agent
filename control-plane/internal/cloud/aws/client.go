package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ssm"

	cloudcfg "github.com/hanzoai/agents/control-plane/internal/cloud"
)

// Clients holds initialized AWS service clients.
type Clients struct {
	EC2 *ec2.Client
	SSM *ssm.Client
}

// NewClients creates AWS SDK v2 clients for the configured region.
func NewClients(ctx context.Context, cfg cloudcfg.AWSConfig) (*Clients, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &Clients{
		EC2: ec2.NewFromConfig(awsCfg),
		SSM: ssm.NewFromConfig(awsCfg),
	}, nil
}
