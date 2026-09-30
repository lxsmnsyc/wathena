-- The account service's tables: passwords, login sessions, and game
-- tickets (strategy section 7.3, decision 0004).
--
-- Rules:
--   - Tokens are never stored. Sessions and tickets keep the SHA-256 of
--     a random 256-bit token.
--   - Password hashes live in their own table, which only the account
--     role can read. The game role cannot read them.
--   - Two-factor login adds tables of its own later, next to
--     account_passwords, so a password stays one credential of several.

-- +goose Up

-- Account names from the signup page: 4 to 24 characters, ASCII letters,
-- digits and underscores, starting with a letter. Names stay unique
-- without regard to case (accounts_name_key). Dev accounts, which
-- waserver --dev makes from character names, keep the looser rule of
-- the first migration.
ALTER TABLE accounts ADD CONSTRAINT accounts_name_rules
    CHECK (dev OR name ~ '^[A-Za-z][A-Za-z0-9_]{3,23}$');

-- A password credential. hash is a PHC string, such as
-- $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>. It names its own
-- parameters, so they can change later: a login with older parameters
-- hashes the password again.
CREATE TABLE account_passwords (
    account_id bigint PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    hash text NOT NULL CHECK (char_length(hash) BETWEEN 1 AND 512),
    changed_at timestamptz NOT NULL DEFAULT now()
);

-- A login session on the account service's pages, kept in a cookie.
CREATE TABLE account_sessions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- SHA-256 of the cookie's token.
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now(),
    -- The session ends at expires_at even when it is in use.
    expires_at timestamptz NOT NULL,
    -- Set at logout. A revoked session is kept until cleanup.
    revoked_at timestamptz,
    -- Shown to the player on a later "your sessions" page. The IP address
    -- is deleted with the session, well within the 90 days of strategy
    -- section 7.10.
    user_agent text NOT NULL DEFAULT '' CHECK (char_length(user_agent) <= 256),
    ip inet,
    -- The character the player chose with Play. Game tickets go to it.
    character_id bigint REFERENCES characters (id) ON DELETE SET NULL,
    CHECK (expires_at > created_at)
);

CREATE INDEX account_sessions_account_id ON account_sessions (account_id);
CREATE INDEX account_sessions_expires_at ON account_sessions (expires_at);

-- A one-time ticket that opens one game connection for one character.
-- The web client gets it from the account service and sends it in Hello.
-- The gateway deletes it when it is used, so it works once.
CREATE TABLE game_tickets (
    -- SHA-256 of the ticket.
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    -- Logout deletes the session's tickets with the session's revoke.
    session_id bigint NOT NULL REFERENCES account_sessions (id) ON DELETE CASCADE,
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    character_id bigint NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CHECK (expires_at > created_at)
);

CREATE INDEX game_tickets_session_id ON game_tickets (session_id);
CREATE INDEX game_tickets_expires_at ON game_tickets (expires_at);

-- Rights, when the roles exist (deploy/postgres/roles.sql):
--   - wathena_account reads and writes passwords, sessions and tickets,
--     makes accounts and characters, and gives new characters their
--     start items. It cannot change or delete characters.
--   - wathena_game only reads a ticket and deletes it when it is used.
--     It gets no rights on passwords or sessions.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT SELECT, INSERT ON accounts TO wathena_account;
        GRANT SELECT, INSERT, UPDATE ON account_passwords TO wathena_account;
        GRANT SELECT, INSERT, UPDATE, DELETE ON account_sessions TO wathena_account;
        GRANT SELECT, INSERT, DELETE ON game_tickets TO wathena_account;
        GRANT SELECT, INSERT ON characters TO wathena_account;
        GRANT INSERT ON inventory TO wathena_account;
        GRANT SELECT ON goose_db_version TO wathena_account;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT, DELETE ON game_tickets TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE game_tickets;
DROP TABLE account_sessions;
DROP TABLE account_passwords;
ALTER TABLE accounts DROP CONSTRAINT accounts_name_rules;
