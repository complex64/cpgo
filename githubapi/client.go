package githubapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v77/github"

	"cpgo"
)

const (
	fileModeRegular = "100644"
	treeEntryBlob   = "blob"
)

// Client implements repository and pull request ports via GitHub REST APIs.
type Client struct {
	githubClient *github.Client
}

var _ cpgo.BranchWriter = (*Client)(nil)
var _ cpgo.PullRequestService = (*Client)(nil)

func NewClient(githubClient *github.Client) (*Client, error) {
	if githubClient == nil {
		return nil, fmt.Errorf("github client is required")
	}

	return &Client{
		githubClient: githubClient,
	}, nil
}

// DefaultBranch returns the configured repository default branch.
func (client *Client) DefaultBranch(ctx context.Context, repository cpgo.RepositoryRef) (string, error) {
	if err := validateRepositoryRef(repository); err != nil {
		return "", err
	}

	repo, _, err := client.githubClient.Repositories.Get(ctx, repository.Owner, repository.Name)
	if err != nil {
		return "", fmt.Errorf("get repository: %w", err)
	}

	defaultBranch := strings.TrimSpace(repo.GetDefaultBranch())
	if defaultBranch == "" {
		return "", fmt.Errorf("repository default branch is empty")
	}

	return defaultBranch, nil
}

// ReadFile returns raw file bytes from a branch using git object lookups.
func (client *Client) ReadFile(ctx context.Context, req cpgo.ReadFileRequest) (cpgo.ReadFileResult, error) {
	if err := validateRepositoryRef(req.Repository); err != nil {
		return cpgo.ReadFileResult{}, err
	}

	if strings.TrimSpace(req.Branch) == "" {
		return cpgo.ReadFileResult{}, fmt.Errorf("branch is required")
	}

	if strings.TrimSpace(req.Path) == "" {
		return cpgo.ReadFileResult{}, fmt.Errorf("path is required")
	}

	baseCommitSHA, baseTreeSHA, err := client.baseCommitTree(ctx, req.Repository, req.Branch)
	if err != nil {
		return cpgo.ReadFileResult{}, err
	}

	tree, _, err := client.githubClient.Git.GetTree(ctx, req.Repository.Owner, req.Repository.Name, baseTreeSHA, true)
	if err != nil {
		return cpgo.ReadFileResult{}, fmt.Errorf("get base tree for commit %s: %w", baseCommitSHA, err)
	}

	var blobSHA string
	for _, entry := range tree.Entries {
		if entry.GetPath() != req.Path {
			continue
		}

		if entry.GetType() != treeEntryBlob {
			return cpgo.ReadFileResult{}, fmt.Errorf("path %q is not a blob entry", req.Path)
		}

		blobSHA = strings.TrimSpace(entry.GetSHA())
		break
	}

	if blobSHA == "" {
		if tree.GetTruncated() {
			return cpgo.ReadFileResult{}, fmt.Errorf("tree response was truncated while resolving path %q", req.Path)
		}

		return cpgo.ReadFileResult{HasFile: false}, nil
	}

	content, _, err := client.githubClient.Git.GetBlobRaw(ctx, req.Repository.Owner, req.Repository.Name, blobSHA)
	if err != nil {
		if isNotFound(err) {
			return cpgo.ReadFileResult{HasFile: false}, nil
		}

		return cpgo.ReadFileResult{}, fmt.Errorf("get blob %s: %w", blobSHA, err)
	}

	return cpgo.ReadFileResult{
		Content: content,
		HasFile: true,
	}, nil
}

// UpsertFileAndForceBranch writes a commit and force-updates the head ref.
func (client *Client) UpsertFileAndForceBranch(ctx context.Context, req cpgo.UpsertFileRequest) (cpgo.UpsertFileResult, error) {
	if err := validateRepositoryRef(req.Repository); err != nil {
		return cpgo.UpsertFileResult{}, err
	}

	if strings.TrimSpace(req.BaseBranch) == "" {
		return cpgo.UpsertFileResult{}, fmt.Errorf("base branch is required")
	}

	if strings.TrimSpace(req.HeadBranch) == "" {
		return cpgo.UpsertFileResult{}, fmt.Errorf("head branch is required")
	}

	if strings.TrimSpace(req.Path) == "" {
		return cpgo.UpsertFileResult{}, fmt.Errorf("path is required")
	}

	if strings.TrimSpace(req.CommitMessage) == "" {
		return cpgo.UpsertFileResult{}, fmt.Errorf("commit message is required")
	}

	baseCommitSHA, baseTreeSHA, err := client.baseCommitTree(ctx, req.Repository, req.BaseBranch)
	if err != nil {
		return cpgo.UpsertFileResult{}, err
	}

	blobSHA, err := client.createBlob(ctx, req.Repository, req.Content)
	if err != nil {
		return cpgo.UpsertFileResult{}, err
	}

	treeSHA, err := client.createTree(ctx, req, baseTreeSHA, blobSHA)
	if err != nil {
		return cpgo.UpsertFileResult{}, err
	}

	commitSHA, err := client.createCommit(ctx, req, treeSHA, baseCommitSHA)
	if err != nil {
		return cpgo.UpsertFileResult{}, err
	}

	isBranchCreated, err := client.updateHeadRef(ctx, req.Repository, req.HeadBranch, commitSHA)
	if err != nil {
		return cpgo.UpsertFileResult{}, err
	}

	return cpgo.UpsertFileResult{
		CommitSHA:       commitSHA,
		IsBranchCreated: isBranchCreated,
	}, nil
}

