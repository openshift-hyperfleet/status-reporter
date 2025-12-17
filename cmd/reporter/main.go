package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/openshift-hyperfleet/status-reporter/pkg/config"
	"github.com/openshift-hyperfleet/status-reporter/pkg/reporter"
)

const (
	shutdownTimeout = 5 * time.Second
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Status Reporter starting...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	logConfig(cfg)

	rep, err := reporter.NewReporter(
		cfg.ResultsPath,
		cfg.GetPollInterval(),
		cfg.GetMaxWaitTime(),
		cfg.ConditionType,
		cfg.PodName,
		cfg.AdapterContainerName,
		cfg.JobName,
		cfg.JobNamespace,
	)
	if err != nil {
		log.Fatalf("Failed to create reporter: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigChan)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run reporter in background with panic recovery
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in reporter: %v\nStack trace:\n%s", r, debug.Stack())
				done <- fmt.Errorf("reporter panicked: %v", r)
			}
		}()
		done <- rep.Run(ctx)
	}()

	// Wait for completion or interruption and exit
	os.Exit(waitForCompletion(sigChan, cancel, done))
}

// waitForCompletion handles both normal completion and signal-driven shutdown.
// It returns the appropriate exit code based on the outcome.
func waitForCompletion(sigChan <-chan os.Signal, cancel context.CancelFunc, done <-chan error) int {
	select {
	case err := <-done:
		// Normal completion path
		return handleNormalCompletion(err)

	case sig := <-sigChan:
		// Shutdown requested
		return handleShutdown(sig, cancel, done)
	}
}

// handleNormalCompletion processes normal reporter completion
func handleNormalCompletion(err error) int {
	if err != nil {
		log.Printf("Reporter finished with error: %v", err)
		return 1
	}
	log.Println("Reporter finished successfully")
	return 0
}

// handleShutdown manages graceful shutdown with timeout
func handleShutdown(sig os.Signal, cancel context.CancelFunc, done <-chan error) int {
	log.Printf("Received signal %v, initiating graceful shutdown...", sig)
	cancel()

	// Create timer with explicit cleanup to avoid resource leak
	timer := time.NewTimer(shutdownTimeout)
	defer timer.Stop()

	// Wait for graceful shutdown with timeout
	select {
	case err := <-done:
		// Reporter stopped within timeout
		if err != nil && !errors.Is(err, context.Canceled) {
			// Real error occurred (context.Canceled is expected during shutdown)
			log.Printf("Reporter stopped with error: %v", err)
			return 1
		}
		log.Println("Shutdown complete")
		return 0

	case <-timer.C:
		// Timeout exceeded - force exit
		log.Printf("Shutdown timeout (%s) exceeded; forcing exit", shutdownTimeout)
		return 1
	}
}

// logConfig logs the loaded configuration
func logConfig(cfg *config.Config) {
	log.Println("Configuration:")
	log.Printf("  JOB_NAME: %s", cfg.JobName)
	log.Printf("  JOB_NAMESPACE: %s", cfg.JobNamespace)
	log.Printf("  POD_NAME: %s", cfg.PodName)
	if cfg.AdapterContainerName != "" {
		log.Printf("  ADAPTER_CONTAINER_NAME: %s", cfg.AdapterContainerName)
	} else {
		log.Printf("  ADAPTER_CONTAINER_NAME: (auto-detect)")
	}
	log.Printf("  RESULTS_PATH: %s", cfg.ResultsPath)
	log.Printf("  POLL_INTERVAL_SECONDS: %d", cfg.PollIntervalSeconds)
	log.Printf("  MAX_WAIT_TIME_SECONDS: %d", cfg.MaxWaitTimeSeconds)
	log.Printf("  CONDITION_TYPE: %s", cfg.ConditionType)
	log.Printf("  LOG_LEVEL: %s", cfg.LogLevel)
}
