-- Moderation and GM commands (phase 6 step 3, proposal 0002 review
-- decision 35, decision 0030).
--
-- Rules:
--   - sanctions keeps kicks and bans too. A mute, a jail and a kick act
--     on a character. A ban acts on an account and has no character.
--     Every row names its account, so a list per account finds them all.
--   - A kick ends when it starts. A permanent ban ends at 'infinity'.
--   - Staff may lift a mute, a jail or a ban early, with a reason. The
--     row then keeps who lifted it and why, with lifted_at.
--   - A closed report has an outcome: no_action, warned or sanctioned,
--     an optional note, and who closed it. A sanctioned report may link
--     its sanction. A report's lines follow the normal chat log limit
--     once it is closed, as before.
--   - The account role reads the bans, so the account service refuses
--     logins and game tickets of a banned account. It reads only the
--     columns it needs. The game role keeps its rights on both tables.

-- +goose Up

ALTER TABLE sanctions
    ALTER COLUMN character_id DROP NOT NULL,
    ADD COLUMN account_id bigint REFERENCES accounts (id) ON DELETE CASCADE,
    ADD COLUMN lifted_by text NOT NULL DEFAULT '' CHECK (char_length(lifted_by) <= 64),
    ADD COLUMN lift_reason text NOT NULL DEFAULT '' CHECK (char_length(lift_reason) <= 255),
    DROP CONSTRAINT sanctions_kind_check,
    DROP CONSTRAINT sanctions_check;

UPDATE sanctions s SET account_id = c.account_id FROM characters c WHERE c.id = s.character_id;

ALTER TABLE sanctions
    ALTER COLUMN account_id SET NOT NULL,
    ADD CONSTRAINT sanctions_kind_check CHECK (kind IN ('mute', 'jail', 'kick', 'ban')),
    ADD CONSTRAINT sanctions_target CHECK ((kind = 'ban') = (character_id IS NULL)),
    ADD CONSTRAINT sanctions_ends CHECK (ends_at > started_at OR kind = 'kick' AND ends_at = started_at),
    ADD CONSTRAINT sanctions_lift CHECK (lifted_at IS NOT NULL OR lifted_by = '' AND lift_reason = '');

CREATE INDEX sanctions_account ON sanctions (account_id, id);
CREATE INDEX sanctions_bans ON sanctions (account_id, ends_at) WHERE kind = 'ban' AND lifted_at IS NULL;

ALTER TABLE reports
    ADD COLUMN outcome text CHECK (outcome IN ('no_action', 'warned', 'sanctioned')),
    ADD COLUMN note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 255),
    ADD COLUMN closed_by text NOT NULL DEFAULT '' CHECK (char_length(closed_by) <= 24),
    ADD COLUMN sanction_id bigint REFERENCES sanctions (id) ON DELETE SET NULL,
    -- Reports closed before this step have no outcome.
    ADD CONSTRAINT reports_outcome CHECK (outcome IS NULL OR status = 'closed'),
    ADD CONSTRAINT reports_sanction CHECK (sanction_id IS NULL OR outcome = 'sanctioned');

CREATE INDEX reports_status ON reports (status, id);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT SELECT (id, account_id, kind, reason, ends_at, lifted_at) ON sanctions TO wathena_account;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        REVOKE SELECT (id, account_id, kind, reason, ends_at, lifted_at) ON sanctions FROM wathena_account;
    END IF;
END
$$;
-- +goose StatementEnd

DROP INDEX reports_status;
ALTER TABLE reports
    DROP CONSTRAINT reports_sanction,
    DROP CONSTRAINT reports_outcome,
    DROP COLUMN sanction_id,
    DROP COLUMN closed_by,
    DROP COLUMN note,
    DROP COLUMN outcome;

DELETE FROM sanctions WHERE kind IN ('kick', 'ban');
DROP INDEX sanctions_bans;
DROP INDEX sanctions_account;
ALTER TABLE sanctions
    DROP CONSTRAINT sanctions_lift,
    DROP CONSTRAINT sanctions_ends,
    DROP CONSTRAINT sanctions_target,
    DROP CONSTRAINT sanctions_kind_check,
    DROP COLUMN lift_reason,
    DROP COLUMN lifted_by,
    DROP COLUMN account_id,
    ALTER COLUMN character_id SET NOT NULL,
    ADD CONSTRAINT sanctions_kind_check CHECK (kind IN ('mute', 'jail')),
    ADD CONSTRAINT sanctions_check CHECK (ends_at > started_at);
