package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config represents the status reporter configuration
type Config struct {
	JobName              string
	JobNamespace         string
	PodName              string
	ResultsPath          string
	PollIntervalSeconds  int
	MaxWaitTimeSeconds   int
	ConditionType        string
	LogLevel             string
	AdapterContainerName string
}

const (
	DefaultResultsPath          = "/results/adapter-result.json"
	DefaultPollIntervalSeconds  = 2
	DefaultMaxWaitTimeSeconds   = 300
	DefaultConditionType        = "Available"
	DefaultLogLevel             = "info"
	DefaultAdapterContainerName = ""
)

const (
	EnvJobName              = "JOB_NAME"
	EnvJobNamespace         = "JOB_NAMESPACE"
	EnvPodName              = "POD_NAME"
	EnvResultsPath          = "RESULTS_PATH"
	EnvPollIntervalSeconds  = "POLL_INTERVAL_SECONDS"
	EnvMaxWaitTimeSeconds   = "MAX_WAIT_TIME_SECONDS"
	EnvConditionType        = "CONDITION_TYPE"
	EnvLogLevel             = "LOG_LEVEL"
	EnvAdapterContainerName = "ADAPTER_CONTAINER_NAME"
)

// ValidationError represents a validation error for configuration or data validation
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	jobName, err := getRequiredEnv(EnvJobName)
	if err != nil {
		return nil, err
	}

	jobNamespace, err := getRequiredEnv(EnvJobNamespace)
	if err != nil {
		return nil, err
	}

	podName, err := getRequiredEnv(EnvPodName)
	if err != nil {
		return nil, err
	}

	resultsPath := getEnvOrDefault(EnvResultsPath, DefaultResultsPath)
	conditionType := getEnvOrDefault(EnvConditionType, DefaultConditionType)
	logLevel := getEnvOrDefault(EnvLogLevel, DefaultLogLevel)
	adapterContainerName := getEnvOrDefault(EnvAdapterContainerName, DefaultAdapterContainerName)

	pollIntervalSeconds, err := getEnvIntOrDefault(EnvPollIntervalSeconds, DefaultPollIntervalSeconds)
	if err != nil {
		return nil, err
	}

	maxWaitTimeSeconds, err := getEnvIntOrDefault(EnvMaxWaitTimeSeconds, DefaultMaxWaitTimeSeconds)
	if err != nil {
		return nil, err
	}

	config := &Config{
		JobName:              jobName,
		JobNamespace:         jobNamespace,
		PodName:              podName,
		ResultsPath:          resultsPath,
		PollIntervalSeconds:  pollIntervalSeconds,
		MaxWaitTimeSeconds:   maxWaitTimeSeconds,
		ConditionType:        conditionType,
		LogLevel:             logLevel,
		AdapterContainerName: adapterContainerName,
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.PollIntervalSeconds <= 0 {
		return &ValidationError{Field: "PollIntervalSeconds", Message: "must be positive"}
	}
	if c.MaxWaitTimeSeconds <= 0 {
		return &ValidationError{Field: "MaxWaitTimeSeconds", Message: "must be positive"}
	}
	if c.PollIntervalSeconds >= c.MaxWaitTimeSeconds {
		return &ValidationError{Field: "PollIntervalSeconds", Message: "must be less than MaxWaitTimeSeconds"}
	}

	if err := c.validateResultsPath(); err != nil {
		return err
	}

	return nil
}

// validateResultsPath ensures the results path is safe
func (c *Config) validateResultsPath() error {
	if strings.HasSuffix(c.ResultsPath, "/") {
		return &ValidationError{
			Field:   "ResultsPath",
			Message: "path must be a file, not a directory",
		}
	}

	cleanPath := filepath.Clean(c.ResultsPath)

	if !filepath.IsAbs(cleanPath) {
		return &ValidationError{
			Field:   "ResultsPath",
			Message: "path must be absolute",
		}
	}

	return nil
}

// GetPollInterval returns poll interval as duration
func (c *Config) GetPollInterval() time.Duration {
	return time.Duration(c.PollIntervalSeconds) * time.Second
}

// GetMaxWaitTime returns max wait time as duration
func (c *Config) GetMaxWaitTime() time.Duration {
	return time.Duration(c.MaxWaitTimeSeconds) * time.Second
}

func getEnvOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func getRequiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", &ValidationError{Field: key, Message: "required"}
	}
	return value, nil
}

func getEnvIntOrDefault(key string, defaultValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("must be a valid integer, got: %s", value),
		}
	}

	return intValue, nil
}
