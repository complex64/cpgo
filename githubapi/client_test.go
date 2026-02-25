package githubapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v77/github"

	"cpgo"
)

func TestClientReadFile(t *testing.T) {
	t.Run("returns missing file when target path does not exist in base tree", func(t *testing.T) {
		githubClient := newGitHubClient(t, http.HandlerFunc(func(response http.ResponseWriter, req *http.Request) {
			switch req.URL.Path {
			case "/repos/acme/payments/git/ref/heads/main":
				_, _ = response.Write([]byte(`{"ref":"refs/heads/main","object":{"type":"commit","sha":"base-commit"}}`))
			case "/repos/acme/payments/git/commits/base-commit":
				_, _ = response.Write([]byte(`{"sha":"base-commit","tree":{"sha":"base-tree"}}`))
			case "/repos/acme/payments/git/trees/base-tree":
				_, _ = response.Write([]byte(`{"sha":"base-tree","truncated":false,"tree":[{"path":"README.md","type":"blob","sha":"readme-sha"}]}`))
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
		}))

		client := mustNewClient(t, githubClient)
		result, err := client.ReadFile(context.Background(), cpgo.ReadFileRequest{
			Repository: cpgo.RepositoryRef{
				Owner: "acme",
				Name:  "payments",
			},
			Branch: "main",
			Path:   "default.pgo",
		})
		if err != nil {
			t.Fatalf("read file: %v", err)
		}

		if result.HasFile {
			t.Fatalf("expected missing file result")
		}
	})

	t.Run("reads file bytes when target path exists", func(t *testing.T) {
		githubClient := newGitHubClient(t, http.HandlerFunc(func(response http.ResponseWriter, req *http.Request) {
			switch req.URL.Path {
			case "/repos/acme/payments/git/ref/heads/main":
				_, _ = response.Write([]byte(`{"ref":"refs/heads/main","object":{"type":"commit","sha":"base-commit"}}`))
			case "/repos/acme/payments/git/commits/base-commit":
				_, _ = response.Write([]byte(`{"sha":"base-commit","tree":{"sha":"base-tree"}}`))
			case "/repos/acme/payments/git/trees/base-tree":
				_, _ = response.Write([]byte(`{"sha":"base-tree","truncated":false,"tree":[{"path":"default.pgo","type":"blob","sha":"pgo-sha"}]}`))
			case "/repos/acme/payments/git/blobs/pgo-sha":
				_, _ = response.Write([]byte("profile-bytes"))
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
		}))

		client := mustNewClient(t, githubClient)
		result, err := client.ReadFile(context.Background(), cpgo.ReadFileRequest{
			Repository: cpgo.RepositoryRef{
				Owner: "acme",
				Name:  "payments",
			},
			Branch: "main",
			Path:   "default.pgo",
		})
		if err != nil {
			t.Fatalf("read file: %v", err)
		}

		if !result.HasFile {
			t.Fatalf("expected file result")
		}

		if string(result.Content) != "profile-bytes" {
			t.Fatalf("expected profile bytes, got %q", string(result.Content))
		}
	})
}

func TestClientFindOpenByHead(t *testing.T) {
	githubClient := newGitHubClient(t, http.HandlerFunc(func(response http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/repos/acme/payments/pulls" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}

		query := req.URL.Query()
		if query.Get("state") != "open" {
			t.Fatalf("expected open state filter, got %s", query.Get("state"))
		}

		if query.Get("base") != "main" {
			t.Fatalf("expected base filter main, got %s", query.Get("base"))
		}

		if query.Get("head") != "acme:cpgo" {
			t.Fatalf("expected head filter acme:cpgo, got %s", query.Get("head"))
		}

		_, _ = response.Write([]byte(`[{"number":42,"title":"perf(pgo): refresh pgo profile","body":"Automated PGO profile refresh.","html_url":"https://github.com/acme/payments/pull/42"}]`))
	}))

	client := mustNewClient(t, githubClient)
	pullRequest, err := client.FindOpenByHead(context.Background(), cpgo.FindPullRequestRequest{
		Repository: cpgo.RepositoryRef{
			Owner: "acme",
			Name:  "payments",
		},
		BaseBranch: "main",
		HeadBranch: "cpgo",
	})
	if err != nil {
		t.Fatalf("find pull request: %v", err)
	}

	if pullRequest == nil {
		t.Fatalf("expected pull request")
	}

	if pullRequest.Number != 42 {
		t.Fatalf("expected pull request number 42, got %d", pullRequest.Number)
	}
}

func TestClientUpsertFileAndForceBranch(t *testing.T) {
	encodedProfile := base64.StdEncoding.EncodeToString([]byte("new-profile"))
	createRefCalled := false

	githubClient := newGitHubClient(t, http.HandlerFunc(func(response http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/repos/acme/payments/git/ref/heads/main":
			_, _ = response.Write([]byte(`{"ref":"refs/heads/main","object":{"type":"commit","sha":"base-commit"}}`))
		case "/repos/acme/payments/git/commits/base-commit":
			_, _ = response.Write([]byte(`{"sha":"base-commit","tree":{"sha":"base-tree"}}`))
		case "/repos/acme/payments/git/blobs":
			var payload struct {
				Content  string `json:"content"`
				Encoding string `json:"encoding"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode blob request: %v", err)
			}

			if payload.Encoding != "base64" {
				t.Fatalf("expected base64 encoding, got %s", payload.Encoding)
			}

			if payload.Content != encodedProfile {
				t.Fatalf("expected encoded profile content")
			}

			_, _ = response.Write([]byte(`{"sha":"blob-sha"}`))
		case "/repos/acme/payments/git/trees":
			_, _ = response.Write([]byte(`{"sha":"tree-sha"}`))
		case "/repos/acme/payments/git/commits":
			_, _ = response.Write([]byte(`{"sha":"commit-sha"}`))
		case "/repos/acme/payments/git/refs/heads/cpgo":
			response.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = response.Write([]byte(`{"message":"Reference does not exist","errors":[]}`))
		case "/repos/acme/payments/git/refs":
			createRefCalled = true
			var payload struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create ref request: %v", err)
			}

			if payload.Ref != "refs/heads/cpgo" {
				t.Fatalf("expected refs/heads/cpgo, got %s", payload.Ref)
			}

			if payload.SHA != "commit-sha" {
				t.Fatalf("expected commit-sha, got %s", payload.SHA)
			}

			_, _ = response.Write([]byte(`{"ref":"refs/heads/cpgo","object":{"type":"commit","sha":"commit-sha"}}`))
		default:
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
	}))

	client := mustNewClient(t, githubClient)
	result, err := client.UpsertFileAndForceBranch(context.Background(), cpgo.UpsertFileRequest{
		Repository: cpgo.RepositoryRef{
			Owner: "acme",
			Name:  "payments",
		},
		BaseBranch:    "main",
		HeadBranch:    "cpgo",
		Path:          "default.pgo",
		Content:       []byte("new-profile"),
		CommitMessage: "perf(pgo): refresh pgo profile",
	})
	if err != nil {
		t.Fatalf("upsert file: %v", err)
	}

	if result.CommitSHA != "commit-sha" {
		t.Fatalf("expected commit-sha, got %s", result.CommitSHA)
	}

	if !result.IsBranchCreated {
		t.Fatalf("expected branch creation")
	}

	if !createRefCalled {
		t.Fatalf("expected create ref call")
	}
}

func mustNewClient(t *testing.T, githubClient *github.Client) *Client {
	t.Helper()

	client, err := NewClient(githubClient)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	return client
}

func newGitHubClient(t *testing.T, handler http.Handler) *github.Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	client := github.NewClient(server.Client())
	client.BaseURL = baseURL
	client.UploadURL = baseURL

	return client
}
