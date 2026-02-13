package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/agents/control-plane/internal/cloud"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// Provisioner implements cloud.CloudProvisioner for Kubernetes pods.
type Provisioner struct {
	client    *kubernetes.Clientset
	config    cloud.K8sConfig
	restCfg   *restclient.Config
	serverURL string // control plane URL for agent registration
	apiKey    string // API key for agent auth
}

// NewProvisioner creates a new K8s provisioner.
func NewProvisioner(cfg cloud.K8sConfig, serverURL, apiKey string) (*Provisioner, error) {
	clientset, err := NewClient()
	if err != nil {
		return nil, err
	}

	// Get rest config for exec
	restCfg, err := restclient.InClusterConfig()
	if err != nil {
		restCfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, fmt.Errorf("failed to get rest config: %w", err)
		}
	}

	return &Provisioner{
		client:    clientset,
		config:    cfg,
		restCfg:   restCfg,
		serverURL: serverURL,
		apiKey:    apiKey,
	}, nil
}

func (p *Provisioner) ProviderName() string { return "k8s" }

// CreateInstance creates a pod in the configured namespace.
func (p *Provisioner) CreateInstance(ctx context.Context, req *cloud.ProvisionRequest) (*cloud.CloudInstance, error) {
	instanceID := uuid.New().String()
	podName := fmt.Sprintf("bot-%s", instanceID[:8])

	image := p.config.DefaultImage
	if req.InstanceType != "" {
		image = req.InstanceType // allow image override via instance_type
	}

	labels := map[string]string{
		"app":                       "hanzo-agent-bot",
		"hanzo.ai/cloud-instance":   instanceID,
		"hanzo.ai/team":             req.TeamID,
		"hanzo.ai/bot-package":      req.BotPackage,
	}
	for k, v := range req.Tags {
		labels["hanzo.ai/tag-"+k] = v
	}

	env := []corev1.EnvVar{
		{Name: "HANZO_AGENTS_SERVER_URL", Value: p.serverURL},
		{Name: "HANZO_AGENTS_API_KEY", Value: p.apiKey},
		{Name: "HANZO_AGENTS_INSTANCE_ID", Value: instanceID},
		{Name: "HANZO_AGENTS_BOT_PACKAGE", Value: req.BotPackage},
	}
	if req.BotVersion != "" {
		env = append(env, corev1.EnvVar{Name: "HANZO_AGENTS_BOT_VERSION", Value: req.BotVersion})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: p.config.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: p.config.ServiceAccount,
			RestartPolicy:      corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "agent",
					Image: image,
					Env:   env,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("250m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}

	created, err := p.client.CoreV1().Pods(p.config.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, &cloud.ProvisionError{
			InstanceID: instanceID,
			Platform:   cloud.PlatformLinux,
			Provider:   "k8s",
			Err:        err,
		}
	}

	log.Info().Str("pod", created.Name).Str("instance_id", instanceID).Msg("K8s pod created")

	now := time.Now().UTC()
	return &cloud.CloudInstance{
		ID:           instanceID,
		Platform:     cloud.PlatformLinux,
		State:        cloud.InstanceStateProvisioning,
		Provider:     "k8s",
		InstanceID:   created.Name,
		InstanceType: image,
		BotPackage:   req.BotPackage,
		BotVersion:   req.BotVersion,
		TeamID:       req.TeamID,
		RequestedAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// GetInstance returns the instance state by looking up the K8s pod.
func (p *Provisioner) GetInstance(ctx context.Context, instanceID string) (*cloud.CloudInstance, error) {
	pod, err := p.findPodByInstanceID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	state := podPhaseToState(pod.Status.Phase)
	podIP := pod.Status.PodIP

	return &cloud.CloudInstance{
		ID:         instanceID,
		Platform:   cloud.PlatformLinux,
		State:      state,
		Provider:   "k8s",
		InstanceID: pod.Name,
		PrivateIP:  podIP,
		TeamID:     pod.Labels["hanzo.ai/team"],
		BotPackage: pod.Labels["hanzo.ai/bot-package"],
	}, nil
}

// ListInstances returns pods matching the given filters.
func (p *Provisioner) ListInstances(ctx context.Context, filters cloud.InstanceFilters) ([]*cloud.CloudInstance, error) {
	labelSelector := "app=hanzo-agent-bot"
	if filters.TeamID != nil {
		labelSelector += ",hanzo.ai/team=" + *filters.TeamID
	}

	pods, err := p.client.CoreV1().Pods(p.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var instances []*cloud.CloudInstance
	for _, pod := range pods.Items {
		instanceID := pod.Labels["hanzo.ai/cloud-instance"]
		state := podPhaseToState(pod.Status.Phase)

		if filters.State != nil && state != *filters.State {
			continue
		}

		instances = append(instances, &cloud.CloudInstance{
			ID:         instanceID,
			Platform:   cloud.PlatformLinux,
			State:      state,
			Provider:   "k8s",
			InstanceID: pod.Name,
			PrivateIP:  pod.Status.PodIP,
			TeamID:     pod.Labels["hanzo.ai/team"],
			BotPackage: pod.Labels["hanzo.ai/bot-package"],
		})
	}

	return instances, nil
}

func (p *Provisioner) StartInstance(ctx context.Context, instanceID string) error {
	return fmt.Errorf("start not supported for K8s pods; create a new instance instead")
}

func (p *Provisioner) StopInstance(ctx context.Context, instanceID string) error {
	return p.TerminateInstance(ctx, instanceID)
}

// TerminateInstance deletes the pod.
func (p *Provisioner) TerminateInstance(ctx context.Context, instanceID string) error {
	pod, err := p.findPodByInstanceID(ctx, instanceID)
	if err != nil {
		return err
	}

	err = p.client.CoreV1().Pods(p.config.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %w", pod.Name, err)
	}

	log.Info().Str("pod", pod.Name).Str("instance_id", instanceID).Msg("K8s pod terminated")
	return nil
}

// GetConnectionInfo returns exec-based connection info.
func (p *Provisioner) GetConnectionInfo(ctx context.Context, instanceID string) (*cloud.ConnectionInfo, error) {
	pod, err := p.findPodByInstanceID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	return &cloud.ConnectionInfo{
		Protocol: cloud.ConnectionProtocolExec,
		Host:     pod.Status.PodIP,
		Extra: map[string]string{
			"pod_name":  pod.Name,
			"namespace": p.config.Namespace,
		},
	}, nil
}

// ExecuteCommand uses SPDY exec to run a command inside the pod.
func (p *Provisioner) ExecuteCommand(ctx context.Context, instanceID, command string) (*cloud.CommandResult, error) {
	pod, err := p.findPodByInstanceID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	req := p.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(p.config.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: []string{"sh", "-c", command},
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(p.restCfg, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	exitCode := 0
	if err != nil {
		exitCode = 1
	}

	return &cloud.CommandResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

// GetLogs returns logs from the pod.
func (p *Provisioner) GetLogs(ctx context.Context, instanceID string, lines int) (string, error) {
	pod, err := p.findPodByInstanceID(ctx, instanceID)
	if err != nil {
		return "", err
	}

	tailLines := int64(lines)
	logReq := p.client.CoreV1().Pods(p.config.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		TailLines: &tailLines,
	})

	stream, err := logReq.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer stream.Close()

	buf := new(strings.Builder)
	if _, err := io.Copy(buf, stream); err != nil {
		return "", fmt.Errorf("failed to read pod logs: %w", err)
	}

	return buf.String(), nil
}

// findPodByInstanceID finds a pod by the cloud instance ID label.
func (p *Provisioner) findPodByInstanceID(ctx context.Context, instanceID string) (*corev1.Pod, error) {
	pods, err := p.client.CoreV1().Pods(p.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("hanzo.ai/cloud-instance=%s", instanceID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find pod: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, cloud.ErrInstanceNotFound
	}

	return &pods.Items[0], nil
}

// podPhaseToState maps a K8s pod phase to an InstanceState.
func podPhaseToState(phase corev1.PodPhase) cloud.InstanceState {
	switch phase {
	case corev1.PodPending:
		return cloud.InstanceStateProvisioning
	case corev1.PodRunning:
		return cloud.InstanceStateRunning
	case corev1.PodSucceeded, corev1.PodFailed:
		return cloud.InstanceStateTerminated
	default:
		return cloud.InstanceStateFailed
	}
}
