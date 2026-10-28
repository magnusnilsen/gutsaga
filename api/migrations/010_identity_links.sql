-- External identities (Smia users) linked to a Battle.net player.
-- One subject maps to one player. The service secret is not stored here.

CREATE TABLE IF NOT EXISTS identity_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider text NOT NULL,
    subject text NOT NULL,
    email text NOT NULL DEFAULT '',
    player_id uuid NOT NULL REFERENCES players (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT identity_links_provider_subject_key UNIQUE (provider, subject),
    CONSTRAINT identity_links_provider_subject_chk CHECK (provider <> '' AND subject <> '')
);

CREATE INDEX IF NOT EXISTS identity_links_player_idx ON identity_links (player_id);
