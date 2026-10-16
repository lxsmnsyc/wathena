-- The public branch of the seed tree (phase 7 step 2, decision 0039).
--
-- Rules:
--   - public_node is the key of child(root, "public") of the world's
--     seed (docs/specs/generator-v2.md, "Seed tree"). `wactl world new`
--     derives it, so the game role never needs the seed itself.
--   - The game role reads public_node. It derives each field's public
--     node from it, which it sends to clients. No secret branch can be
--     derived from it.
--   - Worlds made before this migration have none. waserver refuses a
--     procedural world without it.

-- +goose Up

ALTER TABLE world
    ADD COLUMN public_node bytea CHECK (public_node IS NULL OR octet_length(public_node) = 32);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'wathena_game') THEN
        GRANT SELECT (public_node) ON world TO wathena_game;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

ALTER TABLE world DROP COLUMN public_node;
