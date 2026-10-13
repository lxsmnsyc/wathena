-- Player search, item history, restores and audit reviews (phase 6
-- step 2, proposal 0002 review decision 34, decision 0029).
--
-- Rules:
--   - A review of an audit row says ok or flag. A flag needs a note,
--     which says what is wrong.
--   - Name searches find a prefix of an account or a character name in
--     any case. The text_pattern_ops indexes serve LIKE 'prefix%' under
--     any collation.
--   - account_logins keeps one row per login of the account pages, with
--     its IP address, for 90 days (strategy section 7.10). Sessions go
--     sooner, when they expire or go idle, so they cannot serve as the
--     login records. The account service writes the row with the
--     session, and its cleanup deletes rows older than 90 days.
--   - IP searches find the logins of the last 90 days in a CIDR range.
--     The GiST index serves the <<= operator.
--   - Restores write their item log lines with the kind staff for the
--     copies and the zeny they make. Kinds are text, so no change is
--     needed for that.
--   - The account role adds, reads and deletes login records. The game
--     role gets no rights on them. Otherwise the rights stay: the account
--     role reads accounts and characters and adds reviews, and the game
--     role reads the game tables and the logs and adds audit rows.

-- +goose Up

ALTER TABLE staff_audit_reviews
    ADD COLUMN verdict text NOT NULL DEFAULT 'ok' CHECK (verdict IN ('ok', 'flag')),
    ADD CONSTRAINT staff_audit_reviews_flag_note CHECK (verdict <> 'flag' OR char_length(note) > 0);

CREATE INDEX accounts_name_prefix ON accounts (lower(name) text_pattern_ops);
CREATE INDEX characters_name_prefix ON characters (lower(name) text_pattern_ops);

CREATE TABLE account_logins (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    at timestamptz NOT NULL DEFAULT now(),
    ip inet,
    user_agent text NOT NULL DEFAULT '' CHECK (char_length(user_agent) <= 256)
);

CREATE INDEX account_logins_account ON account_logins (account_id, at);
CREATE INDEX account_logins_at ON account_logins (at);
CREATE INDEX account_logins_ip ON account_logins USING gist (ip inet_ops) WHERE ip IS NOT NULL;

-- The sessions that exist now become the first login records.
INSERT INTO account_logins (account_id, at, ip, user_agent)
SELECT account_id, created_at, ip, user_agent FROM account_sessions
WHERE created_at > now() - interval '90 days';

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT SELECT, INSERT, DELETE ON account_logins TO wathena_account;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE account_logins;
DROP INDEX characters_name_prefix;
DROP INDEX accounts_name_prefix;
ALTER TABLE staff_audit_reviews DROP CONSTRAINT staff_audit_reviews_flag_note, DROP COLUMN verdict;
