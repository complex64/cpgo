package cpgo

import (
	"context"
	"net/url"
)

// ProfileFetcher retrieves raw CPU profile data from a source endpoint.
type ProfileFetcher interface {
	// FetchCPUProfile returns CPU profile bytes for one sampling request.
	FetchCPUProfile(ctx context.Context, req FetchProfileRequest) ([]byte, error)
}

// FetchProfileRequest defines a CPU profile fetch operation.
type FetchProfileRequest struct {
	URL     *url.URL
	Seconds int
	Headers map[string]string
}

// ProfileValidator verifies that a fetched payload is a usable CPU profile.
type ProfileValidator interface {
	// ValidateCPUProfile rejects malformed or unusable profile bytes.
	ValidateCPUProfile(raw []byte) error
}

// RepositoryRef uniquely identifies a repository.
type RepositoryRef struct {
	Owner string
	Name  string
}

// BranchWriter mutates and reads repository contents through a branch workflow.
type BranchWriter interface {
	// DefaultBranch resolves the repository default branch name.
	DefaultBranch(ctx context.Context, repository RepositoryRef) (string, error)
	// ReadFile reads file contents from a specific branch.
	ReadFile(ctx context.Context, req ReadFileRequest) (ReadFileResult, error)
	// UpsertFileAndForceBranch writes a file commit and updates the head branch.
	UpsertFileAndForceBranch(ctx context.Context, req UpsertFileRequest) (UpsertFileResult, error)
}

// ReadFileRequest selects a file on a specific repository branch.
type ReadFileRequest struct {
	Repository RepositoryRef
	Branch     string
	Path       string
}

// ReadFileResult represents an optional file read.
type ReadFileResult struct {
	Content []byte
	HasFile bool
}

// UpsertFileRequest describes a force-update operation for a branch file.
type UpsertFileRequest struct {
	Repository    RepositoryRef
	BaseBranch    string
	HeadBranch    string
	Path          string
	Content       []byte
	CommitMessage string
}

// UpsertFileResult reports the branch update outcome.
type UpsertFileResult struct {
	CommitSHA       string
	IsBranchCreated bool
}

// PullRequestService manages pull requests for the cpgo branch.
type PullRequestService interface {
	// FindOpenByHead finds the open PR that matches base/head pair.
	FindOpenByHead(ctx context.Context, req FindPullRequestRequest) (*PullRequest, error)
	// Create opens a new pull request for the prepared branch.
	Create(ctx context.Context, req CreatePullRequestRequest) (PullRequest, error)
}

// FindPullRequestRequest targets a PR lookup by repository branches.
type FindPullRequestRequest struct {
	Repository RepositoryRef
	BaseBranch string
	HeadBranch string
}

// PullRequest holds the subset of PR metadata used by cpgo.
type PullRequest struct {
	Number int
	Title  string
	Body   string
	URL    string
}

// CreatePullRequestRequest contains fields for opening a PR.
type CreatePullRequestRequest struct {
	Repository RepositoryRef
	BaseBranch string
	HeadBranch string
	Title      string
	Body       string
}
