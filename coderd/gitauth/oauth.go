package gitauth

import (
	"context"
	"net/url"
	"regexp"

	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// scope contains defaults for each Git provider.
var scope = map[codersdk.GitProvider][]string{
	codersdk.GitProviderAzureDevops: {"vso.code_write"},
	codersdk.GitProviderBitBucket:   {"account", "repository:write"},
	codersdk.GitProviderGitLab:      {"write_repository"},
	// "workflow" is required for managing GitHub Actions in a repository.
	codersdk.GitProviderGitHub: {"repo", "workflow"},
}

// regex provides defaults for each Git provider to match their SaaS host URL.
// This is configurable by each provider.
var regex = map[codersdk.GitProvider]*regexp.Regexp{
	codersdk.GitProviderAzureDevops: regexp.MustCompile(`^(https?://)?dev\.azure\.com(/.*)?$`),
	codersdk.GitProviderBitBucket:   regexp.MustCompile(`^(https?://)?bitbucket\.org(/.*)?$`),
	codersdk.GitProviderGitLab:      regexp.MustCompile(`^(https?://)?gitlab\.com(/.*)?$`),
	codersdk.GitProviderGitHub:      regexp.MustCompile(`^(https?://)?github\.com(/.*)?$`),
}

// newJWTOAuthConfig creates a new OAuth2 config that uses a custom
// assertion method that works with Azure Devops. See:
// https://learn.microsoft.com/en-us/azure/devops/integrate/get-started/authentication/oauth?view=azure-devops
func newJWTOAuthConfig(config *oauth2.Config) httpmw.OAuth2Config {
	return &jwtConfig{config}
}

type jwtConfig struct {
	*oauth2.Config
}

func (c *jwtConfig) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return c.Config.AuthCodeURL(state, append(opts, oauth2.SetAuthURLParam("response_type", "Assertion"))...)
}

func (c *jwtConfig) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	v := url.Values{
		"client_assertion_type": {},
		"client_assertion":      {c.ClientSecret},
		"assertion":             {code},
		"grant_type":            {},
	}
	if c.RedirectURL != "" {
		v.Set("redirect_uri", c.RedirectURL)
	}
	return c.Config.Exchange(ctx, code,
		append(opts,
			oauth2.SetAuthURLParam("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"),
			oauth2.SetAuthURLParam("client_assertion", c.ClientSecret),
			oauth2.SetAuthURLParam("assertion", code),
			oauth2.SetAuthURLParam("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer"),
			oauth2.SetAuthURLParam("code", ""),
		)...,
	)
}
