-- Staff roles and the GM audit log (phase 6 step 1, proposal 0002
-- review decision 33, decision 0028).
--
-- Rules:
--   - staff_roles keeps the roles that owners and admins give from the
--     admin screens, one per account. The owner role comes only from
--     the config, so it is never in this table.
--   - staff_audit is the GM audit log. Rows have no foreign keys and
--     keep the account's name, so they outlive the accounts they name.
--     They are kept 2 years. The deletion job comes with phase 8.
--   - The game and account roles can only add audit rows. Neither can
--     change or delete one. The account role reads them for the admin
--     API. The game role cannot read them.
--   - needs_review marks a row for review, such as an action that made
--     items or zeny. Such a row needs a reason. Reviews are kept in
--     staff_audit_reviews, so a review never changes the row.
--   - game_tickets.two_factor records that the ticket's login session
--     logged in with a second factor, while the account had TOTP or a
--     passkey on. Only such a game login counts a staff role.

-- +goose Up

CREATE TABLE staff_roles (
    account_id bigint PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role ~ '^[a-z][a-z0-9_]{0,31}$' AND role <> 'owner'),
    -- Who gave the role. NULL when that account is gone.
    given_by bigint REFERENCES accounts (id) ON DELETE SET NULL,
    given_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE staff_audit (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at timestamptz NOT NULL DEFAULT now(),
    -- The staff account that acted, and the role it acted with.
    account_id bigint NOT NULL CHECK (account_id > 0),
    account_name text NOT NULL CHECK (char_length(account_name) BETWEEN 1 AND 24),
    role text NOT NULL CHECK (role ~ '^[a-z][a-z0-9_]{0,31}$'),
    -- What it did, such as staff.give, and where: admin for the admin
    -- API, game for the game.
    action text NOT NULL CHECK (action ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$' AND char_length(action) <= 64),
    source text NOT NULL CHECK (source IN ('admin', 'game')),
    -- The account, character or thing it acted on, as text, and the
    -- account's ID when it is one.
    target text NOT NULL DEFAULT '' CHECK (char_length(target) <= 64),
    target_account_id bigint CHECK (target_account_id > 0),
    arguments jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(arguments) = 'object' AND octet_length(arguments::text) <= 4096),
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 255),
    needs_review boolean NOT NULL DEFAULT false,
    CHECK (NOT needs_review OR char_length(reason) > 0)
);

CREATE INDEX staff_audit_at ON staff_audit (at);
CREATE INDEX staff_audit_account ON staff_audit (lower(account_name), id);
CREATE INDEX staff_audit_target ON staff_audit (lower(target), id);
CREATE INDEX staff_audit_review ON staff_audit (id) WHERE needs_review;

CREATE TABLE staff_audit_reviews (
    audit_id bigint PRIMARY KEY REFERENCES staff_audit (id) ON DELETE CASCADE,
    reviewer_id bigint NOT NULL CHECK (reviewer_id > 0),
    reviewer_name text NOT NULL CHECK (char_length(reviewer_name) BETWEEN 1 AND 24),
    reviewed_at timestamptz NOT NULL DEFAULT now(),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 255)
);

ALTER TABLE game_tickets ADD COLUMN two_factor boolean NOT NULL DEFAULT false;

-- Rights, when the roles exist (deploy/postgres/roles.sql):
--   - wathena_account gives and removes roles, adds and reads audit
--     rows, and adds and reads reviews.
--   - wathena_game reads roles and adds audit rows. It cannot read
--     audit rows or reviews.
--   - Neither can update or delete an audit row or a review.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_account') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON staff_roles TO wathena_account;
        GRANT SELECT, INSERT ON staff_audit, staff_audit_reviews TO wathena_account;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT ON staff_roles TO wathena_game;
        GRANT INSERT ON staff_audit TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

ALTER TABLE game_tickets DROP COLUMN two_factor;
DROP TABLE staff_audit_reviews;
DROP TABLE staff_audit;
DROP TABLE staff_roles;
