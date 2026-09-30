-- Queries for the account service (server/account) and for redeeming
-- game tickets (server/store). The account service runs them as the
-- wathena_account role. Only RedeemTicket runs as wathena_game.

-- name: CreateAccountWithPassword :one
-- CreateAccountWithPassword makes an account and its password in one
-- statement. It returns no row when the name is taken.
WITH a AS (
    INSERT INTO accounts (name) VALUES (@name)
    ON CONFLICT DO NOTHING
    RETURNING id
)
INSERT INTO account_passwords (account_id, hash)
SELECT id, @hash FROM a
RETURNING account_id;

-- name: PasswordByName :one
-- PasswordByName finds an account and its password hash by name, in any
-- case. Dev accounts have no password row, so they cannot log in here.
SELECT a.id, a.name, p.hash
FROM accounts a
JOIN account_passwords p ON p.account_id = a.id
WHERE lower(a.name) = lower(@name);

-- name: UpdatePasswordHash :exec
-- UpdatePasswordHash stores a new hash of the same password, made with
-- newer parameters. It only replaces the hash that was checked.
UPDATE account_passwords SET hash = @new_hash
WHERE account_id = @account_id AND hash = @old_hash;

-- name: PasswordByAccount :one
-- PasswordByAccount returns an account's password hash, for pages that
-- ask for the password again.
SELECT hash FROM account_passwords WHERE account_id = @account_id;

-- name: CreateSession :one
-- CreateSession makes a login session. factors names what the player
-- proved to open it, such as {password,totp} or {passkey}.
INSERT INTO account_sessions (token_hash, account_id, expires_at, user_agent, ip, factors)
VALUES (@token_hash, @account_id, @expires_at, @user_agent, @ip, @factors::text[])
RETURNING id;

-- name: UseSession :one
-- UseSession finds a live session by its token hash and marks it used.
-- A session is live until it expires, is revoked, or goes unused for
-- idle.
UPDATE account_sessions s SET last_used_at = now()
FROM accounts a
WHERE s.token_hash = @token_hash AND a.id = s.account_id
    AND s.revoked_at IS NULL AND s.expires_at > now()
    AND s.last_used_at > now() - @idle::interval
RETURNING s.id, s.account_id, a.name AS account_name, s.character_id,
    s.authenticated_at, s.factors;

-- name: Reauthenticate :exec
-- Reauthenticate records that the player of a session proved who they
-- are again, just now.
UPDATE account_sessions SET authenticated_at = now()
WHERE id = @session_id AND revoked_at IS NULL;

-- name: RevokeSession :many
-- RevokeSession ends a session and returns its ID, so the game
-- connections it opened can be closed. It returns no row when the
-- session was already revoked.
UPDATE account_sessions SET revoked_at = now()
WHERE token_hash = @token_hash AND revoked_at IS NULL
RETURNING id;

-- name: DeleteSessionTickets :exec
-- DeleteSessionTickets removes tickets a session made and nobody used.
DELETE FROM game_tickets
WHERE session_id = (SELECT s.id FROM account_sessions s WHERE s.token_hash = @token_hash);

-- name: SelectCharacter :execrows
-- SelectCharacter records the character a session plays next, by its
-- slot on the session's account.
UPDATE account_sessions s SET character_id = c.id
FROM characters c
WHERE s.id = @session_id AND c.account_id = s.account_id AND c.slot = @slot;

-- name: CreateTicket :exec
INSERT INTO game_tickets (token_hash, session_id, account_id, character_id, expires_at)
VALUES (@token_hash, @session_id, @account_id, @character_id, @expires_at);

-- name: RedeemTicket :one
-- RedeemTicket uses a ticket up and returns its character. The DELETE
-- makes it work once, even when two connections send it at the same
-- moment. An expired ticket is deleted too, and returns live = false.
-- No row comes back when the character left the ticket's account.
WITH t AS (
    DELETE FROM game_tickets WHERE token_hash = @token_hash
    RETURNING session_id, account_id, character_id, expires_at
)
SELECT t.session_id, t.account_id, t.character_id, t.expires_at > now() AS live,
    c.name, c.job, c.sex, c.hair, c.hair_color
FROM t
JOIN characters c ON c.id = t.character_id AND c.account_id = t.account_id;

-- name: DeleteExpired :one
-- DeleteExpired removes expired tickets, pending logins and passkey
-- ceremonies, and sessions that expired or were revoked. It returns how
-- many sessions went.
WITH t AS (
    DELETE FROM game_tickets WHERE expires_at <= now()
), p AS (
    DELETE FROM account_pending_logins WHERE expires_at <= now()
), w AS (
    DELETE FROM account_webauthn_ceremonies WHERE expires_at <= now()
), s AS (
    DELETE FROM account_sessions
    WHERE expires_at <= now() OR revoked_at IS NOT NULL
        OR last_used_at <= now() - @idle::interval
    RETURNING 1
)
SELECT count(*) FROM s;

-- name: AccountCharacters :many
SELECT id, slot, name, job, sex, hair, hair_color, base_level, job_level, map
FROM characters
WHERE account_id = @account_id
ORDER BY slot;
