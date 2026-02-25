package pprofio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cpgo"
)

func TestFetcherFetchCPUProfile(t *testing.T) {
	t.Run("fetches profile with seconds query and headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodGet {
				t.Fatalf("expected GET request, got %s", req.Method)
			}

			if req.URL.Query().Get("seconds") != "17" {
				t.Fatalf("expected seconds query to be 17, got %s", req.URL.Query().Get("seconds"))
			}

			if req.Header.Get("Authorization") != "Bearer token" {
				t.Fatalf("expected authorization header")
			}

			_, _ = resp.Write([]byte("profile-bytes"))
		}))
		t.Cleanup(server.Close)

		profileURL, err := url.Parse(server.URL + "/debug/pprof/profile")
		if err != nil {
			t.Fatalf("parse profile url: %v", err)
		}

		fetcher := NewFetcher(server.Client())
		profile, err := fetcher.FetchCPUProfile(context.Background(), cpgo.FetchProfileRequest{
			URL:     profileURL,
			Seconds: 17,
			Headers: map[string]string{
				"Authorization": "Bearer token",
			},
		})
		if err != nil {
			t.Fatalf("fetch profile: %v", err)
		}

		if string(profile) != "profile-bytes" {
			t.Fatalf("expected profile bytes, got %q", string(profile))
		}
	})

	t.Run("returns error on non-success status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			http.Error(resp, "profile endpoint unavailable", http.StatusBadGateway)
		}))
		t.Cleanup(server.Close)

		profileURL, err := url.Parse(server.URL + "/debug/pprof/profile")
		if err != nil {
			t.Fatalf("parse profile url: %v", err)
		}

		fetcher := NewFetcher(server.Client())
		_, err = fetcher.FetchCPUProfile(context.Background(), cpgo.FetchProfileRequest{
			URL:     profileURL,
			Seconds: 10,
		})
		if err == nil {
			t.Fatalf("expected fetch error")
		}
	})
}
