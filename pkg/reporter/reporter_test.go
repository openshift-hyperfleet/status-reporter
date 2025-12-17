package reporter_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-hyperfleet/status-reporter/pkg/k8s"
	"github.com/openshift-hyperfleet/status-reporter/pkg/reporter"
	"github.com/openshift-hyperfleet/status-reporter/pkg/reporter/testhelpers"
	"github.com/openshift-hyperfleet/status-reporter/pkg/result"
)

var _ = Describe("Reporter", func() {
	var (
		r    *reporter.StatusReporter
		mock *testhelpers.MockK8sClient
		ctx  context.Context
	)

	BeforeEach(func() {
		mock = testhelpers.NewMockK8sClient()
		ctx = context.Background()
		r = reporter.NewReporterWithClient(
			"/results/test.json",
			2*time.Second,
			300*time.Second,
			"Available",
			"test-pod",
			"adapter",
			mock,
		)
	})

	Describe("Constants", func() {
		It("exports expected constant values", func() {
			Expect(reporter.ConditionStatusTrue).To(Equal("True"))
			Expect(reporter.ConditionStatusFalse).To(Equal("False"))
			Expect(reporter.ReasonAdapterCrashed).To(Equal("AdapterCrashed"))
			Expect(reporter.ReasonAdapterOOMKilled).To(Equal("AdapterOOMKilled"))
			Expect(reporter.ReasonAdapterExitedWithError).To(Equal("AdapterExitedWithError"))
			Expect(reporter.ReasonAdapterTimeout).To(Equal("AdapterTimeout"))
			Expect(reporter.ReasonInvalidResultFormat).To(Equal("InvalidResultFormat"))
		})
	})

	Describe("reporter.NewReporterWithClient", func() {
		It("creates a reporter with custom condition type", func() {
			customRep := reporter.NewReporterWithClient(
				"/results/test.json",
				2*time.Second,
				300*time.Second,
				"Ready",
				"test-pod",
				"adapter",
				mock,
			)
			Expect(customRep).NotTo(BeNil())
		})

		It("uses default condition type when empty", func() {
			customRep := reporter.NewReporterWithClient(
				"/results/test.json",
				2*time.Second,
				300*time.Second,
				"",
				"test-pod",
				"adapter",
				mock,
			)
			Expect(customRep).NotTo(BeNil())
		})
	})

	Describe("updateFromResult", func() {
		Context("with successful adapter result", func() {
			It("updates job status to True", func() {
				adapterResult := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "ValidationPassed",
					Message: "All validations passed",
				}

				err := r.UpdateFromResult(ctx, adapterResult)

				Expect(err).NotTo(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("True"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal("ValidationPassed"))
				Expect(mock.LastUpdatedCondition.Message).To(Equal("All validations passed"))
			})
		})

		Context("with failed adapter result", func() {
			It("updates job status to False", func() {
				adapterResult := &result.AdapterResult{
					Status:  result.StatusFailure,
					Reason:  "ValidationFailed",
					Message: "Some validations failed",
				}

				err := r.UpdateFromResult(ctx, adapterResult)

				Expect(err).NotTo(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal("ValidationFailed"))
				Expect(mock.LastUpdatedCondition.Message).To(Equal("Some validations failed"))
			})
		})

		Context("when k8s client returns error", func() {
			It("returns the error", func() {
				mock.UpdateJobStatusFunc = func(ctx context.Context, condition k8s.JobCondition) error {
					return errors.New("k8s update failed")
				}

				adapterResult := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "ValidationPassed",
					Message: "All validations passed",
				}

				err := r.UpdateFromResult(ctx, adapterResult)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update job status"))
				Expect(err.Error()).To(ContainSubstring("k8s update failed"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("True"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal("ValidationPassed"))
				Expect(mock.LastUpdatedCondition.Message).To(Equal("All validations passed"))
			})
		})

		Context("with custom condition type", func() {
			It("uses the custom condition type", func() {
				customRep := reporter.NewReporterWithClient(
					"/results/test.json",
					2*time.Second,
					300*time.Second,
					"Ready",
					"test-pod",
					"adapter",
					mock,
				)

				adapterResult := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "ValidationPassed",
					Message: "All validations passed",
				}

				err := customRep.UpdateFromResult(ctx, adapterResult)

				Expect(err).NotTo(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Ready"))
			})
		})
	})

	Describe("updateFromError", func() {
		It("updates job status with InvalidResultFormat reason", func() {
			parseErr := errors.New("JSON parsing failed")

			err := r.UpdateFromError(ctx, parseErr)

			Expect(err).To(Equal(parseErr))
			Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
			Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
			Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonInvalidResultFormat))
			Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("Failed to parse adapter result"))
			Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("JSON parsing failed"))
		})

		It("returns error when k8s client fails", func() {
			mock.UpdateJobStatusFunc = func(ctx context.Context, condition k8s.JobCondition) error {
				return errors.New("k8s update failed")
			}

			parseErr := errors.New("JSON parsing failed")
			err := r.UpdateFromError(ctx, parseErr)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update job status"))
			Expect(err.Error()).To(ContainSubstring("k8s update failed"))
			Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
			Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
			Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonInvalidResultFormat))
		})
	})

	Describe("handleTermination", func() {
		var (
			tempDir     string
			resultsPath string
		)

		BeforeEach(func() {
			tempDir = GinkgoT().TempDir()
			resultsPath = filepath.Join(tempDir, "adapter-result.json")
			r = reporter.NewReporterWithClient(
				resultsPath,
				2*time.Second,
				300*time.Second,
				"Available",
				"test-pod",
				"adapter",
				mock,
			)
		})

		Context("when result file exists and is valid", func() {
			It("uses result file instead of exit code", func() {
				// Write a valid result file
				err := os.WriteFile(resultsPath, []byte(`{"status":"success","reason":"AllChecksPassed","message":"All validations passed"}`), 0644)
				Expect(err).NotTo(HaveOccurred())

				terminated := &corev1.ContainerStateTerminated{
					Reason:   "Error",
					ExitCode: 1,
				}

				err = r.HandleTermination(ctx, terminated)

				Expect(err).NotTo(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("True"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal("AllChecksPassed"))
				Expect(mock.LastUpdatedCondition.Message).To(Equal("All validations passed"))
			})
		})

		Context("when result file exists but is invalid", func() {
			It("falls back to using exit code", func() {
				// Write an invalid result file
				err := os.WriteFile(resultsPath, []byte(`{invalid json`), 0644)
				Expect(err).NotTo(HaveOccurred())

				terminated := &corev1.ContainerStateTerminated{
					Reason:   "Error",
					ExitCode: 1,
				}

				err = r.HandleTermination(ctx, terminated)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterExitedWithError))
				Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("Adapter container exited with code 1"))
			})
		})

		Context("when result file does not exist", func() {
			It("uses exit code to determine status", func() {
				terminated := &corev1.ContainerStateTerminated{
					Reason:   "Error",
					ExitCode: 1,
				}

				err := r.HandleTermination(ctx, terminated)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterExitedWithError))
				Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("Adapter container exited with code 1"))
			})
		})

		Context("when container was OOMKilled", func() {
			It("uses OOMKilled reason when no result file", func() {
				terminated := &corev1.ContainerStateTerminated{
					Reason:   "OOMKilled",
					ExitCode: 137,
				}

				err := r.HandleTermination(ctx, terminated)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterOOMKilled))
				Expect(mock.LastUpdatedCondition.Message).To(Equal("Adapter container was killed due to out of memory (OOMKilled)"))
			})
		})
	})

	Describe("updateFromTerminatedContainer", func() {
		Context("when container was OOMKilled", func() {
			It("updates with AdapterOOMKilled reason", func() {
				terminated := &corev1.ContainerStateTerminated{
					Reason:   "OOMKilled",
					ExitCode: 137,
				}

				err := r.UpdateFromTerminatedContainer(ctx, terminated)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterOOMKilled))
				Expect(mock.LastUpdatedCondition.Message).To(Equal("Adapter container was killed due to out of memory (OOMKilled)"))
			})
		})

		Context("when container exited with non-zero code", func() {
			It("updates with AdapterExitedWithError reason", func() {
				terminated := &corev1.ContainerStateTerminated{
					Reason:   "Error",
					ExitCode: 1,
				}

				err := r.UpdateFromTerminatedContainer(ctx, terminated)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterExitedWithError))
				Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("Adapter container exited with code 1"))
				Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("Error"))
			})
		})

		Context("when container exited with zero code", func() {
			// This test case is valid because updateFromTerminatedContainer is only called
			// when we've reached the timeout path (no result file was produced).
			// If the container exited with code 0 but didn't produce the result file,
			// this indicates a bug in the adapter logic - it should have either:
			// 1. Written the result file and exited 0, OR
			// 2. Failed to write the file and exited non-zero
			// Therefore, we mark it as AdapterMissingResults to clearly indicate the issue.
			It("updates with AdapterMissingResults reason when exit code 0 but no result file was produced", func() {
				terminated := &corev1.ContainerStateTerminated{
					Reason:   "Completed",
					ExitCode: 0,
				}

				err := r.UpdateFromTerminatedContainer(ctx, terminated)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterMissingResults))
				Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("exited successfully (code 0)"))
				Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("did not produce a valid result file"))
			})
		})

		Context("when k8s client returns error", func() {
			It("returns the error", func() {
				mock.UpdateJobStatusFunc = func(ctx context.Context, condition k8s.JobCondition) error {
					return errors.New("k8s update failed")
				}

				terminated := &corev1.ContainerStateTerminated{
					Reason:   "OOMKilled",
					ExitCode: 137,
				}

				err := r.UpdateFromTerminatedContainer(ctx, terminated)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update job status"))
				Expect(err.Error()).To(ContainSubstring("k8s update failed"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterOOMKilled))
			})
		})
	})

	Describe("updateFromTimeout", func() {
		Context("when adapter container is terminated with OOMKilled", func() {
			It("updates with AdapterOOMKilled reason", func() {
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:   "OOMKilled",
								ExitCode: 137,
							},
						},
					}, nil
				}

				err := r.UpdateFromTimeout(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterOOMKilled))
			})
		})

		Context("when adapter container is terminated with error", func() {
			It("updates with AdapterExitedWithError reason", func() {
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:   "Error",
								ExitCode: 1,
							},
						},
					}, nil
				}

				err := r.UpdateFromTimeout(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterExitedWithError))
			})
		})

		Context("when adapter container is not terminated", func() {
			It("updates with AdapterTimeout reason", func() {
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					}, nil
				}

				err := r.UpdateFromTimeout(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("timeout waiting for adapter results"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterTimeout))
				Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("Adapter did not produce results within"))
			})
		})

		Context("when getting container status fails", func() {
			It("still updates with AdapterTimeout reason", func() {
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					return nil, errors.New("failed to get container status")
				}

				err := r.UpdateFromTimeout(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("timeout waiting for adapter results"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterTimeout))
			})
		})

		Context("when k8s client update fails", func() {
			It("returns the error", func() {
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					}, nil
				}

				mock.UpdateJobStatusFunc = func(ctx context.Context, condition k8s.JobCondition) error {
					return errors.New("k8s update failed")
				}

				err := r.UpdateFromTimeout(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update job status"))
				Expect(err.Error()).To(ContainSubstring("k8s update failed"))
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterTimeout))
			})
		})
	})

	Describe("Run", func() {
		var (
			tempDir     string
			resultsPath string
		)

		BeforeEach(func() {
			tempDir = GinkgoT().TempDir()
			resultsPath = filepath.Join(tempDir, "adapter-result.json")
		})

		Context("when result file exists immediately", func() {
			It("processes the result successfully", func() {
				// Write result file before starting
				err := os.WriteFile(resultsPath, []byte(`{"status":"success","reason":"AllChecksPassed","message":"All validations passed"}`), 0644)
				Expect(err).NotTo(HaveOccurred())

				r := reporter.NewReporterWithClient(
					resultsPath,
					100*time.Millisecond,
					5*time.Second,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err = r.Run(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("True"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal("AllChecksPassed"))
			})
		})

		Context("when result file appears after polling", func() {
			It("processes the result successfully", func() {
				r := reporter.NewReporterWithClient(
					resultsPath,
					50*time.Millisecond,
					5*time.Second,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				// Write file after a short delay
				go func() {
					time.Sleep(150 * time.Millisecond)
					_ = os.WriteFile(resultsPath, []byte(`{"status":"failure","reason":"ValidationFailed","message":"Some checks failed"}`), 0644)
				}()

				err := r.Run(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal("ValidationFailed"))
			})
		})

		Context("when timeout occurs without result file", func() {
			It("reports timeout error", func() {
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					}, nil
				}

				r := reporter.NewReporterWithClient(
					resultsPath,
					50*time.Millisecond,
					200*time.Millisecond,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err := r.Run(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("timeout waiting for adapter results"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterTimeout))
			})
		})

		Context("when result file has invalid JSON", func() {
			It("reports parse error", func() {
				err := os.WriteFile(resultsPath, []byte(`{invalid json`), 0644)
				Expect(err).NotTo(HaveOccurred())

				r := reporter.NewReporterWithClient(
					resultsPath,
					50*time.Millisecond,
					5*time.Second,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err = r.Run(ctx)

				Expect(err).To(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonInvalidResultFormat))
			})
		})

		Context("when result file is empty", func() {
			It("reports parse error", func() {
				err := os.WriteFile(resultsPath, []byte(""), 0644)
				Expect(err).NotTo(HaveOccurred())

				r := reporter.NewReporterWithClient(
					resultsPath,
					50*time.Millisecond,
					5*time.Second,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err = r.Run(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("result file is empty"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonInvalidResultFormat))
			})
		})

		Context("when result file has invalid status", func() {
			It("reports parse error", func() {
				err := os.WriteFile(resultsPath, []byte(`{"status":"invalid","reason":"Test","message":"Test"}`), 0644)
				Expect(err).NotTo(HaveOccurred())

				r := reporter.NewReporterWithClient(
					resultsPath,
					50*time.Millisecond,
					5*time.Second,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err = r.Run(ctx)

				Expect(err).To(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonInvalidResultFormat))
			})
		})

		Context("when context is cancelled before completion", func() {
			It("stops polling and triggers timeout path", func() {
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					}, nil
				}

				cancelCtx, cancel := context.WithCancel(context.Background())

				r := reporter.NewReporterWithClient(
					resultsPath,
					50*time.Millisecond,
					5*time.Second,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				// Cancel context after a short delay
				go func() {
					time.Sleep(100 * time.Millisecond)
					cancel()
				}()

				err := r.Run(cancelCtx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("timeout waiting for adapter results"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterTimeout))
			})
		})

		Context("when UpdateFromResult fails", func() {
			It("returns the update error", func() {
				err := os.WriteFile(resultsPath, []byte(`{"status":"success","reason":"Test","message":"Test"}`), 0644)
				Expect(err).NotTo(HaveOccurred())

				mock.UpdateJobStatusFunc = func(ctx context.Context, condition k8s.JobCondition) error {
					return errors.New("k8s update failed")
				}

				r := reporter.NewReporterWithClient(
					resultsPath,
					50*time.Millisecond,
					5*time.Second,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err = r.Run(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update job status"))
			})
		})

		Context("when timeout occurs with terminated container", func() {
			It("reports container termination instead of timeout", func() {
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:   "Error",
								ExitCode: 1,
							},
						},
					}, nil
				}

				r := reporter.NewReporterWithClient(
					resultsPath,
					50*time.Millisecond,
					200*time.Millisecond,
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err := r.Run(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterExitedWithError))
			})
		})

		Context("when container terminates during polling without result file", func() {
			It("detects termination immediately and reports exit code", func() {
				callCount := 0
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					callCount++
					if callCount == 1 {
						// First poll: container is running
						return &corev1.ContainerStatus{
							Name: "adapter",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						}, nil
					}
					// Container terminates on second check
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:   "Error",
								ExitCode: 1,
							},
						},
					}, nil
				}

				r := reporter.NewReporterWithClientAndIntervals(
					resultsPath,
					50*time.Millisecond,
					5*time.Second,
					100*time.Millisecond, // Check container status every 100ms for tests
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err := r.Run(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("adapter container terminated"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal(reporter.ReasonAdapterExitedWithError))
				Expect(mock.LastUpdatedCondition.Message).To(ContainSubstring("Adapter container exited with code 1"))
			})
		})

		Context("when container terminates during polling with result file", func() {
			It("detects termination and uses result file", func() {
				callCount := 0
				mock.GetAdapterContainerStatusFunc = func(ctx context.Context, podName, containerName string) (*corev1.ContainerStatus, error) {
					callCount++
					if callCount == 1 {
						// First poll: container is running
						return &corev1.ContainerStatus{
							Name: "adapter",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						}, nil
					}
					// Container terminates on second check, and we write the result file
					if callCount == 2 {
						_ = os.WriteFile(resultsPath, []byte(`{"status":"failure","reason":"ValidationFailed","message":"Validation checks failed"}`), 0644)
					}
					return &corev1.ContainerStatus{
						Name: "adapter",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:   "Error",
								ExitCode: 1,
							},
						},
					}, nil
				}

				r := reporter.NewReporterWithClientAndIntervals(
					resultsPath,
					50*time.Millisecond,
					5*time.Second,
					100*time.Millisecond, // Check container status every 100ms for tests
					"Available",
					"test-pod",
					"adapter",
					mock,
				)

				err := r.Run(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(mock.LastUpdatedCondition.Type).To(Equal("Available"))
				Expect(mock.LastUpdatedCondition.Status).To(Equal("False"))
				Expect(mock.LastUpdatedCondition.Reason).To(Equal("ValidationFailed"))
				Expect(mock.LastUpdatedCondition.Message).To(Equal("Validation checks failed"))
			})
		})
	})
})
