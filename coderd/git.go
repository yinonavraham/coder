package coderd

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func (api *API) gitRepos(rw http.ResponseWriter, r *http.Request) {
	match := r.URL.Query().Get("match")
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	prepared, err := api.HTTPAuth.AuthorizeSQLFilter(r, rbac.ActionRead, rbac.ResourceTemplate.Type)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error preparing sql filter.",
			Detail:  err.Error(),
		})
		return
	}
	templates, err := api.Database.GetAuthorizedTemplates(ctx, database.GetTemplatesWithFilterParams{}, prepared)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching templates in organization.",
			Detail:  err.Error(),
		})
		return
	}
	templateByVersionID := map[uuid.UUID]database.Template{}
	versionIDs := make([]uuid.UUID, 0)
	for _, template := range templates {
		templateByVersionID[template.ActiveVersionID] = template
		versionIDs = append(versionIDs, template.ActiveVersionID)
	}
	params, err := api.Database.GetTemplateVersionParameters(ctx, versionIDs)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template versions.",
			Detail:  err.Error(),
		})
		return
	}

	links, err := api.Database.GetGitAuthLinks(ctx, apiKey.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching git auth links.",
			Detail:  err.Error(),
		})
		return
	}
	linkByID := map[string]database.GitAuthLink{}
	for _, link := range links {
		linkByID[link.ProviderID] = link
	}

	resolvedRepos := make([]codersdk.ResolvedGitRepo, 0)
	for _, param := range params {
		for _, provider := range param.GitProviders {
			var found *gitauth.Config
			for _, config := range api.GitAuthConfigs {
				if config.ID == provider {
					found = config
					break
				}
			}
			if found == nil {
				continue
			}
			link, ok := linkByID[found.ID]
			if !ok {
				continue
			}
			oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken:  link.OAuthAccessToken,
				RefreshToken: link.OAuthRefreshToken,
				Expiry:       link.OAuthExpiry,
			}))
			repos, _, err := gitauth.Repositories(ctx, found, gitauth.RepositoriesOptions{
				HTTPClient: oauthClient,
				Query:      match,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to fetch Git repositories.",
					Detail:  err.Error(),
				})
				return
			}
			template, ok := templateByVersionID[param.TemplateVersionID]
			if !ok {
				continue
			}
			// regex, err := regexp.Compile(param.ValidationRegex)
			// if err != nil {
			// 	continue
			// }
			for _, repo := range repos {
				// if !regex.MatchString(repo.ExternalURL) {
				// 	continue
				// }

				resolvedRepos = append(resolvedRepos, codersdk.ResolvedGitRepo{
					Name:        repo.Name,
					Description: repo.Description,
					Owner:       repo.Owner,
					ExternalURL: repo.ExternalURL,
					Stars:       repo.Stars,
					Templates: []codersdk.ResolvedGitRepoTemplate{{
						Template:      api.convertTemplate(template, 0, ""),
						ParameterName: param.Name,
					}},
				})
			}
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, resolvedRepos)
}
