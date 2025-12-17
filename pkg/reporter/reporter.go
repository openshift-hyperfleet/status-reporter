package reporter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-hyperfleet/status-reporter/pkg/k8s"
	"github.com/openshift-hyperfleet/status-reporter/pkg/result"
)

const (
	ConditionStatusTrue  = "True"
	ConditionStatusFalse = "False"

	ReasonAdapterCrashed         = "AdapterCrashed"
	ReasonAdapterOOMKilled       = "AdapterOOMKilled"
	ReasonAdapterExitedWithError = "AdapterExitedWithError"
	ReasonAdapterTimeout         = "AdapterTimeout"
	ReasonInvalidResultFormat    = "InvalidResultFormat"
	ReasonAdapterMissingResults  = "AdapterMissingResults"

	ContainerReasonOOMKilled = "OOMKilled"

	// DefaultContainerStatusCheckInterval Default container status check interval - checked less frequently than file polling to reduce a K8s API load
	DefaultContainerStatusCheckInterval = 10 * time.Second
)

// K8sClientInterface defines the k8s operations needed by StatusReporter
type K8sClientInterface interface {
	UpdateJobStatus(ctx context.Context, condition k8s.JobCondition) error
	GetAdapterContainerStatus(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error)
}

// pollChannels encapsulates the channels used for communication between polling goroutines and the main Run loop
type pollChannels struct {
	result     chan *result.AdapterResult
	error      chan error
	terminated chan *corev1.ContainerStateTerminated
	done       chan struct{}
}

// StatusReporter is the main status reporter
type StatusReporter struct {
	resultsPath                  string
	pollInterval                 time.Duration
	maxWaitTime                  time.Duration
	containerStatusCheckInterval time.Duration
	conditionType                string
	podName                      string
	adapterContainerName         string
	k8sClient                    K8sClientInterface
	parser                       *result.Parser
}

// NewReporter creates a new status reporter
func NewReporter(resultsPath string, pollInterval, maxWaitTime time.Duration, conditionType, podName, adapterContainerName, jobName, jobNamespace string) (*StatusReporter, error) {
	k8sClient, err := k8s.NewClient(jobNamespace, jobName)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	return newReporterWithClient(resultsPath, pollInterval, maxWaitTime, DefaultContainerStatusCheckInterval, conditionType, podName, adapterContainerName, k8sClient), nil
}

// NewReporterWithClient creates a new status reporter with a custom k8s client (for testing)
func NewReporterWithClient(resultsPath string, pollInterval, maxWaitTime time.Duration, conditionType, podName, adapterContainerName string, k8sClient K8sClientInterface) *StatusReporter {
	return newReporterWithClient(resultsPath, pollInterval, maxWaitTime, DefaultContainerStatusCheckInterval, conditionType, podName, adapterContainerName, k8sClient)
}

// NewReporterWithClientAndIntervals creates a new status reporter with custom intervals (for testing)
func NewReporterWithClientAndIntervals(resultsPath string, pollInterval, maxWaitTime, containerStatusCheckInterval time.Duration, conditionType, podName, adapterContainerName string, k8sClient K8sClientInterface) *StatusReporter {
	return newReporterWithClient(resultsPath, pollInterval, maxWaitTime, containerStatusCheckInterval, conditionType, podName, adapterContainerName, k8sClient)
}

func newReporterWithClient(resultsPath string, pollInterval, maxWaitTime, containerStatusCheckInterval time.Duration, conditionType, podName, adapterContainerName string, k8sClient K8sClientInterface) *StatusReporter {
	return &StatusReporter{
		resultsPath:                  resultsPath,
		pollInterval:                 pollInterval,
		maxWaitTime:                  maxWaitTime,
		containerStatusCheckInterval: containerStatusCheckInterval,
		conditionType:                conditionType,
		podName:                      podName,
		adapterContainerName:         adapterContainerName,
		k8sClient:                    k8sClient,
		parser:                       result.NewParser(),
	}
}

