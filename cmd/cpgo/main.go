package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"cpgo"
	"cpgo/githubapi"
	"cpgo/pprofio"
)

func main() {
	logger := newLogger(os.Stderr)
	if err := run(context.Background(), os.Args[1:], os.Stdout, logger); err != nil {
		logger.Error().Err(err).Msg("cpgo run failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, logger zerolog.Logger) error {
	flagSet := flag.NewFlagSet("cpgo", flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)

	var configPath string
	flagSet.StringVar(&configPath, "config", "", "Path to cpgo YAML configuration file.")

	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(configPath) == "" {
		return fmt.Errorf("config path is required")
	}

	config, err := Load(configPath)
	if err != nil {
		return err
	}

	req, err := BuildRunRequest(config)
	if err != nil {
		return err
	}

	timeout, err := OperationTimeout(config)
	if err != nil {
		return err
	}

	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	logger.Info().Str("config_path", configPath).Msg("starting cpgo run")

	svc, err := newService(runContext, config, req.Repository)
	if err != nil {
		return err
	}

	result, err := svc.Run(runContext, req)
	if err != nil {
		return err
	}

	logger.Info().
		Str("base_branch", result.BaseBranch).
		Str("head_branch", result.HeadBranch).
		Int("pr_number", result.PullRequestNumber).
		Str("commit_sha", result.CommitSHA).
		Bool("changed", result.IsProfileChanged).
		Bool("pr_created", result.IsPullRequestCreated).
		Bool("noop", result.IsNoop).
		Msg("completed cpgo run")

	_, _ = fmt.Fprintf(
		stdout,
		"base_branch=%s head_branch=%s pr_number=%d commit_sha=%s changed=%t pr_created=%t noop=%t\n",
		result.BaseBranch,
		result.HeadBranch,
		result.PullRequestNumber,
		result.CommitSHA,
		result.IsProfileChanged,
		result.IsPullRequestCreated,
		result.IsNoop,
	)

	return nil
}

func newService(ctx context.Context, config File, repository cpgo.RepositorySettings) (*cpgo.Service, error) {
	profileClient, err := ProfileHTTPClient(config)
	if err != nil {
		return nil, err
	}

	ghClient, err := GitHubHTTPClient(config)
	if err != nil {
		return nil, err
	}

	ghAdapter, err := newGitHubAdapter(ctx, config, repository, ghClient)
	if err != nil {
		return nil, err
	}

	return cpgo.NewService(cpgo.Dependencies{
		ProfileFetcher:   pprofio.NewFetcher(profileClient),
		ProfileValidator: pprofio.NewValidator(),
		BranchWriter:     ghAdapter,
		PullRequests:     ghAdapter,
	})
}

func newGitHubAdapter(
	ctx context.Context,
	config File,
	repository cpgo.RepositorySettings,
	httpClient *http.Client,
) (*githubapi.Client, error) {
	token := strings.TrimSpace(config.GitHub.Token)
	if token != "" {
		return githubapi.NewClientFromToken(httpClient, token)
	}

	if config.GitHub.AppID <= 0 {
		return nil, fmt.Errorf("github app id must be positive when token is not configured")
	}

	appKeyPEM, err := ReadAppKey(config)
	if err != nil {
		return nil, err
	}

	return githubapi.NewClientFromApp(ctx, githubapi.AppClientRequest{
		AppID:         config.GitHub.AppID,
		PrivateKeyPEM: appKeyPEM,
		Repository: cpgo.RepositoryRef{
			Owner: repository.Owner,
			Name:  repository.Name,
		},
		HTTPClient: httpClient,
	})
}

func newLogger(output io.Writer) zerolog.Logger {
	return zerolog.New(zerolog.ConsoleWriter{
		Out:        output,
		TimeFormat: time.RFC3339,
	}).With().Timestamp().Logger()
}
