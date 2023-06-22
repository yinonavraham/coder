BEGIN;
ALTER TABLE workspaces ADD COLUMN locked_at timestamptz NULL;
ALTER TYPE build_reason ADD VALUE 'autolock';
ALTER TYPE build_reason ADD VALUE 'failedstop';
ALTER TYPE build_reason ADD VALUE 'autodelete';
COMMIT;
