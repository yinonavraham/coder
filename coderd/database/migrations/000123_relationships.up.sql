CREATE TYPE relationship_member AS ENUM (
    'user',
    'organization',
    'team',
    'workspace',
	'template'
);

CREATE TYPE relationship_permission AS ENUM (
    'read',
    'write',
    'admin'
);

-- Relationships between objects.
CREATE TABLE relationships (
    id UUID NOT NULL UNIQUE,
    parent UUID NOT NULL,
    parent_type relationship_member NOT NULL,
    child UUID NOT NULL,
    child_type relationship_member NOT NULL,
    permission relationship_permission NOT NULL DEFAULT 'read',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    UNIQUE(parent, child)
);

CREATE INDEX parent_parent_type ON relationships (parent, parent_type);
CREATE INDEX child_child_type ON relationships (child, child_type);

CREATE FUNCTION find_relationships(
    target_parent_id UUID, target_parent_type relationship_member,
    target_child_id UUID DEFAULT NULL, target_child_type relationship_member DEFAULT NULL
)
RETURNS setof relationships AS $$
BEGIN
    -- Recursive CTE to traverse the relationship hierarchy
    RETURN QUERY
    WITH RECURSIVE relationship_hierarchy AS (
        -- Base case: Direct relationships where the source is the parent
        SELECT
            *
        FROM relationships r
        WHERE r.parent = target_parent_id
            AND r.parent_type = target_parent_type
            AND (r.child_type = target_child_type OR target_child_type IS NULL)

        UNION ALL

        -- Recursive step: Relationships where the parent is a child in the previous level
        SELECT
            r.id,
            r.parent,
            r.parent_type,
            r.child,
            r.child_type,
            r.permission,
            r.created_at
        FROM relationships r
        JOIN relationship_hierarchy rh ON r.parent = rh.child AND r.parent_type = rh.child_type
        WHERE (r.child_type = target_child_type OR target_child_type IS NULL)
    )
    SELECT * FROM relationship_hierarchy rh
    WHERE rh.child = COALESCE(target_child_id, rh.child)
    UNION
    SELECT
        r.id,
        r.parent,
        r.parent_type,
        r.child,
        r.child_type,
        r.permission,
        r.created_at
    FROM relationships r
    JOIN find_relationships(target_parent_id, target_parent_type, NULL, 'organization') org_admin
    ON org_admin.permission = 'admin' AND r.parent_type = 'user' AND r.child_type = 'workspace';
END;
$$ LANGUAGE plpgsql;
