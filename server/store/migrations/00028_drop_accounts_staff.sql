-- Drops accounts.staff (proposal 0002 review decision 41, decision
-- 0036). The column gave no role. Roles come from the config's
-- staff.owners and from staff_roles (decision 0028). `wactl world new`
-- prints the staff.owners line for its account instead.

-- +goose Up

ALTER TABLE accounts DROP COLUMN staff;

-- +goose Down

ALTER TABLE accounts ADD COLUMN staff boolean NOT NULL DEFAULT false;