// FindOpenByHead resolves an open PR by base/head branch filters.
func (client *Client) FindOpenByHead(ctx context.Context, req cpgo.FindPullRequestRequest) (*cpgo.PullRequest, error) {
	if err := validateRepositoryRef(req.Repository); err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.BaseBranch) == "" {
		return nil, fmt.Errorf("base branch is required")
	}

	if strings.TrimSpace(req.HeadBranch) == "" {
		return nil, fmt.Errorf("head branch is required")
	}

	pullRequests, _, err := client.githubClient.PullRequests.List(ctx, req.Repository.Owner, req.Repository.Name, &github.PullRequestListOptions{
		State: "open",
		Base:  req.BaseBranch,
		Head:  req.Repository.Owner + ":" + req.HeadBranch,
		ListOptions: github.ListOptions{
			PerPage: 1,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}

	if len(pullRequests) == 0 {
		return nil, nil
	}

	pullRequest := pullRequests[0]
	return &cpgo.PullRequest{
		Number: pullRequest.GetNumber(),
		Title:  pullRequest.GetTitle(),
		Body:   pullRequest.GetBody(),
		URL:    pullRequest.GetHTMLURL(),
	}, nil
}

// Create opens a new pull request from head branch to base branch.
func (client *Client) Create(ctx context.Context, req cpgo.CreatePullRequestRequest) (cpgo.PullRequest, error) {
	if err := validateRepositoryRef(req.Repository); err != nil {
		return cpgo.PullRequest{}, err
	}

	if strings.TrimSpace(req.BaseBranch) == "" {
		return cpgo.PullRequest{}, fmt.Errorf("base branch is required")
	}

	if strings.TrimSpace(req.HeadBranch) == "" {
		return cpgo.PullRequest{}, fmt.Errorf("head branch is required")
	}

	if strings.TrimSpace(req.Title) == "" {
		return cpgo.PullRequest{}, fmt.Errorf("pull request title is required")
	}

	if strings.TrimSpace(req.Body) == "" {
		return cpgo.PullRequest{}, fmt.Errorf("pull request body is required")
	}

	pullRequest, _, err := client.githubClient.PullRequests.Create(ctx, req.Repository.Owner, req.Repository.Name, &github.NewPullRequest{
		Title: new(req.Title),
		Head:  new(req.HeadBranch),
		Base:  new(req.BaseBranch),
		Body:  new(req.Body),
	})
	if err != nil {
		return cpgo.PullRequest{}, fmt.Errorf("create pull request: %w", err)
	}

	return cpgo.PullRequest{
		Number: pullRequest.GetNumber(),
		Title:  pullRequest.GetTitle(),
		Body:   pullRequest.GetBody(),
		URL:    pullRequest.GetHTMLURL(),
	}, nil
}

// baseCommitTree fetches the base branch commit and tree SHAs.
func (client *Client) baseCommitTree(ctx context.Context, repository cpgo.RepositoryRef, baseBranch string) (string, string, error) {
	baseRef, _, err := client.githubClient.Git.GetRef(ctx, repository.Owner, repository.Name, "heads/"+baseBranch)
	if err != nil {
		return "", "", fmt.Errorf("get base branch ref: %w", err)
	}

	baseCommitSHA := strings.TrimSpace(baseRef.GetObject().GetSHA())
	if baseCommitSHA == "" {
		return "", "", fmt.Errorf("base branch ref has empty commit sha")
	}

	baseCommit, _, err := client.githubClient.Git.GetCommit(ctx, repository.Owner, repository.Name, baseCommitSHA)
	if err != nil {
		return "", "", fmt.Errorf("get base commit: %w", err)
	}

	baseTreeSHA := strings.TrimSpace(baseCommit.GetTree().GetSHA())
	if baseTreeSHA == "" {
		return "", "", fmt.Errorf("base commit has empty tree sha")
	}

	return baseCommitSHA, baseTreeSHA, nil
}

// createBlob stores profile bytes as a git blob.
func (client *Client) createBlob(ctx context.Context, repository cpgo.RepositoryRef, content []byte) (string, error) {
	encodedContent := base64.StdEncoding.EncodeToString(content)

	blob, _, err := client.githubClient.Git.CreateBlob(ctx, repository.Owner, repository.Name, github.Blob{
		Content:  new(encodedContent),
		Encoding: new("base64"),
	})
	if err != nil {
		return "", fmt.Errorf("create blob: %w", err)
	}

	blobSHA := strings.TrimSpace(blob.GetSHA())
	if blobSHA == "" {
		return "", fmt.Errorf("created blob has empty sha")
	}

	return blobSHA, nil
}

// createTree builds a tree that updates the configured profile path.
func (client *Client) createTree(ctx context.Context, req cpgo.UpsertFileRequest, baseTreeSHA string, blobSHA string) (string, error) {
	tree, _, err := client.githubClient.Git.CreateTree(ctx, req.Repository.Owner, req.Repository.Name, baseTreeSHA, []*github.TreeEntry{
		{
			Path: new(req.Path),
			Mode: new(fileModeRegular),
			Type: new(treeEntryBlob),
			SHA:  new(blobSHA),
		},
	})
	if err != nil {
		return "", fmt.Errorf("create tree: %w", err)
	}

	treeSHA := strings.TrimSpace(tree.GetSHA())
	if treeSHA == "" {
		return "", fmt.Errorf("created tree has empty sha")
	}

	return treeSHA, nil
}

// createCommit creates a commit with the updated tree and base parent.
func (client *Client) createCommit(ctx context.Context, req cpgo.UpsertFileRequest, treeSHA string, parentCommitSHA string) (string, error) {
	commit, _, err := client.githubClient.Git.CreateCommit(ctx, req.Repository.Owner, req.Repository.Name, github.Commit{
		Message: new(req.CommitMessage),
		Tree: &github.Tree{
			SHA: new(treeSHA),
		},
		Parents: []*github.Commit{
			{
				SHA: new(parentCommitSHA),
			},
		},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("create commit: %w", err)
	}

	commitSHA := strings.TrimSpace(commit.GetSHA())
	if commitSHA == "" {
		return "", fmt.Errorf("created commit has empty sha")
	}

	return commitSHA, nil
}

// updateHeadRef force-updates the branch ref, creating it when absent.
func (client *Client) updateHeadRef(ctx context.Context, repository cpgo.RepositoryRef, headBranch string, commitSHA string) (bool, error) {
	_, _, err := client.githubClient.Git.UpdateRef(ctx, repository.Owner, repository.Name, "heads/"+headBranch, github.UpdateRef{
		SHA:   commitSHA,
		Force: new(true),
	})
	if err == nil {
		return false, nil
	}

	if !isNotFound(err) && !isReferenceMissing(err) {
		return false, fmt.Errorf("force update branch ref: %w", err)
	}

	_, _, err = client.githubClient.Git.CreateRef(ctx, repository.Owner, repository.Name, github.CreateRef{
		Ref: "refs/heads/" + headBranch,
		SHA: commitSHA,
	})
	if err == nil {
		return true, nil
	}

	// The branch may have been created concurrently after the initial update attempt.
	_, _, updateErr := client.githubClient.Git.UpdateRef(ctx, repository.Owner, repository.Name, "heads/"+headBranch, github.UpdateRef{
		SHA:   commitSHA,
		Force: new(true),
	})
	if updateErr == nil {
		return false, nil
	}

	return false, fmt.Errorf("create branch ref: %w (retry update failed: %v)", err, updateErr)
}

func isNotFound(err error) bool {
	var githubError *github.ErrorResponse
	if !errors.As(err, &githubError) {
		return false
	}

	if githubError.Response == nil {
		return false
	}

	return githubError.Response.StatusCode == http.StatusNotFound
}

func isReferenceMissing(err error) bool {
	var githubError *github.ErrorResponse
	if !errors.As(err, &githubError) {
		return false
	}

	if githubError.Response == nil || githubError.Response.StatusCode != http.StatusUnprocessableEntity {
		return false
	}

	if strings.Contains(strings.ToLower(githubError.Message), "reference does not exist") {
		return true
	}

	for _, item := range githubError.Errors {
		if strings.Contains(strings.ToLower(item.Message), "reference does not exist") {
			return true
		}
	}

	return false
}

func validateRepositoryRef(repository cpgo.RepositoryRef) error {
	if strings.TrimSpace(repository.Owner) == "" {
		return fmt.Errorf("repository owner is required")
	}

	if strings.TrimSpace(repository.Name) == "" {
		return fmt.Errorf("repository name is required")
	}

	return nil
}