// Run starts the reporter and blocks until completion
func (r *StatusReporter) Run(ctx context.Context) error {
	log.Printf("Status reporter starting...")
	log.Printf("  Pod: %s", r.podName)
	log.Printf("  Results path: %s", r.resultsPath)
	log.Printf("  Poll interval: %s", r.pollInterval)
	log.Printf("  Max wait time: %s", r.maxWaitTime)

	timeoutCtx, cancel := context.WithTimeout(ctx, r.maxWaitTime)
	defer cancel()

	// Buffered channels (size 1) prevent goroutine leaks if the main select has already
	// chosen another case when a sender tries to send
	channels := &pollChannels{
		result:     make(chan *result.AdapterResult, 1),
		error:      make(chan error, 1),
		terminated: make(chan *corev1.ContainerStateTerminated, 1),
		done:       make(chan struct{}),
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go r.pollForResultFile(timeoutCtx, channels, &wg)
	go r.monitorContainerStatus(timeoutCtx, channels, &wg)

	var reportErr error
	select {
	case adapterResult := <-channels.result:
		reportErr = r.UpdateFromResult(ctx, adapterResult)
	case err := <-channels.error:
		reportErr = r.UpdateFromError(ctx, err)
	case terminated := <-channels.terminated:
		reportErr = r.HandleTermination(ctx, terminated)
	case <-timeoutCtx.Done():
		// Give precedence to results/errors/termination that may have arrived just before timeout
		select {
		case adapterResult := <-channels.result:
			reportErr = r.UpdateFromResult(ctx, adapterResult)
		case err := <-channels.error:
			reportErr = r.UpdateFromError(ctx, err)
		case terminated := <-channels.terminated:
			reportErr = r.HandleTermination(ctx, terminated)
		default:
			reportErr = r.UpdateFromTimeout(ctx)
		}
	}

	close(channels.done)
	wg.Wait()

	return reportErr
}

// pollForResultFile polls for the result file at regular intervals.
// This is separated from container monitoring to allow fast polling of the local filesystem
// without incurring the cost of K8s API calls on every iteration.
func (r *StatusReporter) pollForResultFile(ctx context.Context, channels *pollChannels, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	log.Printf("Polling for result file at %s (interval: %s)...", r.resultsPath, r.pollInterval)

	for {
		select {
		case <-channels.done:
			log.Printf("Result file polling stopped by shutdown signal")
			return
		case <-ctx.Done():
			log.Printf("Result file polling cancelled: %v", ctx.Err())
			return
		case <-ticker.C:
			// Check for result file (fast local filesystem operation)
			if _, err := os.Stat(r.resultsPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				// Unexpected stat error (e.g., permission denied)
				select {
				case channels.error <- fmt.Errorf("failed to stat result file path=%s: %w", r.resultsPath, err):
				case <-channels.done:
					return
				}
				return
			}

			log.Printf("Result file found, parsing...")
			adapterResult, err := r.parser.ParseFile(r.resultsPath)
			if err != nil {
				select {
				case channels.error <- err:
				case <-channels.done:
					return
				}
				return
			}

			log.Printf("Result parsed successfully: status=%s, reason=%s", adapterResult.Status, adapterResult.Reason)
			select {
			case channels.result <- adapterResult:
			case <-channels.done:
				return
			}
			return
		}
	}
}

// checkContainerStatus checks if the adapter container has terminated.
// Returns true if terminated (and sends notification), false otherwise.
func (r *StatusReporter) checkContainerStatus(ctx context.Context, channels *pollChannels) bool {
	containerStatus, err := r.k8sClient.GetAdapterContainerStatus(ctx, r.podName, r.adapterContainerName)
	if err != nil {
		log.Printf("Warning: failed to get container status pod=%s container=%s: %v",
			r.podName, r.adapterContainerName, err)
		return false
	}

	if containerStatus != nil && containerStatus.State.Terminated != nil {
		log.Printf("Container terminated: pod=%s container=%s reason=%s exitCode=%d",
			r.podName, r.adapterContainerName,
			containerStatus.State.Terminated.Reason,
			containerStatus.State.Terminated.ExitCode)
		select {
		case channels.terminated <- containerStatus.State.Terminated:
		case <-channels.done:
		}
		return true
	}
	return false
}

// monitorContainerStatus monitors the adapter container status at regular intervals.
// This is separated from file polling to reduce K8s API load - we check container status
// less frequently (every 10s by default) compared to file polling (typically 50-100ms).
func (r *StatusReporter) monitorContainerStatus(ctx context.Context, channels *pollChannels, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("Monitoring container status for pod=%s container=%s (interval: %s)...",
		r.podName, r.adapterContainerName, r.containerStatusCheckInterval)

	// Perform immediate check before starting ticker
	if r.checkContainerStatus(ctx, channels) {
		return
	}

	ticker := time.NewTicker(r.containerStatusCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-channels.done:
			log.Printf("Container status monitoring stopped by shutdown signal")
			return
		case <-ctx.Done():
			log.Printf("Container status monitoring cancelled: %v", ctx.Err())
			return
		case <-ticker.C:
			if r.checkContainerStatus(ctx, channels) {
				return
			}
		}
	}
}

// HandleTermination handles container termination by checking for result file first.
// Priority order:
// 1. If valid result file exists -> use it (adapter's intended status)
// 2. If result file missing or invalid -> use container exit code
func (r *StatusReporter) HandleTermination(ctx context.Context, terminated *corev1.ContainerStateTerminated) error {
	log.Printf("Adapter container terminated: reason=%s, exitCode=%d", terminated.Reason, terminated.ExitCode)

	adapterResult, err := r.tryParseResultFile()
	switch {
	case err == nil && adapterResult != nil:
		// Happy path: valid result file exists
		log.Printf("Using result file: status=%s, reason=%s", adapterResult.Status, adapterResult.Reason)
		return r.UpdateFromResult(ctx, adapterResult)

	case errors.Is(err, os.ErrNotExist):
		// Expected: adapter terminated without producing result file
		log.Printf("No result file found, using container exit code")

	case err != nil:
		// Unexpected: file exists but can't read/parse it
		log.Printf("Warning: result file error: %v. Falling back to container exit code", err)
	}

	// No valid result file, update based on container termination state
	return r.UpdateFromTerminatedContainer(ctx, terminated)
}

