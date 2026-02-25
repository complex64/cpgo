package cpgo

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	defaultProfileSeconds  = 30
	defaultHeadBranch      = "cpgo"
	defaultManagedByMarker = "<!-- managed-by:cpgo -->"
	defaultPRTitle         = "perf(pgo): refresh pgo profile"
	defaultPRBody          = "Automated PGO profile refresh."
	defaultCommitMessage   = "perf(pgo): refresh pgo profile"
)

// RunRequest captures one complete cpgo refresh operation.
type RunRequest struct {
	Profile     ProfileSettings
	Repository  RepositorySettings
	PullRequest PullRequestSettings
	Commit      CommitSettings
}

// ProfileSettings describes where and how to collect the CPU profile.
type ProfileSettings struct {
	URL     *url.URL
	Seconds int
	Headers map[string]string
}

// RepositorySettings identifies the target repository and branch strategy.
type RepositorySettings struct {
	Owner      string
	Name       string
	PGOPath    string
	BaseBranch string
	HeadBranch string
}

// PullRequestSettings controls the automation PR identity and metadata.
type PullRequestSettings struct {
	Title           string
	Body            string
	ManagedByMarker string
}

// CommitSettings defines commit metadata for profile updates.
type CommitSettings struct {
	Message string
}

// normalized validates required fields and applies cpgo defaults.
func (req RunRequest) normalized() (RunRequest, error) {
	normalized := req

	if normalized.Profile.URL == nil {
		return RunRequest{}, fmt.Errorf("profile url is required")
	}

	if normalized.Profile.URL.Scheme == "" || normalized.Profile.URL.Host == "" {
		return RunRequest{}, fmt.Errorf("profile url must include scheme and host")
	}

	if normalized.Profile.Seconds <= 0 {
		normalized.Profile.Seconds = defaultProfileSeconds
	}

	if strings.TrimSpace(normalized.Repository.Owner) == "" {
		return RunRequest{}, fmt.Errorf("repository owner is required")
	}

	if strings.TrimSpace(normalized.Repository.Name) == "" {
		return RunRequest{}, fmt.Errorf("repository name is required")
	}

	if strings.TrimSpace(normalized.Repository.PGOPath) == "" {
		return RunRequest{}, fmt.Errorf("repository pgo path is required")
	}

	if strings.TrimSpace(normalized.Repository.HeadBranch) == "" {
		normalized.Repository.HeadBranch = defaultHeadBranch
	}

	if strings.TrimSpace(normalized.PullRequest.ManagedByMarker) == "" {
		normalized.PullRequest.ManagedByMarker = defaultManagedByMarker
	}

	if strings.TrimSpace(normalized.PullRequest.Title) == "" {
		normalized.PullRequest.Title = defaultPRTitle
	}

	if strings.TrimSpace(normalized.PullRequest.Body) == "" {
		normalized.PullRequest.Body = defaultPRBody
	}

	if strings.TrimSpace(normalized.Commit.Message) == "" {
		normalized.Commit.Message = defaultCommitMessage
	}

	return normalized, nil
}
