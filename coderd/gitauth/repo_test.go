package gitauth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v43/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestRepos(t *testing.T) {
	t.Parallel()

	githubRepo := &github.Repository{
		Name: github.String("coder"),
		Owner: &github.User{
			Login: github.String("coder"),
		},
		Description:     github.String("hello world"),
		HTMLURL:         github.String("https://github.com/coder/coder"),
		StargazersCount: github.Int(1000),
	}
	gitauthRepo := gitauth.Repository{
		Owner:       "coder",
		Name:        "coder",
		Description: "hello world",
		ExternalURL: "https://github.com/coder/coder",
		Stars:       github.Int(1000),
	}

	t.Run("GitHubWithoutQuery", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Ensure that the URL path is for Enterprise!
			assert.Equal(t, "/api/v3/user/repos", r.URL.Path)
			data, err := json.Marshal([]*github.Repository{githubRepo})
			assert.NoError(t, err)
			w.Write(data)
		}))
		t.Cleanup(srv.Close)
		ctx, cancelFunc := testutil.Context(t)
		defer cancelFunc()
		repos, more, err := gitauth.Repositories(ctx, &gitauth.Config{
			Type:    codersdk.GitProviderGitHub,
			BaseURL: srv.URL,
		}, gitauth.RepositoriesOptions{
			HTTPClient: http.DefaultClient,
		})
		require.NoError(t, err)
		require.False(t, more)
		require.Len(t, repos, 1)
		require.Equal(t, gitauthRepo, repos[0])
	})

	t.Run("GitHubWithQuery", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Ensure that the URL path is for Enterprise!
			assert.Equal(t, "/api/v3/search/repositories", r.URL.Path)
			data, err := json.Marshal(&github.RepositoriesSearchResult{
				Repositories: []*github.Repository{githubRepo},
			})
			assert.NoError(t, err)
			w.Write(data)
		}))
		t.Cleanup(srv.Close)
		ctx, cancelFunc := testutil.Context(t)
		defer cancelFunc()
		repos, more, err := gitauth.Repositories(ctx, &gitauth.Config{
			Type:    codersdk.GitProviderGitHub,
			BaseURL: srv.URL,
		}, gitauth.RepositoriesOptions{
			HTTPClient: http.DefaultClient,
			Query:      "testing",
		})
		require.NoError(t, err)
		require.False(t, more)
		require.Len(t, repos, 1)
		require.Equal(t, gitauthRepo, repos[0])
	})
}
