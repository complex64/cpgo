package cpgo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrUnmanagedPullRequest = errors.New("existing pull request is not managed by cpgo")

// Dependencies bundles runtime ports required by Service.
type Dependencies struct {
	ProfileFetcher   ProfileFetcher
	ProfileValidator ProfileValidator
	BranchWriter     BranchWriter
	PullRequests     PullRequestService
}

// Service orchestrates one cpgo execution using injected ports.
type Service struct {
	profileFetcher   ProfileFetcher
	profileValidator ProfileValidator
	branchWriter     BranchWriter
	pullRequests     PullRequestService
}

// RunResult summarizes what changed during one run.
type RunResult struct {
	BaseBranch           string
	HeadBranch           string
	PullRequestNumber    int
	CommitSHA            string
	IsProfileChanged     bool
	IsPullRequestCreated bool
	IsNoop               bool
}

// NewService validates dependencies and returns an executable service.
func NewService(deps Dependencies) (*Service, error) {
	switch {
	case deps.ProfileFetcher == nil:
		return nil, fmt.Errorf("profile fetcher is required")
	case deps.ProfileValidator == nil:
		return nil, fmt.Errorf("profile validator is required")
	case deps.BranchWriter == nil:
		return nil, fmt.Errorf("branch writer is required")
	case deps.PullRequests == nil:
		return nil, fmt.Errorf("pull request service is required")
	}

	return &Service{
		profileFetcher:   deps.ProfileFetcher,
		profileValidator: deps.ProfileValidator,
		branchWriter:     deps.BranchWriter,
		pullRequests:     deps.PullRequests,
	}, nil
}

// Run executes a full fetch-validate-write-pr cycle for one request.
func (svc *Service) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	normalized, err := req.normalized()
	if err != nil {
		return RunResult{}, err
	}

	repository := RepositoryRef{
		Owner: normalized.Repository.Owner,
		Name:  normalized.Repository.Name,
	}

	profile, err := svc.profileFetcher.FetchCPUProfile(ctx, FetchProfileRequest{
		URL:     normalized.Profile.URL,
		Seconds: normalized.Profile.Seconds,
		Headers: normalized.Profile.Headers,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("fetch cpu profile: %w", err)
	}

	if err := svc.profileValidator.ValidateCPUProfile(profile); err != nil {
		return RunResult{}, fmt.Errorf("validate cpu profile: %w", err)
	}

	baseBranch, err := svc.resolveBaseBranch(ctx, repository, normalized.Repository.BaseBranch)
	if err != nil {
		return RunResult{}, err
	}

	openPR, err := svc.pullRequests.FindOpenByHead(ctx, FindPullRequestRequest{
		Repository: repository,
		BaseBranch: baseBranch,
		HeadBranch: normalized.Repository.HeadBranch,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("find open pull request: %w", err)
	}

	if openPR != nil && !strings.Contains(openPR.Body, normalized.PullRequest.ManagedByMarker) {
		return RunResult{}, ErrUnmanagedPullRequest
	}

	readResult, err := svc.branchWriter.ReadFile(ctx, ReadFileRequest{
		Repository: repository,
		Branch:     baseBranch,
		Path:       normalized.Repository.PGOPath,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("read base branch pgo file: %w", err)
	}

	if readResult.HasFile && bytes.Equal(readResult.Content, profile) {
		return RunResult{
			BaseBranch:        baseBranch,
			HeadBranch:        normalized.Repository.HeadBranch,
			PullRequestNumber: prNumber(openPR),
			IsNoop:            true,
		}, nil
	}

	writeResult, err := svc.branchWriter.UpsertFileAndForceBranch(ctx, UpsertFileRequest{
		Repository:    repository,
		BaseBranch:    baseBranch,
		HeadBranch:    normalized.Repository.HeadBranch,
		Path:          normalized.Repository.PGOPath,
		Content:       profile,
		CommitMessage: normalized.Commit.Message,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("update pgo branch: %w", err)
	}

	result := RunResult{
		BaseBranch:       baseBranch,
		HeadBranch:       normalized.Repository.HeadBranch,
		CommitSHA:        writeResult.CommitSHA,
		IsProfileChanged: true,
	}

	if openPR != nil {
		result.PullRequestNumber = openPR.Number
		return result, nil
	}

	createdPR, err := svc.pullRequests.Create(ctx, CreatePullRequestRequest{
		Repository: repository,
		BaseBranch: baseBranch,
		HeadBranch: normalized.Repository.HeadBranch,
		Title:      normalized.PullRequest.Title,
		Body:       appendMarker(normalized.PullRequest.Body, normalized.PullRequest.ManagedByMarker),
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("create pull request: %w", err)
	}

	result.PullRequestNumber = createdPR.Number
	result.IsPullRequestCreated = true

	return result, nil
}

// resolveBaseBranch picks the configured base or repository default branch.
func (svc *Service) resolveBaseBranch(ctx context.Context, repository RepositoryRef, baseBranchCfg string) (string, error) {
	if strings.TrimSpace(baseBranchCfg) != "" {
		return baseBranchCfg, nil
	}

	baseBranch, err := svc.branchWriter.DefaultBranch(ctx, repository)
	if err != nil {
		return "", fmt.Errorf("resolve default branch: %w", err)
	}

	return baseBranch, nil
}

func prNumber(existing *PullRequest) int {
	if existing == nil {
		return 0
	}

	return existing.Number
}

func appendMarker(body string, marker string) string {
	if strings.Contains(body, marker) {
		return body
	}

	if strings.TrimSpace(body) == "" {
		return marker
	}

	return strings.TrimRight(body, "\n") + "\n\n" + marker
}
