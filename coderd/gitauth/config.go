package gitauth

import (
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// Config is used for authentication for Git operations.
type Config struct {
	httpmw.OAuth2Config
	// ID is a unique identifier for the authenticator.
	ID string
	// Regex is a regexp that URLs will match against.
	Regex *regexp.Regexp
	// Type is the type of provider.
	Type codersdk.GitProvider
	// NoRefresh stops Coder from using the refresh token
	// to renew the access token.
	//
	// Some organizations have security policies that require
	// re-authentication for every token.
	NoRefresh bool
	// ValidateURL ensures an access token is valid before
	// returning it to the user. If omitted, tokens will
	// not be validated before being returned.
	ValidateURL string
	// BaseURL is the root URL for all endpoints.
	BaseURL string
}

// ConvertConfig converts the YAML configuration entry to the
// parsed and ready-to-consume provider type.
func ConvertConfig(entries []codersdk.GitAuthConfig, accessURL *url.URL) ([]*Config, error) {
	ids := map[string]struct{}{}
	configs := []*Config{}
	for _, entry := range entries {
		var typ codersdk.GitProvider
		switch entry.Type {
		case codersdk.GitProviderAzureDevops:
			typ = codersdk.GitProviderAzureDevops
		case codersdk.GitProviderBitBucket:
			typ = codersdk.GitProviderBitBucket
		case codersdk.GitProviderGitHub:
			typ = codersdk.GitProviderGitHub
		case codersdk.GitProviderGitLab:
			typ = codersdk.GitProviderGitLab
		default:
			return nil, xerrors.Errorf("unknown git provider type: %q", entry.Type)
		}
		if entry.ID == "" {
			// Default to the type.
			entry.ID = string(typ)
		}
		if valid := httpapi.NameValid(entry.ID); valid != nil {
			return nil, xerrors.Errorf("git auth provider %q doesn't have a valid id: %w", entry.ID, valid)
		}

		_, exists := ids[entry.ID]
		if exists {
			if entry.ID == string(typ) {
				return nil, xerrors.Errorf("multiple %s git auth providers provided. you must specify a unique id for each", typ)
			}
			return nil, xerrors.Errorf("multiple git providers exist with the id %q. specify a unique id for each", entry.ID)
		}
		ids[entry.ID] = struct{}{}

		if entry.ClientID == "" {
			return nil, xerrors.Errorf("%q git auth provider: client_id must be provided", entry.ID)
		}
		if entry.ClientSecret == "" {
			return nil, xerrors.Errorf("%q git auth provider: client_secret must be provided", entry.ID)
		}
		authRedirect, err := accessURL.Parse(fmt.Sprintf("/gitauth/%s/callback", entry.ID))
		if err != nil {
			return nil, xerrors.Errorf("parse gitauth callback url: %w", err)
		}
		regex := regex[typ]
		if entry.Regex != "" {
			regex, err = regexp.Compile(entry.Regex)
			if err != nil {
				return nil, xerrors.Errorf("compile regex for git auth provider %q: %w", entry.ID, entry.Regex)
			}
		}

		authURL := entry.AuthURL
		tokenURL := entry.TokenURL
		validateURL := entry.ValidateURL
		baseURL := entry.BaseURL

		if baseURL == "" {
			baseURL = defaultBaseURL[typ]
		}
		parsedBase, err := url.Parse(baseURL)
		if err != nil {
			return nil, xerrors.Errorf("parse base url: %w", err)
		}

		if authURL == "" {
			switch typ {
			case codersdk.GitProviderGitHub:
				authURL, err = parseURL(parsedBase, "/login/oauth/authorize")
			case codersdk.GitProviderGitLab:
				authURL, err = parseURL(parsedBase, "/oauth/authorize")
			case codersdk.GitProviderBitBucket:
				authURL, err = parseURL(parsedBase, "/site/oauth2/authorize")
			case codersdk.GitProviderAzureDevops:
				authURL, err = parseURL(parsedBase, "/oauth2/authorize")
			default:
				return nil, xerrors.Errorf("no base auth url for type %q", typ)
			}
			if err != nil {
				return nil, xerrors.Errorf("parse base auth url: %w", err)
			}
		}
		if tokenURL == "" {
			switch typ {
			case codersdk.GitProviderGitHub:
				tokenURL, err = parseURL(parsedBase, "/login/oauth/access_token")
			case codersdk.GitProviderGitLab:
				tokenURL, err = parseURL(parsedBase, "/oauth/token")
			case codersdk.GitProviderBitBucket:
				tokenURL, err = parseURL(parsedBase, "/site/oauth2/access_token")
			case codersdk.GitProviderAzureDevops:
				tokenURL, err = parseURL(parsedBase, "/oauth2/token")
			default:
				return nil, xerrors.Errorf("no base token url for type %q", typ)
			}
			if err != nil {
				return nil, xerrors.Errorf("parse base token url: %w", err)
			}
		}
		if validateURL == "" {
			switch typ {
			case codersdk.GitProviderGitHub:
				// If we're on hosted GitHub, use the subdomain!
				if baseURL == defaultBaseURL[typ] {
					validateURL = "https://api.github.com/user"
					break
				}
				validateURL, err = parseURL(parsedBase, "/api/v3/user")
			case codersdk.GitProviderGitLab:
				validateURL, err = parseURL(parsedBase, "/oauth/token/info")
			case codersdk.GitProviderBitBucket:
				if baseURL == defaultBaseURL[typ] {
					validateURL = "https://api.bitbucket.org/2.0/user"
					break
				}
				// Validation is not implemented for self-hosted BitBucket server.
			}
			if err != nil {
				return nil, xerrors.Errorf("parse validate url: %w", err)
			}
		}

		oauth2Config := &oauth2.Config{
			ClientID:     entry.ClientID,
			ClientSecret: entry.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  authURL,
				TokenURL: tokenURL,
			},
			RedirectURL: authRedirect.String(),
			Scopes:      scope[typ],
		}
		if entry.Scopes != nil && len(entry.Scopes) > 0 {
			oauth2Config.Scopes = entry.Scopes
		}

		var oauthConfig httpmw.OAuth2Config = oauth2Config
		// Azure DevOps uses JWT token authentication!
		if typ == codersdk.GitProviderAzureDevops {
			oauthConfig = newJWTOAuthConfig(oauth2Config)
		}

		configs = append(configs, &Config{
			OAuth2Config: oauthConfig,
			ID:           entry.ID,
			Regex:        regex,
			Type:         typ,
			NoRefresh:    entry.NoRefresh,
			ValidateURL:  validateURL,
			BaseURL:      baseURL,
		})
	}
	return configs, nil
}

// parseURL parses the path provided on the base but returns a string
// instead of a URL for reducing duplicate code above.
func parseURL(base *url.URL, path string) (string, error) {
	parsed, err := base.Parse(path)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

// defaultBaseURL contains defaults to use in API clients.
var defaultBaseURL = map[codersdk.GitProvider]string{
	codersdk.GitProviderBitBucket:   "https://bitbucket.org",
	codersdk.GitProviderGitHub:      "https://github.com",
	codersdk.GitProviderGitLab:      "https://gitlab.com",
	codersdk.GitProviderAzureDevops: "https://app.vssps.visualstudio.com",
}
