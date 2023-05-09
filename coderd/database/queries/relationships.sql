-- name: GetRelationships :many
SELECT * FROM find_relationships(@parent_id, @parent_type, @child_id, @child_type);

-- name: InsertRelationships :many
INSERT INTO relationships SELECT
	UNNEST(@id :: uuid [ ]) AS id,
	UNNEST(@parent :: uuid [ ]) AS parent,
	UNNEST(@parent_type :: relationship_member [ ]) AS parent_type,
	UNNEST(@child :: uuid [ ]) AS child,
	UNNEST(@child_type :: relationship_member [ ]) AS child_type,
	UNNEST(@permission :: relationship_permission [ ]) AS permission,
	UNNEST(@created_at :: timestamptz [ ]) AS created_at RETURNING *;
