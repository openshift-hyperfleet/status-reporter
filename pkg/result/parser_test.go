package result_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/status-reporter/pkg/result"
)

var _ = Describe("Parser", func() {
	var parser *result.Parser

	BeforeEach(func() {
		parser = result.NewParser()
	})

	Describe("NewParser", func() {
		It("creates a new parser", func() {
			Expect(parser).NotTo(BeNil())
		})
	})

	Describe("ParseFile", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "parser-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})

		Context("with valid files", func() {
			It("parses valid success result", func() {
				content := `{"status":"success","reason":"TestPassed","message":"Test completed"}`
				tmpFile := filepath.Join(tmpDir, "result.json")
				err := os.WriteFile(tmpFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())

				r, err := parser.ParseFile(tmpFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(r).NotTo(BeNil())
				Expect(r.Status).To(Equal(result.StatusSuccess))
			})

			It("parses valid failure result", func() {
				content := `{"status":"failure","reason":"TestFailed","message":"Test failed"}`
				tmpFile := filepath.Join(tmpDir, "result.json")
				err := os.WriteFile(tmpFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())

				r, err := parser.ParseFile(tmpFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(r).NotTo(BeNil())
				Expect(r.Status).To(Equal(result.StatusFailure))
			})
		})

		Context("with invalid files", func() {
			It("returns error for empty file", func() {
				tmpFile := filepath.Join(tmpDir, "empty.json")
				err := os.WriteFile(tmpFile, []byte(""), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = parser.ParseFile(tmpFile)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("result file is empty"))
			})

			It("returns error for invalid JSON", func() {
				content := `{invalid json}`
				tmpFile := filepath.Join(tmpDir, "invalid.json")
				err := os.WriteFile(tmpFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = parser.ParseFile(tmpFile)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse JSON"))
			})

			It("returns error for invalid status", func() {
				content := `{"status":"invalid","reason":"Test","message":"Test"}`
				tmpFile := filepath.Join(tmpDir, "badstatus.json")
				err := os.WriteFile(tmpFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = parser.ParseFile(tmpFile)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid result format"))
			})

			It("returns error for file too large", func() {
				content := strings.Repeat("x", 1*1024*1024+1)
				tmpFile := filepath.Join(tmpDir, "large.json")
				err := os.WriteFile(tmpFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = parser.ParseFile(tmpFile)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("result file too large"))
			})

			It("returns error for nonexistent file", func() {
				_, err := parser.ParseFile("/nonexistent/path/file.json")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read result file"))
			})
		})
	})

	Describe("Parse", func() {
		Context("with valid data", func() {
			It("parses valid JSON", func() {
				data := []byte(`{"status":"success","reason":"OK","message":"OK"}`)
				r, err := parser.Parse(data)
				Expect(err).NotTo(HaveOccurred())
				Expect(r).NotTo(BeNil())
				Expect(r.Status).To(Equal(result.StatusSuccess))
			})

			It("provides defaults for missing fields", func() {
				data := []byte(`{"status":"success"}`)
				r, err := parser.Parse(data)
				Expect(err).NotTo(HaveOccurred())
				Expect(r.Reason).To(Equal(result.DefaultReason))
				Expect(r.Message).To(Equal(result.DefaultMessage))
			})
		})

		Context("with invalid data", func() {
			It("returns error for invalid JSON", func() {
				data := []byte(`{bad json`)
				_, err := parser.Parse(data)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse JSON"))
			})

			It("returns error for invalid status value", func() {
				data := []byte(`{"status":"unknown","reason":"Test","message":"Test"}`)
				_, err := parser.Parse(data)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid result format"))
			})
		})
	})
})