// tryParseResultFile attempts to read and parse the result file.
// Returns (nil, os.ErrNotExist) if file doesn't exist, or (nil, err) for other errors.
func (r *StatusReporter) tryParseResultFile() (*result.AdapterResult, error) {
	if _, err := os.Stat(r.resultsPath); err != nil {
		return nil, err // Could be ErrNotExist or permission error
	}

	adapterResult, err := r.parser.ParseFile(r.resultsPath)
	if err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	return adapterResult, nil
}

// UpdateFromResult updates Job status from adapter result
func (r *StatusReporter) UpdateFromResult(ctx context.Context, adapterResult *result.AdapterResult) error {
	log.Printf("Updating Job status from adapter result...")

	conditionStatus := ConditionStatusTrue
	if !adapterResult.IsSuccess() {
		conditionStatus = ConditionStatusFalse
	}

	condition := k8s.JobCondition{
		Type:    r.conditionType,
		Status:  conditionStatus,
		Reason:  adapterResult.Reason,
		Message: adapterResult.Message,
	}

	if err := r.k8sClient.UpdateJobStatus(ctx, condition); err != nil {
		return fmt.Errorf("failed to update job status: pod=%s condition=%s: %w", r.podName, r.conditionType, err)
	}

	log.Printf("Job status updated successfully: %s=%s (reason: %s)", r.conditionType, conditionStatus, adapterResult.Reason)
	return nil
}

// UpdateFromError updates Job status when parsing fails
func (r *StatusReporter) UpdateFromError(ctx context.Context, err error) error {
	log.Printf("Failed to parse result file: %v", err)

	condition := k8s.JobCondition{
		Type:    r.conditionType,
		Status:  ConditionStatusFalse,
		Reason:  ReasonInvalidResultFormat,
		Message: fmt.Sprintf("Failed to parse adapter result: %v", err),
	}

	if updateErr := r.k8sClient.UpdateJobStatus(ctx, condition); updateErr != nil {
		return fmt.Errorf("failed to update job status: %w", updateErr)
	}

	log.Printf("Job status updated: %s=False (reason: %s)", r.conditionType, ReasonInvalidResultFormat)
	return err
}

// UpdateFromTimeout updates Job status when timeout occurs.
// As a last attempt, checks if container has terminated to provide more specific error info.
func (r *StatusReporter) UpdateFromTimeout(ctx context.Context) error {
	log.Printf("Timeout waiting for adapter results (max wait: %s)", r.maxWaitTime)
	log.Printf("Checking adapter container status: pod=%s container=%s", r.podName, r.adapterContainerName)

	containerStatus, err := r.k8sClient.GetAdapterContainerStatus(ctx, r.podName, r.adapterContainerName)
	if err != nil {
		log.Printf("Warning: failed to get container status pod=%s container=%s: %v",
			r.podName, r.adapterContainerName, err)
	} else if containerStatus != nil && containerStatus.State.Terminated != nil {
		return r.UpdateFromTerminatedContainer(ctx, containerStatus.State.Terminated)
	}

	condition := k8s.JobCondition{
		Type:    r.conditionType,
		Status:  ConditionStatusFalse,
		Reason:  ReasonAdapterTimeout,
		Message: fmt.Sprintf("Adapter did not produce results within %s", r.maxWaitTime),
	}

	if err := r.k8sClient.UpdateJobStatus(ctx, condition); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	log.Printf("Job status updated: %s=False (reason: %s)", r.conditionType, ReasonAdapterTimeout)
	return errors.New("timeout waiting for adapter results")
}

// UpdateFromTerminatedContainer updates Job status from container termination state
func (r *StatusReporter) UpdateFromTerminatedContainer(ctx context.Context, terminated *corev1.ContainerStateTerminated) error {
	var reason, message string

	if terminated.Reason == ContainerReasonOOMKilled {
		reason = ReasonAdapterOOMKilled
		message = "Adapter container was killed due to out of memory (OOMKilled)"
	} else if terminated.ExitCode != 0 {
		reason = ReasonAdapterExitedWithError
		message = fmt.Sprintf("Adapter container exited with code %d: %s", terminated.ExitCode, terminated.Reason)
	} else {
		reason = ReasonAdapterMissingResults
		message = fmt.Sprintf("Adapter container exited successfully (code 0) but did not produce a valid result file: %s", terminated.Reason)
	}

	log.Printf("Adapter container terminated: reason=%s, exitCode=%d", terminated.Reason, terminated.ExitCode)

	condition := k8s.JobCondition{
		Type:    r.conditionType,
		Status:  ConditionStatusFalse,
		Reason:  reason,
		Message: message,
	}

	if err := r.k8sClient.UpdateJobStatus(ctx, condition); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	log.Printf("Job status updated: %s=False (reason: %s)", r.conditionType, reason)
	return fmt.Errorf("adapter container terminated: %s", message)
}
