package result

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	StatusSuccess = "success"
	StatusFailure = "failure"

	DefaultReason  = "NoReasonProvided"
	DefaultMessage = "No message provided"

	maxReasonLength  = 128
	maxMessageLength = 1024
)

// ResultError represents a validation error for adapter result validation
type ResultError struct {
	Field   string
	Message string
}

func (e *ResultError) Error() string {
	return e.Field + ": " + e.Message
}

// AdapterResult represents the result contract that any adapter must produce
type AdapterResult struct {
	// Status must be either StatusSuccess or StatusFailure
	Status string `json:"status"`

	// Reason is a machine-readable identifier (e.g., "AllChecksPassed", "DNSConfigured")
	Reason string `json:"reason"`

	// Message is a human-readable description
	Message string `json:"message"`

	// Details contains optional adapter-specific data as raw JSON
	Details json.RawMessage `json:"details,omitempty"`
}

// IsSuccess returns true if the adapter operation succeeded
func (r *AdapterResult) IsSuccess() bool {
	return r.Status == StatusSuccess
}

// Validate validates and normalizes the result
func (r *AdapterResult) Validate() error {
	if r.Status != StatusSuccess && r.Status != StatusFailure {
		return &ResultError{
			Field:   "status",
			Message: fmt.Sprintf("must be either '%s' or '%s'", StatusSuccess, StatusFailure),
		}
	}

	r.Reason = strings.TrimSpace(r.Reason)
	if r.Reason == "" {
		r.Reason = DefaultReason
	}
	if len(r.Reason) > maxReasonLength {
		r.Reason = truncateUTF8(r.Reason, maxReasonLength)
	}

	r.Message = strings.TrimSpace(r.Message)
	if r.Message == "" {
		r.Message = DefaultMessage
	}
	if len(r.Message) > maxMessageLength {
		r.Message = truncateUTF8(r.Message, maxMessageLength)
	}

	return nil
}

// truncateUTF8 safely truncates a string to maxBytes without splitting multi-byte UTF-8 characters
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	// Find the last valid UTF-8 character boundary before maxBytes
	for i := maxBytes; i > 0; i-- {
		if utf8.RuneStart(s[i]) {
			return s[:i]
		}
	}

	// Fallback (should never happen with valid UTF-8)
	return ""
}
