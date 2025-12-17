package k8s_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/status-reporter/pkg/k8s"
)

var _ = Describe("JobCondition", func() {
	Describe("creation", func() {
		It("can be created with all fields", func() {
			now := time.Now()
			condition := k8s.JobCondition{
				Type:               "Available",
				Status:             "True",
				Reason:             "TestPassed",
				Message:            "Test completed successfully",
				LastTransitionTime: now,
			}

			Expect(condition.Type).To(Equal("Available"))
			Expect(condition.Status).To(Equal("True"))
			Expect(condition.Reason).To(Equal("TestPassed"))
			Expect(condition.Message).To(Equal("Test completed successfully"))
			Expect(condition.LastTransitionTime).To(Equal(now))
		})

		It("can be created with zero LastTransitionTime", func() {
			condition := k8s.JobCondition{
				Type:    "Available",
				Status:  "False",
				Reason:  "TestFailed",
				Message: "Test failed",
			}

			Expect(condition.LastTransitionTime.IsZero()).To(BeTrue())
		})
	})
})
