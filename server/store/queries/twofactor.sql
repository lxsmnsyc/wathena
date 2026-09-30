-- Queries for two-factor login and passkeys (server/account). They run
-- as the wathena_account role, the only role with rights on these
-- tables.

-- name: TOTPByAccount :one
SELECT secret, enabled_at, last_step FROM account_totp WHERE account_id = @account_id;

-- name: StartTOTP :execrows
-- StartTOTP stores a new secret that waits for its first code. It does
-- not replace a secret that is already turned on, and returns 0 then.
INSERT INTO account_totp (account_id, secret) VALUES (@account_id, @secret)
ON CONFLICT (account_id) DO UPDATE
    SET secret = EXCLUDED.secret, created_at = now(), last_step = 0
    WHERE account_totp.enabled_at IS NULL;

-- name: EnableTOTP :execrows
-- EnableTOTP turns TOTP on after the first code, and only for the
-- secret that code was checked against.
UPDATE account_totp SET enabled_at = now(), last_step = @step
WHERE account_id = @account_id AND enabled_at IS NULL AND secret = @secret;

-- name: UseTOTPStep :execrows
-- UseTOTPStep accepts a code's time step once. It returns 0 when a code
-- of this step or a later one was already accepted, so two requests
-- with the same code cannot both pass.
UPDATE account_totp SET last_step = @step
WHERE account_id = @account_id AND enabled_at IS NOT NULL AND last_step < @step;

-- name: ResealTOTP :exec
-- ResealTOTP stores a secret sealed again under the current key. It only
-- replaces the sealed value that was read.
UPDATE account_totp SET secret = @new_secret
WHERE account_id = @account_id AND secret = @old_secret;

-- name: DeleteTOTP :exec
-- DeleteTOTP turns TOTP off and deletes the recovery codes with it.
WITH r AS (
    DELETE FROM account_recovery_codes WHERE account_id = @account_id
)
DELETE FROM account_totp WHERE account_totp.account_id = @account_id;

-- name: DeleteRecoveryCodes :exec
DELETE FROM account_recovery_codes WHERE account_id = @account_id;

-- name: InsertRecoveryCodes :exec
INSERT INTO account_recovery_codes (account_id, code_hash)
SELECT @account_id, h FROM unnest(@hashes::bytea[]) AS h;

-- name: UseRecoveryCode :execrows
-- UseRecoveryCode spends a recovery code. It returns 0 for a code that
-- is wrong or was used.
UPDATE account_recovery_codes SET used_at = now()
WHERE account_id = @account_id AND code_hash = @code_hash AND used_at IS NULL;

-- name: RecoveryCodesLeft :one
SELECT count(*) FROM account_recovery_codes
WHERE account_id = @account_id AND used_at IS NULL;

-- name: CreatePendingLogin :exec
INSERT INTO account_pending_logins (token_hash, account_id, factor, expires_at)
VALUES (@token_hash, @account_id, @factor, @expires_at);

-- name: AttemptPendingLogin :one
-- AttemptPendingLogin counts one code attempt on a live pending login
-- and returns it. It returns no row once the login expired or used its
-- attempts.
UPDATE account_pending_logins p SET attempts = p.attempts + 1
FROM accounts a
WHERE p.token_hash = @token_hash AND a.id = p.account_id
    AND p.expires_at > now() AND p.attempts < @max_attempts
RETURNING p.account_id, a.name AS account_name, p.factor;

-- name: PendingLogin :one
-- PendingLogin reads a live pending login without counting an attempt,
-- to show the code form.
SELECT p.account_id, p.attempts FROM account_pending_logins p
WHERE p.token_hash = @token_hash AND p.expires_at > now() AND p.attempts < @max_attempts;

-- name: DeletePendingLogin :exec
DELETE FROM account_pending_logins WHERE token_hash = @token_hash;

-- name: WebAuthnHandle :one
-- WebAuthnHandle returns an account's WebAuthn user handle, and stores
-- the given new one when the account has none yet.
WITH ins AS (
    INSERT INTO account_webauthn_users (account_id, user_handle)
    VALUES (@account_id, @new_handle)
    ON CONFLICT (account_id) DO NOTHING
    RETURNING user_handle
)
SELECT user_handle FROM ins
UNION ALL
SELECT u.user_handle FROM account_webauthn_users u WHERE u.account_id = @account_id
LIMIT 1;

-- name: WebAuthnHandleOf :one
SELECT user_handle FROM account_webauthn_users WHERE account_id = @account_id;

-- name: PasskeyOwner :one
-- PasskeyOwner finds the account of a passkey by its credential ID and
-- relying party ID, for a login without a name.
SELECT u.account_id, u.user_handle, a.name AS account_name
FROM account_passkeys p
JOIN account_webauthn_users u ON u.account_id = p.account_id
JOIN accounts a ON a.id = p.account_id
WHERE p.credential_id = @credential_id AND p.rp_id = @rp_id;

-- name: AccountPasskeys :many
SELECT id, rp_id, credential_id, public_key, attestation_type, attestation_format,
    aaguid, transports, attachment, flags, sign_count, clone_warning, name,
    created_at, last_used_at
FROM account_passkeys
WHERE account_id = @account_id
ORDER BY created_at, id;

-- name: CreatePasskey :execrows
-- CreatePasskey stores a new passkey while the account has fewer than
-- max_passkeys. It returns 0 when the account is full or the credential
-- ID is taken.
INSERT INTO account_passkeys (account_id, rp_id, credential_id, public_key,
    attestation_type, attestation_format, aaguid, transports, attachment,
    flags, sign_count, name)
SELECT @account_id, @rp_id, @credential_id, @public_key,
    @attestation_type, @attestation_format, @aaguid, @transports::text[], @attachment,
    @flags, @sign_count, @name
WHERE (SELECT count(*) FROM account_passkeys c WHERE c.account_id = @account_id) < @max_passkeys::bigint
ON CONFLICT (credential_id) DO NOTHING;

-- name: UsePasskey :exec
-- UsePasskey records a login with a passkey: its new counter, flags and
-- clone warning.
UPDATE account_passkeys
SET sign_count = @sign_count, flags = @flags, clone_warning = @clone_warning, last_used_at = now()
WHERE id = @id;

-- name: DeletePasskey :execrows
DELETE FROM account_passkeys WHERE id = @id AND account_id = @account_id;

-- name: CreateCeremony :exec
INSERT INTO account_webauthn_ceremonies (token_hash, kind, session_id, name, data, expires_at)
VALUES (@token_hash, @kind, @session_id, @name, @data, @expires_at);

-- name: TakeCeremony :one
-- TakeCeremony uses up a WebAuthn ceremony's state, so each challenge
-- can be answered once. An expired one is deleted too and returns
-- live = false.
DELETE FROM account_webauthn_ceremonies
WHERE token_hash = @token_hash
RETURNING kind, session_id, name, data, expires_at > now() AS live;
