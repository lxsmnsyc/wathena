---@meta wa.parties
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Parties, as rAthena's party system (review decision 27). A party is one for the whole world. The write calls replace party_create, party_addmember, party_delmember, party_changeleader, party_changeoption and party_destroy, and skip the leader's rights and the invite, as those commands do.
---@class wa.parties
local parties = {}

---Makes a party with the player as its leader, as party_create, and returns it. Exp share starts as each, and the items as each. nil when the name is taken or the leader has a party. The party_join event follows, with the reason "script". A test case needs no permission for it.
---
---Needs the permission `party.manage`.
---@param leader wa.Player
---@param name string 1 to 24 characters.
---@return wa.Party?
function parties.create(leader, name) end

---A party by ID, or nil.
---
---Needs the permission `game.read`.
---@param id integer A party's ID, such as player.party.id.
---@return wa.Party?
function parties.get(id) end

---A party by name, or nil.
---
---Needs the permission `game.read`.
---@param name string A party's name, without regard to case.
---@return wa.Party?
function parties.find(name) end

return parties
