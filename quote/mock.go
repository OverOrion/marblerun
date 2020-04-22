package quote

import (
	"crypto/sha256"
	"errors"

	"github.com/google/go-cmp/cmp"
)

type entry struct {
	message []byte
	pp      PackageProperties
	ip      InfrastructureProperties
}

// MockValidator is a mockup quote validator
type MockValidator struct {
	valid map[string]entry
}

// NewMockValidator creates a new MockValidator
func NewMockValidator() *MockValidator {
	return &MockValidator{
		make(map[string]entry),
	}
}

// Validate implements the Validator interface
func (m *MockValidator) Validate(quote []byte, message []byte, pp PackageProperties, ip InfrastructureProperties) error {
	entry, found := m.valid[string(quote)]
	if !found {
		return errors.New("wrong quote")
	}
	if !cmp.Equal(entry.message, message) {
		return errors.New("wrong message")
	}
	if !cmp.Equal(entry.requirements, requirements) {
		return errors.New("wrong requirements")
	}
	return nil
}

// AddValidQuote adds a valid quote
func (m *MockValidator) AddValidQuote(quote []byte, message []byte, pp PackageProperties, ip InfrastructureProperties) {
	m.valid[string(quote)] = entry{message, requirements}
}

// MockIssuer is a mockup quote issuer
type MockIssuer struct{}

// NewMockIssuer creates a new MockIssuer
func NewMockIssuer() *MockIssuer {
	return &MockIssuer{}
}

// Issue implements the Issuer interface
func (m *MockIssuer) Issue(message []byte) ([]byte, error) {
	quote := sha256.Sum256(message)
	return quote[:], nil
}
