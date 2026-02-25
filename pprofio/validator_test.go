package pprofio

import (
	"bytes"
	"testing"

	"github.com/google/pprof/profile"
)

func TestValidatorValidateCPUProfile(t *testing.T) {
	t.Run("accepts a valid pprof payload", func(t *testing.T) {
		validator := NewValidator()

		validProfile := &profile.Profile{
			SampleType: []*profile.ValueType{
				{
					Type: "samples",
					Unit: "count",
				},
			},
			Location: []*profile.Location{
				{
					ID: 1,
				},
			},
		}
		validProfile.Sample = []*profile.Sample{
			{
				Value:    []int64{1},
				Location: []*profile.Location{validProfile.Location[0]},
			},
		}

		var raw bytes.Buffer
		if err := validProfile.Write(&raw); err != nil {
			t.Fatalf("write valid profile: %v", err)
		}

		if err := validator.ValidateCPUProfile(raw.Bytes()); err != nil {
			t.Fatalf("validate profile: %v", err)
		}
	})

	t.Run("rejects invalid profile payload", func(t *testing.T) {
		validator := NewValidator()
		if err := validator.ValidateCPUProfile([]byte("not-a-profile")); err == nil {
			t.Fatalf("expected validation error")
		}
	})
}
