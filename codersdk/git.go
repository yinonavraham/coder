package codersdk

// ResolvedGitRepo represents a Git repository that has been resolved
// to a set of templates. This could have occurred from a string match
// or a simple search.
type ResolvedGitRepo struct {
	// Name is the repository name.
	// e.g. code-server
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
	// ExternalURL is a link to a repo..
	// e.g. https://github.com/coder/coder
	ExternalURL string `json:"external_url"`
	// CloneURL is the URL that will be passed in as a parameter.
	CloneURL string `json:"clone_url"`
	// Stars is the number of stargazers a repository has.
	// If stars is not a thing in the provider, this
	// field will be nil.
	Stars *int `json:"stars"`
	// Templates that accept the Git repository provided
	// as a parameter name.
	Templates []ResolvedGitRepoTemplate `json:"templates"`
}

// ResolvedGitRepoTemplate contains the template that the repository
// resolves to, along with the version, and the parameter name.
type ResolvedGitRepoTemplate struct {
	Template
	ParameterName string `json:"parameter_name"`
}
