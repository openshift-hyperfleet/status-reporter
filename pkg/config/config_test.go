package config_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/status-reporter/pkg/config"
)

var _ = Describe("Config", func() {
	var originalEnv map[string]string

	BeforeEach(func() {
		originalEnv = make(map[string]string)
		envVars := []string{
			"JOB_NAME", "JOB_NAMESPACE", "POD_NAME",
			"RESULTS_PATH", "POLL_INTERVAL_SECONDS", "MAX_WAIT_TIME_SECONDS",
			"CONDITION_TYPE", "LOG_LEVEL", "ADAPTER_CONTAINER_NAME",
		}
		for _, key := range envVars {
			originalEnv[key] = os.Getenv(key)
			os.Unsetenv(key)
		}
	})

	AfterEach(func() {
		for key, value := range originalEnv {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	})

	Describe("Load", func() {
		Context("with valid required configuration", func() {
			BeforeEach(func() {
				os.Setenv("JOB_NAME", "test-job")
				os.Setenv("JOB_NAMESPACE", "test-namespace")
				os.Setenv("POD_NAME", "test-pod")
			})

			It("loads configuration successfully", func() {
				cfg, err := config.Load()
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())
				Expect(cfg.JobName).To(Equal("test-job"))
				Expect(cfg.JobNamespace).To(Equal("test-namespace"))
				Expect(cfg.PodName).To(Equal("test-pod"))
			})

			It("uses default values for optional fields", func() {
				cfg, err := config.Load()
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.ResultsPath).To(Equal("/results/adapter-result.json"))
				Expect(cfg.PollIntervalSeconds).To(Equal(2))
				Expect(cfg.MaxWaitTimeSeconds).To(Equal(300))
				Expect(cfg.ConditionType).To(Equal("Available"))
				Expect(cfg.LogLevel).To(Equal("info"))
				Expect(cfg.AdapterContainerName).To(Equal(""))
			})

			It("uses custom values when provided", func() {
				os.Setenv("RESULTS_PATH", "/results/custom/path.json")
				os.Setenv("POLL_INTERVAL_SECONDS", "5")
				os.Setenv("MAX_WAIT_TIME_SECONDS", "600")
				os.Setenv("CONDITION_TYPE", "Ready")
				os.Setenv("LOG_LEVEL", "debug")
				os.Setenv("ADAPTER_CONTAINER_NAME", "my-adapter")

				cfg, err := config.Load()
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.ResultsPath).To(Equal("/results/custom/path.json"))
				Expect(cfg.PollIntervalSeconds).To(Equal(5))
				Expect(cfg.MaxWaitTimeSeconds).To(Equal(600))
				Expect(cfg.ConditionType).To(Equal("Ready"))
				Expect(cfg.LogLevel).To(Equal("debug"))
				Expect(cfg.AdapterContainerName).To(Equal("my-adapter"))
			})

			It("trims whitespace from values", func() {
				os.Setenv("JOB_NAME", "  test-job  ")
				os.Setenv("JOB_NAMESPACE", "  test-namespace  ")

				cfg, err := config.Load()
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.JobName).To(Equal("test-job"))
				Expect(cfg.JobNamespace).To(Equal("test-namespace"))
			})
		})

		Context("with missing required configuration", func() {
			It("returns error when JOB_NAME is missing", func() {
				os.Setenv("JOB_NAMESPACE", "test-namespace")
				os.Setenv("POD_NAME", "test-pod")

				_, err := config.Load()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("JOB_NAME"))
			})

			It("returns error when JOB_NAMESPACE is missing", func() {
				os.Setenv("JOB_NAME", "test-job")
				os.Setenv("POD_NAME", "test-pod")

				_, err := config.Load()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("JOB_NAMESPACE"))
			})

			It("returns error when POD_NAME is missing", func() {
				os.Setenv("JOB_NAME", "test-job")
				os.Setenv("JOB_NAMESPACE", "test-namespace")

				_, err := config.Load()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("POD_NAME"))
			})
		})

		Context("with invalid integer values", func() {
			BeforeEach(func() {
				os.Setenv("JOB_NAME", "test-job")
				os.Setenv("JOB_NAMESPACE", "test-namespace")
				os.Setenv("POD_NAME", "test-pod")
			})

			It("returns error for invalid POLL_INTERVAL_SECONDS", func() {
				os.Setenv("POLL_INTERVAL_SECONDS", "invalid")

				_, err := config.Load()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("POLL_INTERVAL_SECONDS"))
			})

			It("returns error for invalid MAX_WAIT_TIME_SECONDS", func() {
				os.Setenv("MAX_WAIT_TIME_SECONDS", "invalid")

				_, err := config.Load()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("MAX_WAIT_TIME_SECONDS"))
			})
		})
	})

	Describe("Validate", func() {
		Context("with valid configuration", func() {
			It("validates successfully", func() {
				cfg := &config.Config{
					JobName:             "test-job",
					JobNamespace:        "test-namespace",
					PodName:             "test-pod",
					ResultsPath:         "/results/result.json",
					PollIntervalSeconds: 2,
					MaxWaitTimeSeconds:  300,
				}
				Expect(cfg.Validate()).To(Succeed())
			})
		})

		Context("with invalid timing parameters", func() {
			It("returns error for zero poll interval", func() {
				cfg := &config.Config{
					ResultsPath:         "/results/result.json",
					PollIntervalSeconds: 0,
					MaxWaitTimeSeconds:  300,
				}
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be positive"))
			})

			It("returns error for negative poll interval", func() {
				cfg := &config.Config{
					ResultsPath:         "/results/result.json",
					PollIntervalSeconds: -1,
					MaxWaitTimeSeconds:  300,
				}
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be positive"))
			})

			It("returns error for zero max wait time", func() {
				cfg := &config.Config{
					ResultsPath:         "/results/result.json",
					PollIntervalSeconds: 2,
					MaxWaitTimeSeconds:  0,
				}
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be positive"))
			})

			It("returns error when poll interval >= max wait time", func() {
				cfg := &config.Config{
					ResultsPath:         "/results/result.json",
					PollIntervalSeconds: 300,
					MaxWaitTimeSeconds:  300,
				}
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be less than MaxWaitTimeSeconds"))
			})
		})

		Context("with invalid results path", func() {
			It("returns error for relative path", func() {
				cfg := &config.Config{
					ResultsPath:         "results/result.json",
					PollIntervalSeconds: 2,
					MaxWaitTimeSeconds:  300,
				}
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be absolute"))
			})


			It("returns error for directory path", func() {
				cfg := &config.Config{
					ResultsPath:         "/results/",
					PollIntervalSeconds: 2,
					MaxWaitTimeSeconds:  300,
				}
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be a file"))
			})
		})
	})

	Describe("GetPollInterval", func() {
		It("returns poll interval as duration", func() {
			cfg := &config.Config{PollIntervalSeconds: 5}
			Expect(cfg.GetPollInterval()).To(Equal(5 * time.Second))
		})
	})

	Describe("GetMaxWaitTime", func() {
		It("returns max wait time as duration", func() {
			cfg := &config.Config{MaxWaitTimeSeconds: 600}
			Expect(cfg.GetMaxWaitTime()).To(Equal(600 * time.Second))
		})
	})
})
