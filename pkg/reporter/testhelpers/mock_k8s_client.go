package testhelpers

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-hyperfleet/status-reporter/pkg/k8s"
)

// MockK8sClient is a mock implementation of k8s client operations for testing
type MockK8sClient struct {
	UpdateJobStatusFunc           func(ctx context.Context, condition k8s.JobCondition) error
	GetAdapterContainerStatusFunc func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error)
	LastUpdatedCondition          k8s.JobCondition
}

func NewMockK8sClient() *MockK8sClient {
	return &MockK8sClient{}
}

func (m *MockK8sClient) UpdateJobStatus(ctx context.Context, condition k8s.JobCondition) error {
	m.LastUpdatedCondition = condition
	if m.UpdateJobStatusFunc != nil {
		return m.UpdateJobStatusFunc(ctx, condition)
	}
	return nil
}

func (m *MockK8sClient) GetAdapterContainerStatus(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
	if m.GetAdapterContainerStatusFunc != nil {
		return m.GetAdapterContainerStatusFunc(ctx, podName, containerName)
	}
	return nil, nil
}
