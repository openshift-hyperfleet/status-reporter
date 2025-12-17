package result_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestResult(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Result Suite")
}
