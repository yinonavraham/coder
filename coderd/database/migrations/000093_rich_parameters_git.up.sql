ALTER TABLE template_version_parameters ADD COLUMN
	git_providers text[];

COMMENT ON COLUMN template_version_parameters.git_providers IS 'Git providers that must be authenticated for the parameter value to be consumable.';
