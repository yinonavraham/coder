package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

func (api *API) gitRepos(rw http.ResponseWriter, r *http.Request) {
	// match := r.URL.Query().Get("match")
	apiKey := httpmw.APIKey(r)
	ctx := r.Context()

	links, err := api.Database.GetGitAuthLinks(r.Context(), apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to fetch Git auth links.",
			Detail:  err.Error(),
		})
		return
	}

	linkByID := map[string]database.GitAuthLink{}
	for _, link := range links {
		linkByID[link.ProviderID] = link
	}

	for _, config := range api.GitAuthConfigs {
		link, ok := linkByID[config.ID]
		if !ok {
			// They need to authenticate with the provider!
		}

		link, updated, err := refreshGitToken(ctx, api.Database, apiKey.UserID, config, link)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to refresh git auth token.",
				Detail:  err.Error(),
			})
			return
		}
		if !updated {
			// They need to authenticate with the provider!
		}

		gitauth.Repositories(ctx, config, gitauth.RepositoriesOptions{})
	}
}
