-- A session factor for a passkey that verified its user (proposal 0002,
-- review decision 54, docs/security/review-2026-10.md, "A passkey
-- without user verification counted as a second factor").
--
-- Rules:
--   - A passkey login whose assertion has the user verification (UV)
--     flag records the factors passkey and passkey_verified.
--   - A passkey login without the flag records passkey only.
--   - A staff role counts only for a session with totp, recovery_code or
--     passkey_verified. passkey alone no longer counts.
--   - Sessions made before this migration keep their factors. A staff
--     member whose session has passkey alone logs in again.

-- +goose Up

ALTER TABLE account_sessions DROP CONSTRAINT account_sessions_factors_check;
ALTER TABLE account_sessions ADD CONSTRAINT account_sessions_factors_check
    CHECK (cardinality(factors) BETWEEN 1 AND 4
        AND factors <@ ARRAY['password', 'totp', 'recovery_code', 'passkey', 'passkey_verified']);

-- +goose Down

DELETE FROM account_sessions WHERE 'passkey_verified' = ANY (factors);
ALTER TABLE account_sessions DROP CONSTRAINT account_sessions_factors_check;
ALTER TABLE account_sessions ADD CONSTRAINT account_sessions_factors_check
    CHECK (cardinality(factors) BETWEEN 1 AND 4
        AND factors <@ ARRAY['password', 'totp', 'recovery_code', 'passkey']);
