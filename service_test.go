package cpgo

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestServiceRun(t *testing.T) {
	t.Run("blocks updates when existing pull request is unmanaged", func(t *testing.T) {
		branchWriter := &branchWriterStub{
			defaultBranch: "main",
			readFileResult: ReadFileResult{
				HasFile: false,
			},
		}

		pullRequests := &pullRequestServiceStub{
			findResult: &PullRequest{
				Number: 12,
				Body:   "manual pull request",
			},
		}

		service := mustNewService(t, &profileFetcherStub{profile: []byte("cpu")}, &profileValidatorStub{}, branchWriter, pullRequests)

		_, err := service.Run(context.Background(), newRunRequest(t))
		if !errors.Is(err, ErrUnmanagedPullRequest) {
			t.Fatalf("expected ErrUnmanagedPullRequest, got %v", err)
		}

		if branchWriter.hasUpsertCall {
			t.Fatalf("expected no branch updates for unmanaged pull request")
		}
	})

	t.Run("returns noop when profile already matches base branch file", func(t *testing.T) {
		branchWriter := &branchWriterStub{
			defaultBranch: "main",
			readFileResult: ReadFileResult{
				Content: []byte("same-profile"),
				HasFile: true,
			},
		}

		pullRequests := &pullRequestServiceStub{}
		service := mustNewService(t, &profileFetcherStub{profile: []byte("same-profile")}, &profileValidatorStub{}, branchWriter, pullRequests)

		result, err := service.Run(context.Background(), newRunRequest(t))
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}

		if !result.IsNoop {
			t.Fatalf("expected noop result")
		}

		if result.IsProfileChanged {
			t.Fatalf("expected no profile change")
		}

		if branchWriter.hasUpsertCall {
			t.Fatalf("expected no branch updates for noop run")
		}
	})

	t.Run("creates pull request on profile change when one does not exist", func(t *testing.T) {
		branchWriter := &branchWriterStub{
			defaultBranch: "main",
			readFileResult: ReadFileResult{
				Content: []byte("stale-profile"),
				HasFile: true,
			},
			upsertResult: UpsertFileResult{
				CommitSHA: "abc123",
			},
		}

		pullRequests := &pullRequestServiceStub{
			createResult: PullRequest{
				Number: 22,
			},
		}

		service := mustNewService(t, &profileFetcherStub{profile: []byte("fresh-profile")}, &profileValidatorStub{}, branchWriter, pullRequests)

		result, err := service.Run(context.Background(), newRunRequest(t))
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}

		if !result.IsProfileChanged {
			t.Fatalf("expected profile to be marked as changed")
		}

		if !result.IsPullRequestCreated {
			t.Fatalf("expected pull request creation")
		}

		if result.PullRequestNumber != 22 {
			t.Fatalf("expected pull request number 22, got %d", result.PullRequestNumber)
		}

		if !branchWriter.hasUpsertCall {
			t.Fatalf("expected branch update")
		}

		if branchWriter.upsertRequest.HeadBranch != "cpgo" {
			t.Fatalf("expected default branch name cpgo, got %s", branchWriter.upsertRequest.HeadBranch)
		}

		if !pullRequests.hasCreateCall {
			t.Fatalf("expected create pull request call")
		}

		if !strings.Contains(pullRequests.createRequest.Body, defaultManagedByMarker) {
			t.Fatalf("expected managed-by marker in pull request body")
		}
	})

	t.Run("updates managed pull request without creating a new one", func(t *testing.T) {
		branchWriter := &branchWriterStub{
			defaultBranch: "main",
			readFileResult: ReadFileResult{
				Content: []byte("old-profile"),
				HasFile: true,
			},
			upsertResult: UpsertFileResult{
				CommitSHA: "def456",
			},
		}

		pullRequests := &pullRequestServiceStub{
			findResult: &PullRequest{
				Number: 99,
				Body:   "Automated PGO profile refresh.\n\n<!-- managed-by:cpgo -->",
			},
		}

		service := mustNewService(t, &profileFetcherStub{profile: []byte("new-profile")}, &profileValidatorStub{}, branchWriter, pullRequests)

		result, err := service.Run(context.Background(), newRunRequest(t))
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}

		if result.PullRequestNumber != 99 {
			t.Fatalf("expected existing pull request number 99, got %d", result.PullRequestNumber)
		}

		if result.IsPullRequestCreated {
			t.Fatalf("expected no pull request creation")
		}

		if pullRequests.hasCreateCall {
			t.Fatalf("expected no create pull request call")
		}
	})
}

