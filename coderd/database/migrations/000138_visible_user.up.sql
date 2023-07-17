BEGIN;

CREATE VIEW
    visible_users
AS
	SELECT
		id, username, avatar_url
	FROM
		users;

COMMENT ON VIEW visible_users IS 'Visible fields of users are allowed to be joined with other tables for including context of other resources.';

COMMIT;
