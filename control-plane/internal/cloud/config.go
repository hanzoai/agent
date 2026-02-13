package cloud

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// CloudConfig holds the top-level cloud provisioning configuration.
type CloudConfig struct {
	Enabled bool      `yaml:"enabled" mapstructure:"enabled"`
	AWS     AWSConfig `yaml:"aws" mapstructure:"aws"`
	K8s     K8sConfig `yaml:"k8s" mapstructure:"k8s"`
	Billing BillingConfig `yaml:"billing" mapstructure:"billing"`

	// Safety limits
	MaxInstancesPerTeam int           `yaml:"max_instances_per_team" mapstructure:"max_instances_per_team"`
	ProvisioningTimeout time.Duration `yaml:"provisioning_timeout" mapstructure:"provisioning_timeout"`
	MonitorInterval     time.Duration `yaml:"monitor_interval" mapstructure:"monitor_interval"`
}

// AWSConfig holds AWS-specific provisioning configuration.
type AWSConfig struct {
	Enabled            bool   `yaml:"enabled" mapstructure:"enabled"`
	Region             string `yaml:"region" mapstructure:"region"`
	VPCID              string `yaml:"vpc_id" mapstructure:"vpc_id"`
	SubnetIDs          []string `yaml:"subnet_ids" mapstructure:"subnet_ids"`
	SecurityGroupID    string `yaml:"security_group_id" mapstructure:"security_group_id"`
	IAMInstanceProfile string `yaml:"iam_instance_profile" mapstructure:"iam_instance_profile"`

	MacOS   AWSMacOSConfig   `yaml:"macos" mapstructure:"macos"`
	Windows AWSWindowsConfig `yaml:"windows" mapstructure:"windows"`
}

// AWSMacOSConfig holds macOS-specific AWS configuration.
type AWSMacOSConfig struct {
	DedicatedHostIDs []string `yaml:"dedicated_host_ids" mapstructure:"dedicated_host_ids"`
	AMIID            string   `yaml:"ami_id" mapstructure:"ami_id"`
	InstanceType     string   `yaml:"instance_type" mapstructure:"instance_type"`
	MinHostAllocation time.Duration `yaml:"min_host_allocation" mapstructure:"min_host_allocation"`
	IdleHostRelease   time.Duration `yaml:"idle_host_release" mapstructure:"idle_host_release"`
}

// AWSWindowsConfig holds Windows-specific AWS configuration.
type AWSWindowsConfig struct {
	AMIID               string `yaml:"ami_id" mapstructure:"ami_id"`
	DefaultInstanceType string `yaml:"default_instance_type" mapstructure:"default_instance_type"`
}

// K8sConfig holds Kubernetes-specific provisioning configuration.
type K8sConfig struct {
	Enabled      bool   `yaml:"enabled" mapstructure:"enabled"`
	Namespace    string `yaml:"namespace" mapstructure:"namespace"`
	DefaultImage string `yaml:"default_image" mapstructure:"default_image"`
	// ServiceAccount to use for pods. Defaults to "default".
	ServiceAccount string `yaml:"service_account" mapstructure:"service_account"`
}

// Defaults fills in zero values with sensible defaults.
func (c *CloudConfig) Defaults() {
	if c.MaxInstancesPerTeam == 0 {
		c.MaxInstancesPerTeam = 10
	}
	if c.ProvisioningTimeout == 0 {
		c.ProvisioningTimeout = 10 * time.Minute
	}
	if c.MonitorInterval == 0 {
		c.MonitorInterval = 30 * time.Second
	}
	if c.AWS.Region == "" {
		c.AWS.Region = "us-east-1"
	}
	if c.AWS.MacOS.InstanceType == "" {
		c.AWS.MacOS.InstanceType = "mac2.metal"
	}
	if c.AWS.MacOS.MinHostAllocation == 0 {
		c.AWS.MacOS.MinHostAllocation = 24 * time.Hour
	}
	if c.AWS.MacOS.IdleHostRelease == 0 {
		c.AWS.MacOS.IdleHostRelease = 25 * time.Hour // 1 hour after 24h min
	}
	if c.AWS.Windows.DefaultInstanceType == "" {
		c.AWS.Windows.DefaultInstanceType = "t3.large"
	}
	if c.K8s.Namespace == "" {
		c.K8s.Namespace = "hanzo"
	}
	if c.K8s.DefaultImage == "" {
		c.K8s.DefaultImage = "ghcr.io/hanzoai/agent:sdk-latest"
	}
	if c.K8s.ServiceAccount == "" {
		c.K8s.ServiceAccount = "default"
	}
}

// ApplyEnvOverrides applies environment variable overrides to the cloud config.
func (c *CloudConfig) ApplyEnvOverrides() {
	if v := os.Getenv("HANZO_AGENTS_CLOUD_ENABLED"); v != "" {
		c.Enabled = v == "true" || v == "1"
	}

	// AWS overrides
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_ENABLED"); v != "" {
		c.AWS.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AWS_REGION"); v != "" {
		c.AWS.Region = v
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_VPC_ID"); v != "" {
		c.AWS.VPCID = v
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_SUBNET_IDS"); v != "" {
		c.AWS.SubnetIDs = strings.Split(v, ",")
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_SECURITY_GROUP_ID"); v != "" {
		c.AWS.SecurityGroupID = v
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_IAM_INSTANCE_PROFILE"); v != "" {
		c.AWS.IAMInstanceProfile = v
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_MACOS_AMI"); v != "" {
		c.AWS.MacOS.AMIID = v
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_MACOS_HOST_IDS"); v != "" {
		c.AWS.MacOS.DedicatedHostIDs = strings.Split(v, ",")
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_WINDOWS_AMI"); v != "" {
		c.AWS.Windows.AMIID = v
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_AWS_WINDOWS_INSTANCE_TYPE"); v != "" {
		c.AWS.Windows.DefaultInstanceType = v
	}

	// K8s overrides
	if v := os.Getenv("HANZO_AGENTS_CLOUD_K8S_ENABLED"); v != "" {
		c.K8s.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_K8S_NAMESPACE"); v != "" {
		c.K8s.Namespace = v
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_K8S_DEFAULT_IMAGE"); v != "" {
		c.K8s.DefaultImage = v
	}

	// Limits
	if v := os.Getenv("HANZO_AGENTS_CLOUD_MAX_INSTANCES_PER_TEAM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxInstancesPerTeam = n
		}
	}

	// Billing
	if v := os.Getenv("HANZO_AGENTS_CLOUD_BILLING_ENABLED"); v != "" {
		c.Billing.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_BILLING_URL"); v != "" {
		c.Billing.ServiceURL = v
	}
	if v := os.Getenv("HANZO_AGENTS_CLOUD_BILLING_API_KEY"); v != "" {
		c.Billing.APIKey = v
	}
}
