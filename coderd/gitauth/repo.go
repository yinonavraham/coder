package gitauth

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/go-github/v43/github"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

// Repository represents a repository returned by gitauth.
type Repository struct {
	// ExternalURL is a link to a repo.
	// e.g. https://github.com/coder/coder
	ExternalURL string
	// Owner is the organization or namespace name.
	// e.g. coder
	Owner string
	// Name is the repository name.
	// e.g. code-server
	Name        string
	Description string
	// Stars is the number of stargazers a repository has.
	// If stars is not a thing in the provider, this
	// field will be nil.
	Stars *int
}

// RepositoriesOptions are required fields to be passed
// when listing repositories.
type RepositoriesOptions struct {
	Query      string
	HTTPClient *http.Client
	Page       int
}

// Repositories returns repositories matching a specific query.
// The bool returned will be true if there are more results
// than are returned. This is for pagination.
func Repositories(ctx context.Context, config *Config, opts RepositoriesOptions) ([]Repository, bool, error) {
	switch config.Type {
	case codersdk.GitProviderGitHub:
		client := github.NewClient(opts.HTTPClient)
		// If we're not using hosted GitHub, we create a
		// GitHub Enterprise client pointed at the base URL.
		if config.BaseURL != defaultBaseURL[config.Type] {
			parsed, err := url.Parse(config.BaseURL)
			if err != nil {
				return nil, false, err
			}
			parsed, err = parsed.Parse("/")
			if err != nil {
				return nil, false, err
			}
			client, err = github.NewEnterpriseClient(parsed.String(), "", opts.HTTPClient)
			if err != nil {
				return nil, false, xerrors.Errorf("create enterprise client: %w", err)
			}
		}
		var result []*github.Repository
		var resp *github.Response
		if opts.Query != "" {
			var searchResult *github.RepositoriesSearchResult
			var err error
			searchResult, resp, err = client.Search.Repositories(ctx, opts.Query, &github.SearchOptions{
				ListOptions: github.ListOptions{
					Page: opts.Page,
				},
			})
			if err != nil {
				return nil, false, xerrors.Errorf("request repos: %w", err)
			}
			result = searchResult.Repositories
		} else {
			var err error
			result, resp, err = client.Repositories.List(ctx, "", &github.RepositoryListOptions{
				ListOptions: github.ListOptions{
					Page: opts.Page,
				},
			})
			if err != nil {
				return nil, false, xerrors.Errorf("list repositories: %w", err)
			}
		}
		repos := make([]Repository, 0, len(result))
		for _, repo := range result {
			repos = append(repos, Repository{
				ExternalURL: repo.GetHTMLURL(),
				Owner:       repo.Owner.GetLogin(),
				Name:        repo.GetName(),
				Description: repo.GetDescription(),
				Stars:       repo.StargazersCount,
			})
		}
		return repos, resp.NextPage != 0, nil
	default:
		return nil, false, xerrors.Errorf("%q does not support listing repos", config.Type)
	}
}
