-- Two-factor login and passkeys (strategy section 7.3, decision 0005).
--
-- Rules:
--   - Only wathena_account can read or write these tables. The game role
--     gets no rights on them.
--   - TOTP secrets are stored encrypted with AES-256-GCM, under a key
--     from the account service's environment. A copy of the database
--     alone does not give the secrets.
--   - Recovery codes, pending logins and passkey ceremonies keep only
--     the SHA-256 of random values, as sessions do.

-- +goose Up

-- How a login session was opened, and when the player last proved who
-- they are. Pages that change the login methods ask for a fresh proof
-- when authenticated_at is old.
ALTER TABLE account_sessions
    ADD COLUMN authenticated_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN factors text[] NOT NULL DEFAULT '{password}'
        CHECK (cardinality(factors) BETWEEN 1 AND 4
            AND factors <@ ARRAY['password', 'totp', 'recovery_code', 'passkey']);

-- A TOTP credential (RFC 6238: SHA-1, 6 digits, 30-second steps).
CREATE TABLE account_totp (
    account_id bigint PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    -- The secret, sealed with AES-256-GCM: a version byte, an 8-byte key
    -- ID, a 12-byte nonce, then the ciphertext and tag. The account ID
    -- is bound in as additional data, so a sealed secret cannot be moved
    -- to another account.
    secret bytea NOT NULL CHECK (octet_length(secret) BETWEEN 37 AND 256),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- NULL while the player has not confirmed a first code. Login only
    -- asks for a code once this is set.
    enabled_at timestamptz,
    -- The last time step whose code was accepted. A code is only
    -- accepted for a later step, so a code cannot be used twice.
    last_step bigint NOT NULL DEFAULT 0 CHECK (last_step >= 0)
);

-- Single-use recovery codes, made when TOTP is turned on.
CREATE TABLE account_recovery_codes (
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    -- SHA-256 of an 80-bit random code.
    code_hash bytea NOT NULL CHECK (octet_length(code_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at timestamptz,
    PRIMARY KEY (account_id, code_hash)
);

-- A login whose password (or passkey without user verification) passed
-- and which waits for a TOTP code. It is not a session: it only opens
-- the code form, for a few minutes and a few attempts.
CREATE TABLE account_pending_logins (
    -- SHA-256 of the token in the browser's pending-login cookie.
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    -- The factor that already passed.
    factor text NOT NULL CHECK (factor IN ('password', 'passkey')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    CHECK (expires_at > created_at)
);

CREATE INDEX account_pending_logins_expires_at ON account_pending_logins (expires_at);

-- The WebAuthn user handle of an account: a random value that the
-- authenticator stores with a passkey and returns at login. It is not
-- the account ID, so a passkey does not reveal it.
CREATE TABLE account_webauthn_users (
    account_id bigint PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    user_handle bytea NOT NULL UNIQUE CHECK (octet_length(user_handle) = 32),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- A passkey (WebAuthn credential).
CREATE TABLE account_passkeys (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES account_webauthn_users (account_id) ON DELETE CASCADE,
    -- The relying party ID it was made for: the host of the account
    -- service's public URL. Passkeys of another RP ID are not offered.
    rp_id text NOT NULL CHECK (char_length(rp_id) BETWEEN 1 AND 253),
    credential_id bytea NOT NULL UNIQUE CHECK (octet_length(credential_id) BETWEEN 16 AND 1023),
    -- The COSE public key. It is not a secret.
    public_key bytea NOT NULL CHECK (octet_length(public_key) BETWEEN 1 AND 4096),
    attestation_type text NOT NULL DEFAULT '' CHECK (char_length(attestation_type) <= 32),
    attestation_format text NOT NULL DEFAULT '' CHECK (char_length(attestation_format) <= 32),
    aaguid bytea CHECK (octet_length(aaguid) = 16),
    transports text[] NOT NULL DEFAULT '{}' CHECK (cardinality(transports) <= 8),
    attachment text NOT NULL DEFAULT '' CHECK (char_length(attachment) <= 32),
    -- The authenticator data flags byte (UP, UV, BE, BS).
    flags smallint NOT NULL CHECK (flags BETWEEN 0 AND 255),
    -- The signature counter. A login whose counter does not go up, when
    -- either value is not 0, sets clone_warning and is refused.
    sign_count bigint NOT NULL DEFAULT 0 CHECK (sign_count BETWEEN 0 AND 4294967295),
    clone_warning boolean NOT NULL DEFAULT false,
    -- The player's name for it, such as "Phone".
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

CREATE INDEX account_passkeys_account_id ON account_passkeys (account_id);

-- The state kept between the two steps of a WebAuthn ceremony: the
-- challenge and its options. The browser holds a cookie with a token,
-- and a ceremony can finish only once, from that browser.
CREATE TABLE account_webauthn_ceremonies (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    kind text NOT NULL CHECK (kind IN ('register', 'login', 'reauth')),
    -- Set for register and reauth: the session that started it.
    session_id bigint REFERENCES account_sessions (id) ON DELETE CASCADE,
    -- The name of the passkey being registered.
    name text NOT NULL DEFAULT '' CHECK (char_length(name) <= 64),
    -- go-webauthn's SessionData as JSON.
    data jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CHECK (expires_at > created_at),
    CHECK ((kind = 'login') = (session_id IS NULL))
);

CREATE INDEX account_webauthn_ceremonies_expires_at ON account_webauthn_ceremonies (expires_at);

-- Rights, when the roles exist (deploy/postgres/roles.sql). Only the
-- account role gets any. The game role has none on these tables, so it
-- cannot read TOTP secrets, recovery codes or passkeys.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON account_totp TO wathena_account;
        GRANT SELECT, INSERT, UPDATE, DELETE ON account_recovery_codes TO wathena_account;
        GRANT SELECT, INSERT, UPDATE, DELETE ON account_pending_logins TO wathena_account;
        GRANT SELECT, INSERT ON account_webauthn_users TO wathena_account;
        GRANT SELECT, INSERT, UPDATE, DELETE ON account_passkeys TO wathena_account;
        GRANT SELECT, INSERT, DELETE ON account_webauthn_ceremonies TO wathena_account;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE account_webauthn_ceremonies;
DROP TABLE account_passkeys;
DROP TABLE account_webauthn_users;
DROP TABLE account_pending_logins;
DROP TABLE account_recovery_codes;
DROP TABLE account_totp;
ALTER TABLE account_sessions DROP COLUMN factors, DROP COLUMN authenticated_at;
