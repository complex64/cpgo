package pprofio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cpgo"
)

const defaultHTTPClientTimeout = 45 * time.Second

// Fetcher collects CPU profiles from remote pprof HTTP endpoints.
type Fetcher struct {
	httpClient *http.Client
}

var _ cpgo.ProfileFetcher = (*Fetcher)(nil)

// NewFetcher returns a fetcher with a sane default timeout.
func NewFetcher(httpClient *http.Client) *Fetcher {
	return &Fetcher{
		httpClient: withDefaultTimeout(httpClient),
	}
}

// FetchCPUProfile requests a single CPU profile sample window.
func (fetcher *Fetcher) FetchCPUProfile(ctx context.Context, req cpgo.FetchProfileRequest) ([]byte, error) {
	if req.URL == nil {
		return nil, fmt.Errorf("profile url is required")
	}

	if req.Seconds <= 0 {
		return nil, fmt.Errorf("profile seconds must be positive")
	}

	profileURL := withProfileSeconds(*req.URL, req.Seconds)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build profile request: %w", err)
	}

	for key, value := range req.Headers {
		if strings.TrimSpace(key) == "" {
			continue
		}

		httpReq.Header.Set(key, value)
	}

	resp, err := fetcher.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fetch profile: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		preview, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		if readErr != nil {
			return nil, fmt.Errorf("fetch profile: unexpected status %s", resp.Status)
		}

		return nil, fmt.Errorf("fetch profile: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(preview)))
	}

	profile, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read profile response: %w", err)
	}

	if len(profile) == 0 {
		return nil, fmt.Errorf("profile response is empty")
	}

	return profile, nil
}

func withDefaultTimeout(httpClient *http.Client) *http.Client {
	if httpClient == nil {
		return &http.Client{
			Timeout: defaultHTTPClientTimeout,
		}
	}

	httpClientCopy := *httpClient
	if httpClientCopy.Timeout <= 0 {
		httpClientCopy.Timeout = defaultHTTPClientTimeout
	}

	return &httpClientCopy
}

func withProfileSeconds(baseURL url.URL, seconds int) url.URL {
	query := baseURL.Query()
	query.Set("seconds", strconv.Itoa(seconds))
	baseURL.RawQuery = query.Encode()

	return baseURL
}
