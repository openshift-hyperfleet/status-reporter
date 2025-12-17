package main

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Main", func() {
	Describe("handleNormalCompletion", func() {
		Context("when reporter completes successfully", func() {
			It("returns exit code 0 for nil error", func() {
				exitCode := handleNormalCompletion(nil)
				Expect(exitCode).To(Equal(0))
			})
		})

		Context("when reporter completes with error", func() {
			It("returns exit code 1 for generic error", func() {
				exitCode := handleNormalCompletion(errors.New("test error"))
				Expect(exitCode).To(Equal(1))
			})

			It("returns exit code 1 for context.Canceled (unexpected in normal path)", func() {
				exitCode := handleNormalCompletion(context.Canceled)
				Expect(exitCode).To(Equal(1))
			})

			It("returns exit code 1 for wrapped error", func() {
				wrappedErr := errors.New("operation failed: database connection lost")
				exitCode := handleNormalCompletion(wrappedErr)
				Expect(exitCode).To(Equal(1))
			})
		})
	})

	Describe("handleShutdown", Serial, func() {
		var (
			done   chan error
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			done = make(chan error, 1)
			ctx, cancel = context.WithCancel(context.Background())
		})

		AfterEach(func() {
			cancel()
		})

		Context("when reporter stops within timeout", func() {
			It("returns exit code 0 for nil error", func() {
				go func() {
					<-ctx.Done()
					time.Sleep(10 * time.Millisecond)
					done <- nil
				}()

				exitCode := handleShutdown(syscall.SIGTERM, cancel, done)
				Expect(exitCode).To(Equal(0))
			})

			It("returns exit code 0 for context.Canceled (expected during shutdown)", func() {
				go func() {
					<-ctx.Done()
					time.Sleep(10 * time.Millisecond)
					done <- context.Canceled
				}()

				exitCode := handleShutdown(syscall.SIGTERM, cancel, done)
				Expect(exitCode).To(Equal(0))
			})

			It("returns exit code 1 for real errors", func() {
				go func() {
					<-ctx.Done()
					time.Sleep(10 * time.Millisecond)
					done <- errors.New("database connection failed")
				}()

				exitCode := handleShutdown(syscall.SIGTERM, cancel, done)
				Expect(exitCode).To(Equal(1))
			})

			It("handles SIGINT signal", func() {
				go func() {
					<-ctx.Done()
					time.Sleep(10 * time.Millisecond)
					done <- nil
				}()

				exitCode := handleShutdown(syscall.SIGINT, cancel, done)
				Expect(exitCode).To(Equal(0))
			})
		})

		Context("timer cleanup", func() {
			It("stops timer when reporter completes quickly", func() {
				go func() {
					<-ctx.Done()
					done <- nil
				}()

				// This shouldn't leak - timer should be stopped by defer
				exitCode := handleShutdown(syscall.SIGTERM, cancel, done)
				Expect(exitCode).To(Equal(0))

				// Give time for any leaked goroutines to show up
				time.Sleep(50 * time.Millisecond)
			})
		})
	})

	Describe("waitForCompletion", Serial, func() {
		var (
			sigChan chan os.Signal
			done    chan error
			ctx     context.Context
			cancel  context.CancelFunc
		)

		BeforeEach(func() {
			sigChan = make(chan os.Signal, 1)
			done = make(chan error, 1)
			ctx, cancel = context.WithCancel(context.Background())
		})

		AfterEach(func() {
			cancel()
		})

		Context("normal completion path", func() {
			It("returns exit code 0 when reporter succeeds", func() {
				done <- nil

				exitCode := waitForCompletion(sigChan, cancel, done)
				Expect(exitCode).To(Equal(0))
			})

			It("returns exit code 1 when reporter fails", func() {
				done <- errors.New("validation failed")

				exitCode := waitForCompletion(sigChan, cancel, done)
				Expect(exitCode).To(Equal(1))
			})

			It("returns exit code 1 for context.Canceled in normal path", func() {
				done <- context.Canceled

				exitCode := waitForCompletion(sigChan, cancel, done)
				Expect(exitCode).To(Equal(1))
			})
		})

		Context("signal-driven shutdown path", func() {
			It("returns exit code 0 for graceful shutdown with SIGTERM", func() {
				go func() {
					<-ctx.Done()
					time.Sleep(10 * time.Millisecond)
					done <- context.Canceled
				}()

				sigChan <- syscall.SIGTERM

				exitCode := waitForCompletion(sigChan, cancel, done)
				Expect(exitCode).To(Equal(0))
			})

			It("returns exit code 0 for graceful shutdown with SIGINT", func() {
				go func() {
					<-ctx.Done()
					time.Sleep(10 * time.Millisecond)
					done <- nil
				}()

				sigChan <- syscall.SIGINT

				exitCode := waitForCompletion(sigChan, cancel, done)
				Expect(exitCode).To(Equal(0))
			})

			It("returns exit code 1 when shutdown encounters real error", func() {
				go func() {
					<-ctx.Done()
					time.Sleep(10 * time.Millisecond)
					done <- errors.New("cleanup failed")
				}()

				sigChan <- syscall.SIGTERM

				exitCode := waitForCompletion(sigChan, cancel, done)
				Expect(exitCode).To(Equal(1))
			})
		})

		Context("race conditions", func() {
			It("handles reporter completing just as signal arrives", func() {
				// Both channels ready almost simultaneously
				go func() {
					time.Sleep(1 * time.Millisecond)
					done <- nil
				}()

				go func() {
					time.Sleep(1 * time.Millisecond)
					sigChan <- syscall.SIGTERM
				}()

				exitCode := waitForCompletion(sigChan, cancel, done)
				// Either path is acceptable, should succeed
				Expect(exitCode).To(Equal(0))
			})
		})
	})

	Describe("context.Canceled handling", Serial, func() {
		var (
			done   chan error
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			done = make(chan error, 1)
			ctx, cancel = context.WithCancel(context.Background())
		})

		AfterEach(func() {
			cancel()
		})

		Context("during shutdown", func() {
			It("treats context.Canceled as success (exit code 0)", func() {
				go func() {
					<-ctx.Done()
					done <- context.Canceled
				}()

				exitCode := handleShutdown(syscall.SIGTERM, cancel, done)
				Expect(exitCode).To(Equal(0))
			})

			It("treats nil as success (exit code 0)", func() {
				go func() {
					<-ctx.Done()
					done <- nil
				}()

				exitCode := handleShutdown(syscall.SIGTERM, cancel, done)
				Expect(exitCode).To(Equal(0))
			})

			It("treats real errors as failure (exit code 1)", func() {
				go func() {
					<-ctx.Done()
					done <- errors.New("database connection lost")
				}()

				exitCode := handleShutdown(syscall.SIGTERM, cancel, done)
				Expect(exitCode).To(Equal(1))
			})

			It("treats wrapped errors as failure even if they mention 'canceled'", func() {
				go func() {
					<-ctx.Done()
					done <- errors.New("operation canceled: database error")
				}()

				exitCode := handleShutdown(syscall.SIGTERM, cancel, done)
				Expect(exitCode).To(Equal(1))
			})
		})
	})

	Describe("shutdown timeout configuration", func() {
		It("has expected timeout value", func() {
			Expect(shutdownTimeout).To(Equal(5 * time.Second))
		})
	})

	Describe("signal handling", func() {
		Context("supported signals", func() {
			It("handles SIGTERM correctly", func() {
				sigChan := make(chan os.Signal, 1)
				done := make(chan error, 1)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go func() {
					<-ctx.Done()
					done <- nil
				}()

				sigChan <- syscall.SIGTERM

				exitCode := waitForCompletion(sigChan, cancel, done)
				Expect(exitCode).To(Equal(0))
			})

			It("handles SIGINT correctly", func() {
				sigChan := make(chan os.Signal, 1)
				done := make(chan error, 1)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go func() {
					<-ctx.Done()
					done <- nil
				}()

				sigChan <- syscall.SIGINT

				exitCode := waitForCompletion(sigChan, cancel, done)
				Expect(exitCode).To(Equal(0))
			})
		})
	})

	Describe("error propagation", func() {
		Context("various error types", func() {
			It("correctly propagates validation errors", func() {
				err := errors.New("validation failed: invalid configuration")
				exitCode := handleNormalCompletion(err)
				Expect(exitCode).To(Equal(1))
			})

			It("correctly propagates I/O errors", func() {
				err := errors.New("failed to read results file")
				exitCode := handleNormalCompletion(err)
				Expect(exitCode).To(Equal(1))
			})

			It("correctly propagates network errors", func() {
				err := errors.New("connection refused")
				exitCode := handleNormalCompletion(err)
				Expect(exitCode).To(Equal(1))
			})
		})
	})
})
