-- Queries for parties and friend lists (decision 0020).

-- name: Parties :many
SELECT id, name, exp_even, item_even, pickup_shared, leader_id
FROM parties
ORDER BY id;

-- name: PartyMembers :many
-- PartyMembers are every party's members, with what the party window
-- shows of an offline member.
SELECT m.party_id, m.slot, c.id, c.account_id, c.name, c.base_level, c.job, c.map
FROM party_members m
JOIN characters c ON c.id = m.character_id
ORDER BY m.party_id, m.slot;

-- name: InsertParty :exec
INSERT INTO parties (id, name, exp_even, item_even, pickup_shared, leader_id)
VALUES (@id, @name, @exp_even, @item_even, @pickup_shared, @leader_id);

-- name: UpdateParty :exec
UPDATE parties
SET name = @name, exp_even = @exp_even, item_even = @item_even, pickup_shared = @pickup_shared, leader_id = @leader_id
WHERE id = @id;

-- name: DeleteParty :exec
DELETE FROM parties WHERE id = @id;

-- name: LeaveParties :exec
-- LeaveParties takes a character out of any party, before it joins one.
DELETE FROM party_members WHERE character_id = @character_id;

-- name: InsertPartyMember :exec
INSERT INTO party_members (party_id, character_id, slot)
VALUES (@party_id, @character_id, @slot)
ON CONFLICT (party_id, slot) DO UPDATE SET character_id = EXCLUDED.character_id;

-- name: DeletePartyMember :exec
DELETE FROM party_members WHERE party_id = @party_id AND character_id = @character_id;

-- name: CharacterFriends :many
SELECT c.id, c.name
FROM character_friends f
JOIN characters c ON c.id = f.friend_id
WHERE f.character_id = @character_id
ORDER BY f.added;

-- name: InsertFriend :exec
INSERT INTO character_friends (character_id, friend_id)
VALUES (@character_id, @friend_id)
ON CONFLICT DO NOTHING;

-- name: DeleteFriend :exec
DELETE FROM character_friends WHERE character_id = @character_id AND friend_id = @friend_id;
