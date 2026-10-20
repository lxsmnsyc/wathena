---@meta wa.variables
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Saved variables, declared at the top of a file with a scope and a type. They are stored under the mod's ID. A mod declares at most 256 variables, and declaring one again gives the same handle. Core has no such limit (review decision 58). A mod's values take at most 1 MiB as JSON on one player, and 1 MiB in its server variables. Its server variables, and apart its temporary server and NPC variables, hold at most 256 values, which core does not have either.
---@class wa.variables
local variables = {}

---Declares a integer variable. Saved with the character.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.IntegerVariable
function variables.character_integer(name) end

---Declares a boolean variable. Saved with the character.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.BooleanVariable
function variables.character_boolean(name) end

---Declares a string variable. Saved with the character.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.StringVariable
function variables.character_string(name) end

---Declares a table variable. Saved with the character.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.TableVariable
function variables.character_table(name) end

---Declares a integer variable. Kept until the character logs out.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.IntegerVariable
function variables.character_temporary_integer(name) end

---Declares a boolean variable. Kept until the character logs out.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.BooleanVariable
function variables.character_temporary_boolean(name) end

---Declares a string variable. Kept until the character logs out.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.StringVariable
function variables.character_temporary_string(name) end

---Declares a table variable. Kept until the character logs out.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.TableVariable
function variables.character_temporary_table(name) end

---Declares a integer variable. Shared by the account's characters.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.IntegerVariable
function variables.account_integer(name) end

---Declares a boolean variable. Shared by the account's characters.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.BooleanVariable
function variables.account_boolean(name) end

---Declares a string variable. Shared by the account's characters.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.StringVariable
function variables.account_string(name) end

---Declares a table variable. Shared by the account's characters.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.TableVariable
function variables.account_table(name) end

---Declares a integer variable. One value for the whole world, saved soon after a change.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.ServerIntegerVariable
function variables.server_integer(name) end

---Declares a boolean variable. One value for the whole world, saved soon after a change.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.ServerBooleanVariable
function variables.server_boolean(name) end

---Declares a string variable. One value for the whole world, saved soon after a change.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.ServerStringVariable
function variables.server_string(name) end

---Declares a table variable. One value for the whole world, saved soon after a change.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.ServerTableVariable
function variables.server_table(name) end

---Declares a integer variable. One value for the whole world, not saved, as rAthena's $@ variables (review decision 58).
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.ServerIntegerVariable
function variables.server_temporary_integer(name) end

---Declares a boolean variable. One value for the whole world, not saved, as rAthena's $@ variables (review decision 58).
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.ServerBooleanVariable
function variables.server_temporary_boolean(name) end

---Declares a string variable. One value for the whole world, not saved, as rAthena's $@ variables (review decision 58).
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.ServerStringVariable
function variables.server_temporary_string(name) end

---Declares a table variable. One value for the whole world, not saved, as rAthena's $@ variables (review decision 58).
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.ServerTableVariable
function variables.server_temporary_table(name) end

---Declares a integer variable. One value per NPC, which every zone reads, not saved, as rAthena's .x variables and getvariableofnpc (review decision 58).
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.NpcIntegerVariable
function variables.npc_integer(name) end

---Declares a boolean variable. One value per NPC, which every zone reads, not saved, as rAthena's .x variables and getvariableofnpc (review decision 58).
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.NpcBooleanVariable
function variables.npc_boolean(name) end

---Declares a string variable. One value per NPC, which every zone reads, not saved, as rAthena's .x variables and getvariableofnpc (review decision 58).
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.NpcStringVariable
function variables.npc_string(name) end

---Declares a table variable. One value per NPC, which every zone reads, not saved, as rAthena's .x variables and getvariableofnpc (review decision 58).
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.NpcTableVariable
function variables.npc_table(name) end

---Declares a integer variable. Raises an error until instances exist.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.IntegerVariable
function variables.instance_integer(name) end

---Declares a boolean variable. Raises an error until instances exist.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.BooleanVariable
function variables.instance_boolean(name) end

---Declares a string variable. Raises an error until instances exist.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.StringVariable
function variables.instance_string(name) end

---Declares a table variable. Raises an error until instances exist.
---
---Needs the permission `storage.mod`.
---@param name string 1 to 64 letters, digits and _, not starting with a digit.
---@return wa.TableVariable
function variables.instance_table(name) end

return variables
