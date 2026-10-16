CREATE TABLE collaborators (
    id UUID PRIMARY KEY,
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    collaborator_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX ix_collaborators_owner_user_id ON collaborators (owner_user_id);
CREATE INDEX ix_collaborators_collaborator_user_id ON collaborators (collaborator_user_id);
