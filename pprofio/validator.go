package pprofio

import (
	"fmt"

	"github.com/google/pprof/profile"

	"cpgo"
)

// Validator ensures profile payloads are valid pprof data with samples.
type Validator struct{}

var _ cpgo.ProfileValidator = (*Validator)(nil)

// NewValidator returns a pprof payload validator.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateCPUProfile verifies pprof encoding and minimum sample presence.
func (validator *Validator) ValidateCPUProfile(raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("cpu profile is empty")
	}

	parsed, err := profile.ParseData(raw)
	if err != nil {
		return fmt.Errorf("parse cpu profile: %w", err)
	}

	if len(parsed.Sample) == 0 {
		return fmt.Errorf("cpu profile has no samples")
	}

	return nil
}