func mustNewService(
	t *testing.T,
	profileFetcher ProfileFetcher,
	profileValidator ProfileValidator,
	branchWriter BranchWriter,
	pullRequests PullRequestService,
) *Service {
	t.Helper()

	service, err := NewService(Dependencies{
		ProfileFetcher:   profileFetcher,
		ProfileValidator: profileValidator,
		BranchWriter:     branchWriter,
		PullRequests:     pullRequests,
	})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	return service
}

func newRunRequest(t *testing.T) RunRequest {
	t.Helper()

	profileURL, err := url.Parse("https://service.example.com/debug/pprof/profile")
	if err != nil {
		t.Fatalf("failed to parse profile url: %v", err)
	}

	return RunRequest{
		Profile: ProfileSettings{
			URL: profileURL,
		},
		Repository: RepositorySettings{
			Owner:   "acme",
			Name:    "payments",
			PGOPath: "default.pgo",
		},
	}
}

// profileFetcherStub injects deterministic profile fetch behavior.
type profileFetcherStub struct {
	profile []byte
	err     error
}

// FetchCPUProfile returns the configured payload for test scenarios.
func (stub *profileFetcherStub) FetchCPUProfile(context.Context, FetchProfileRequest) ([]byte, error) {
	return append([]byte(nil), stub.profile...), stub.err
}

// profileValidatorStub injects deterministic profile validation behavior.
type profileValidatorStub struct {
	err error
}

// ValidateCPUProfile returns the configured validation error.
func (stub *profileValidatorStub) ValidateCPUProfile([]byte) error {
	return stub.err
}

// branchWriterStub captures and returns deterministic branch operations.
type branchWriterStub struct {
	defaultBranch  string
	defaultErr     error
	readFileResult ReadFileResult
	readFileErr    error
	upsertResult   UpsertFileResult
	upsertErr      error
	upsertRequest  UpsertFileRequest
	hasUpsertCall  bool
}

// DefaultBranch returns the stubbed base branch value.
func (stub *branchWriterStub) DefaultBranch(context.Context, RepositoryRef) (string, error) {
	return stub.defaultBranch, stub.defaultErr
}

// ReadFile returns the stubbed file read result.
func (stub *branchWriterStub) ReadFile(context.Context, ReadFileRequest) (ReadFileResult, error) {
	return stub.readFileResult, stub.readFileErr
}

// UpsertFileAndForceBranch records and returns stubbed write results.
func (stub *branchWriterStub) UpsertFileAndForceBranch(_ context.Context, req UpsertFileRequest) (UpsertFileResult, error) {
	stub.hasUpsertCall = true
	stub.upsertRequest = req
	return stub.upsertResult, stub.upsertErr
}

// pullRequestServiceStub captures and returns deterministic PR operations.
type pullRequestServiceStub struct {
	findResult    *PullRequest
	findErr       error
	createResult  PullRequest
	createErr     error
	createRequest CreatePullRequestRequest
	hasCreateCall bool
}

// FindOpenByHead returns the stubbed pull request lookup result.
func (stub *pullRequestServiceStub) FindOpenByHead(context.Context, FindPullRequestRequest) (*PullRequest, error) {
	return stub.findResult, stub.findErr
}

// Create records and returns the stubbed pull request creation result.
func (stub *pullRequestServiceStub) Create(_ context.Context, req CreatePullRequestRequest) (PullRequest, error) {
	stub.hasCreateCall = true
	stub.createRequest = req
	return stub.createResult, stub.createErr
}
