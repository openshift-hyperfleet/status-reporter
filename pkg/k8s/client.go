package k8s

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

const (
	// StatusReporterContainerName is the name of the status reporter sidecar container
	StatusReporterContainerName = "status-reporter"
)

// Client wraps Kubernetes client operations
type Client struct {
	clientset *kubernetes.Clientset
	namespace string
	jobName   string
}

// NewClient creates a new Kubernetes client using in-cluster config
func NewClient(namespace, jobName string) (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &Client{
		clientset: clientset,
		namespace: namespace,
		jobName:   jobName,
	}, nil
}

// JobCondition represents a Kubernetes Job condition
type JobCondition struct {
	Type               string
	Status             string
	Reason             string
	Message            string
	LastTransitionTime time.Time
}

// UpdateJobStatus updates the Job status with the given condition
// Note: RetryOnConflict only retries on conflict errors; NotFound and other errors return immediately
func (c *Client) UpdateJobStatus(ctx context.Context, condition JobCondition) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Basic input validation to avoid creating invalid JobStatus objects.
		switch corev1.ConditionStatus(condition.Status) {
		case corev1.ConditionTrue, corev1.ConditionFalse, corev1.ConditionUnknown:
		default:
			return fmt.Errorf("invalid condition status: %q (expected True/False/Unknown)", condition.Status)
		}

		// Fetch the latest job object to get current resourceVersion
		job, err := c.clientset.BatchV1().Jobs(c.namespace).Get(ctx, c.jobName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("job %s/%s not found: %w", c.namespace, c.jobName, err)
			}
			return err
		}

		transitionTime := condition.LastTransitionTime
		if transitionTime.IsZero() {
			transitionTime = time.Now()
		}

		newCondition := batchv1.JobCondition{
			Type:               batchv1.JobConditionType(condition.Type),
			Status:             corev1.ConditionStatus(condition.Status),
			LastTransitionTime: metav1.NewTime(transitionTime),
			Reason:             condition.Reason,
			Message:            condition.Message,
		}

		conditionUpdated := false
		for i, existing := range job.Status.Conditions {
			if existing.Type != newCondition.Type {
				continue
			}
			// No-op if semantically identical; preserves LastTransitionTime.
			if existing.Status == newCondition.Status && existing.Reason == newCondition.Reason && existing.Message == newCondition.Message {
				return nil
			}
			job.Status.Conditions[i] = newCondition
			conditionUpdated = true
			break
		}

		if !conditionUpdated {
			job.Status.Conditions = append(job.Status.Conditions, newCondition)
		}

		_, err = c.clientset.BatchV1().Jobs(c.namespace).UpdateStatus(ctx, job, metav1.UpdateOptions{})
		return err
	})
}

// GetPodStatus retrieves pod status by name
func (c *Client) GetPodStatus(ctx context.Context, podName string) (*corev1.PodStatus, error) {
	pod, err := c.clientset.CoreV1().Pods(c.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: namespace=%s pod=%s: %w", c.namespace, podName, err)
	}

	return &pod.Status, nil
}

// GetAdapterContainerStatus finds the adapter container status
func (c *Client) GetAdapterContainerStatus(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
	podStatus, err := c.GetPodStatus(ctx, podName)
	if err != nil {
		return nil, err
	}

	if containerName != "" {
		for _, cs := range podStatus.ContainerStatuses {
			if cs.Name == containerName {
				return &cs, nil
			}
		}
		return nil, fmt.Errorf("container not found: namespace=%s pod=%s container=%s", c.namespace, podName, containerName)
	}

	for _, cs := range podStatus.ContainerStatuses {
		if cs.Name != StatusReporterContainerName {
			return &cs, nil
		}
	}

	return nil, fmt.Errorf("adapter container not found: namespace=%s pod=%s", c.namespace, podName)
}
