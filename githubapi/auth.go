package githubapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v77/github"

	"cpgo"
)

const defaultGitHubHTTPTimeout = 30 * time.Second

// AppClientRequest holds GitHub App credentials and repository target data.
type AppClientRequest struct {
	AppID         int64
	PrivateKeyPEM []byte
	Repository    cpgo.RepositoryRef
	HTTPClient    *http.Client
}

func NewClientFromToken(httpClient *http.Client, token string) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("token is required")
	}

	githubClient := github.NewClient(withTimeout(httpClient)).WithAuthToken(token)
	return NewClient(githubClient)
}

func NewClientFromApp(ctx context.Context, req AppClientRequest) (*Client, error) {
	if req.AppID <= 0 {
		return nil, fmt.Errorf("app id must be positive")
	}

	if len(req.PrivateKeyPEM) == 0 {
		return nil, fmt.Errorf("private key is required")
	}

	if err := validateRepositoryRef(req.Repository); err != nil {
		return nil, err
	}

	baseTransport := http.DefaultTransport
	if req.HTTPClient != nil && req.HTTPClient.Transport != nil {
		baseTransport = req.HTTPClient.Transport
	}

	appTransport, err := ghinstallation.NewAppsTransport(baseTransport, req.AppID, req.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("create github app transport: %w", err)
	}

	appHTTPClient := withTransport(req.HTTPClient, appTransport)
	appClient := github.NewClient(appHTTPClient)

	installation, _, err := appClient.Apps.FindRepositoryInstallation(ctx, req.Repository.Owner, req.Repository.Name)
	if err != nil {
		return nil, fmt.Errorf("find repository installation: %w", err)
	}

	installationTransport := ghinstallation.NewFromAppsTransport(appTransport, installation.GetID())
	installationHTTPClient := withTransport(req.HTTPClient, installationTransport)
	installationClient := github.NewClient(installationHTTPClient)

	return NewClient(installationClient)
}

func withTimeout(httpClient *http.Client) *http.Client {
	if httpClient == nil {
		return &http.Client{
			Timeout: defaultGitHubHTTPTimeout,
		}
	}

	httpClientCopy := *httpClient
	if httpClientCopy.Timeout <= 0 {
		httpClientCopy.Timeout = defaultGitHubHTTPTimeout
	}

	return &httpClientCopy
}

func withTransport(httpClient *http.Client, transport http.RoundTripper) *http.Client {
	client := withTimeout(httpClient)
	client.Transport = transport

	return client
}
