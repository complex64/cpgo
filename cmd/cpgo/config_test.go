package main

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Run("decodes yaml configuration", func(t *testing.T) {
		tempFile, err := os.CreateTemp(t.TempDir(), "cpgo-config-*.yaml")
		if err != nil {
			t.Fatalf("create temp config: %v", err)
		}
		defer func() { _ = tempFile.Close() }()

		_, err = tempFile.WriteString(`
profile:
  url: https://example.com/debug/pprof/profile
  seconds: 60
repository:
  owner: acme
  name: payments
  pgo_path: default.pgo
github:
  app_id: 123
  private_key_path: /tmp/key.pem
`)
		if err != nil {
			t.Fatalf("write temp config: %v", err)
		}

		cfg, err := Load(tempFile.Name())
		if err != nil {
			t.Fatalf("load config: %v", err)
		}

		if cfg.Profile.Seconds != 60 {
			t.Fatalf("expected seconds to be 60, got %d", cfg.Profile.Seconds)
		}

		if cfg.Repository.Owner != "acme" {
			t.Fatalf("expected owner acme, got %s", cfg.Repository.Owner)
		}
	})
}

func TestBuildRunRequest(t *testing.T) {
	t.Run("maps parsed config to run request", func(t *testing.T) {
		req, err := BuildRunRequest(File{
			Profile: Profile{
				URL:     "https://example.com/debug/pprof/profile",
				Seconds: 30,
				Headers: map[string]string{
					"Authorization": "Bearer x",
				},
			},
			Repository: Repository{
				Owner:   "acme",
				Name:    "payments",
				PGOPath: "default.pgo",
			},
		})
		if err != nil {
			t.Fatalf("build run request: %v", err)
		}

		if req.Profile.URL.String() != "https://example.com/debug/pprof/profile" {
			t.Fatalf("unexpected profile url: %s", req.Profile.URL.String())
		}

		if req.Repository.Owner != "acme" {
			t.Fatalf("unexpected owner: %s", req.Repository.Owner)
		}
	})

	t.Run("returns error for invalid profile url", func(t *testing.T) {
		_, err := BuildRunRequest(File{
			Profile: Profile{
				URL: "://bad-url",
			},
		})
		if err == nil {
			t.Fatalf("expected url parse error")
		}
	})
}

func TestOperationTimeout(t *testing.T) {
	t.Run("uses default timeout when unset", func(t *testing.T) {
		timeout, err := OperationTimeout(File{})
		if err != nil {
			t.Fatalf("operation timeout: %v", err)
		}

		if timeout != 2*time.Minute {
			t.Fatalf("expected default timeout of 2m, got %s", timeout)
		}
	})

	t.Run("returns error for invalid timeout", func(t *testing.T) {
		_, err := OperationTimeout(File{
			Runtime: Runtime{
				Timeout: "abc",
			},
		})
		if err == nil {
			t.Fatalf("expected timeout parse error")
		}
	})
}
