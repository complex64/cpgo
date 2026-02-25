package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"cpgo"
)

const (
	defaultOperationTimeout = 2 * time.Minute
	defaultProfileTimeout   = 45 * time.Second
	defaultGitHubTimeout    = 30 * time.Second
)

// File is the root cpgo runtime configuration document.
type File struct {
	Profile     Profile
	Repository  Repository
	GitHub      GitHub
	PullRequest PullRequest
	Commit      Commit
	Runtime     Runtime
}

// Profile configures CPU profile collection from the target service.
type Profile struct {
	URL     string            `yaml:"url"`
	Seconds int               `yaml:"seconds"`
	Timeout string            `yaml:"timeout"`
	Headers map[string]string `yaml:"headers"`
}

// Repository configures where cpgo writes profile updates.
type Repository struct {
	Owner      string `yaml:"owner"`
	Name       string `yaml:"name"`
	PGOPath    string `yaml:"pgo_path"`
	BaseBranch string `yaml:"base_branch"`
	HeadBranch string `yaml:"head_branch"`
}

// GitHub configures authentication and API timeout behavior.
type GitHub struct {
	AppID          int64  `yaml:"app_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
	Token          string `yaml:"token"`
	Timeout        string `yaml:"timeout"`
}

// PullRequest configures metadata for cpgo-managed pull requests.
type PullRequest struct {
	Title           string `yaml:"title"`
	Body            string `yaml:"body"`
	ManagedByMarker string `yaml:"managed_by_marker"`
}

// Commit configures commit metadata for generated updates.
type Commit struct {
	Message string `yaml:"message"`
}

// Runtime configures top-level execution timing.
type Runtime struct {
	Timeout string `yaml:"timeout"`
}

// Load reads and decodes a cpgo configuration file from disk.
func Load(path string) (File, error) {
	if strings.TrimSpace(path) == "" {
		return File{}, fmt.Errorf("config path is required")
	}

	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return File{}, fmt.Errorf("decode config file: %w", err)
	}

	var cfg File
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "yaml"}); err != nil {
		return File{}, fmt.Errorf("unmarshal config file: %w", err)
	}

	return cfg, nil
}

// BuildRunRequest maps configuration data into a validated run request.
func BuildRunRequest(cfg File) (cpgo.RunRequest, error) {
	profileURLString := strings.TrimSpace(cfg.Profile.URL)
	if profileURLString == "" {
		return cpgo.RunRequest{}, fmt.Errorf("profile url is required")
	}

	profileURL, err := url.Parse(profileURLString)
	if err != nil {
		return cpgo.RunRequest{}, fmt.Errorf("parse profile url: %w", err)
	}

	return cpgo.RunRequest{
		Profile: cpgo.ProfileSettings{
			URL:     profileURL,
			Seconds: cfg.Profile.Seconds,
			Headers: cloneHeaders(cfg.Profile.Headers),
		},
		Repository: cpgo.RepositorySettings{
			Owner:      strings.TrimSpace(cfg.Repository.Owner),
			Name:       strings.TrimSpace(cfg.Repository.Name),
			PGOPath:    strings.TrimSpace(cfg.Repository.PGOPath),
			BaseBranch: strings.TrimSpace(cfg.Repository.BaseBranch),
			HeadBranch: strings.TrimSpace(cfg.Repository.HeadBranch),
		},
		PullRequest: cpgo.PullRequestSettings{
			Title:           strings.TrimSpace(cfg.PullRequest.Title),
			Body:            strings.TrimSpace(cfg.PullRequest.Body),
			ManagedByMarker: strings.TrimSpace(cfg.PullRequest.ManagedByMarker),
		},
		Commit: cpgo.CommitSettings{
			Message: strings.TrimSpace(cfg.Commit.Message),
		},
	}, nil
}

// OperationTimeout resolves the total run timeout with defaults.
func OperationTimeout(cfg File) (time.Duration, error) {
	return parseDurationOrDefault(cfg.Runtime.Timeout, defaultOperationTimeout, "runtime timeout")
}

// ProfileHTTPClient builds an HTTP client for remote profile collection.
func ProfileHTTPClient(cfg File) (*http.Client, error) {
	timeout, err := parseDurationOrDefault(cfg.Profile.Timeout, defaultProfileTimeout, "profile timeout")
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Timeout: timeout,
	}, nil
}

// GitHubHTTPClient builds an HTTP client for GitHub API operations.
func GitHubHTTPClient(cfg File) (*http.Client, error) {
	timeout, err := parseDurationOrDefault(cfg.GitHub.Timeout, defaultGitHubTimeout, "github timeout")
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Timeout: timeout,
	}, nil
}

// ReadAppKey loads the GitHub App private key from disk.
func ReadAppKey(cfg File) ([]byte, error) {
	privateKeyPath := strings.TrimSpace(cfg.GitHub.PrivateKeyPath)
	if privateKeyPath == "" {
		return nil, fmt.Errorf("github private key path is required")
	}

	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read github private key: %w", err)
	}

	if len(privateKey) == 0 {
		return nil, fmt.Errorf("github private key is empty")
	}

	return privateKey, nil
}

func parseDurationOrDefault(raw string, defaultValue time.Duration, fieldName string) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}

	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", fieldName, err)
	}

	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", fieldName)
	}

	return parsed, nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}

	return cloned
}
