package result_test

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/status-reporter/pkg/result"
)

var _ = Describe("AdapterResult", func() {
	Describe("IsSuccess", func() {
		It("returns true for success status", func() {
			r := &result.AdapterResult{Status: result.StatusSuccess}
			Expect(r.IsSuccess()).To(BeTrue())
		})

		It("returns false for failure status", func() {
			r := &result.AdapterResult{Status: result.StatusFailure}
			Expect(r.IsSuccess()).To(BeFalse())
		})

		It("returns false for invalid status", func() {
			r := &result.AdapterResult{Status: "invalid"}
			Expect(r.IsSuccess()).To(BeFalse())
		})
	})

	Describe("Validate", func() {
		Context("with valid results", func() {
			It("accepts valid success result", func() {
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "AllChecksPassed",
					Message: "All validation checks passed",
				}
				Expect(r.Validate()).To(Succeed())
			})

			It("accepts valid failure result", func() {
				r := &result.AdapterResult{
					Status:  result.StatusFailure,
					Reason:  "SomeCheckFailed",
					Message: "Validation failed",
				}
				Expect(r.Validate()).To(Succeed())
			})
		})

		Context("with invalid status", func() {
			It("returns error for invalid status", func() {
				r := &result.AdapterResult{
					Status:  "invalid",
					Reason:  "Test",
					Message: "Test message",
				}
				err := r.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be either 'success' or 'failure'"))
			})
		})

		Context("with empty or whitespace fields", func() {
			It("provides default reason for empty reason", func() {
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "",
					Message: "Test message",
				}
				Expect(r.Validate()).To(Succeed())
				Expect(r.Reason).To(Equal(result.DefaultReason))
			})

			It("provides default reason for whitespace-only reason", func() {
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "   ",
					Message: "Test message",
				}
				Expect(r.Validate()).To(Succeed())
				Expect(r.Reason).To(Equal(result.DefaultReason))
			})

			It("provides default message for empty message", func() {
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "TestReason",
					Message: "",
				}
				Expect(r.Validate()).To(Succeed())
				Expect(r.Message).To(Equal(result.DefaultMessage))
			})

			It("provides default message for whitespace-only message", func() {
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "TestReason",
					Message: "   ",
				}
				Expect(r.Validate()).To(Succeed())
				Expect(r.Message).To(Equal(result.DefaultMessage))
			})
		})

		Context("with whitespace", func() {
			It("trims leading and trailing whitespace from reason", func() {
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "  TestReason  ",
					Message: "Test message",
				}
				Expect(r.Validate()).To(Succeed())
				Expect(r.Reason).To(Equal("TestReason"))
			})

			It("trims leading and trailing whitespace from message", func() {
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "TestReason",
					Message: "  Test message  ",
				}
				Expect(r.Validate()).To(Succeed())
				Expect(r.Message).To(Equal("Test message"))
			})
		})

		Context("with overly long fields", func() {
			It("truncates long reason to max length", func() {
				longReason := strings.Repeat("A", 200)
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  longReason,
					Message: "Test message",
				}
				Expect(r.Validate()).To(Succeed())
				Expect(len(r.Reason)).To(Equal(128))
			})

			It("truncates long message to max length", func() {
				longMessage := strings.Repeat("A", 2000)
				r := &result.AdapterResult{
					Status:  result.StatusSuccess,
					Reason:  "TestReason",
					Message: longMessage,
				}
				Expect(r.Validate()).To(Succeed())
				Expect(len(r.Message)).To(Equal(1024))
			})
		})
	})

	Describe("JSON marshaling", func() {
		It("unmarshals basic success result", func() {
			jsonData := `{"status":"success","reason":"TestPassed","message":"Test completed"}`
			var r result.AdapterResult

			err := json.Unmarshal([]byte(jsonData), &r)
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Status).To(Equal(result.StatusSuccess))
			Expect(r.Reason).To(Equal("TestPassed"))
			Expect(r.Message).To(Equal("Test completed"))
		})

		It("unmarshals result with details", func() {
			jsonData := `{"status":"failure","reason":"TestFailed","message":"Test failed","details":{"key":"value"}}`
			var r result.AdapterResult

			err := json.Unmarshal([]byte(jsonData), &r)
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Status).To(Equal(result.StatusFailure))
			Expect(r.Details).To(Equal(json.RawMessage(`{"key":"value"}`)))
		})

		It("unmarshals result with nested details", func() {
			jsonData := `{"status":"success","reason":"OK","message":"OK","details":{"nested":{"deep":"value"}}}`
			var r result.AdapterResult

			err := json.Unmarshal([]byte(jsonData), &r)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(r.Details)).To(ContainSubstring("nested"))
			Expect(string(r.Details)).To(ContainSubstring("deep"))
		})
	})
})

var _ = Describe("ResultError", func() {
	It("formats error message correctly", func() {
		err := &result.ResultError{Field: "status", Message: "required"}
		Expect(err.Error()).To(Equal("status: required"))
	})

	It("handles longer messages", func() {
		err := &result.ResultError{Field: "reason", Message: "must be alphanumeric"}
		Expect(err.Error()).To(Equal("reason: must be alphanumeric"))
	})
})
