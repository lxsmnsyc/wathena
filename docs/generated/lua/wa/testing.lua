---@meta wa.testing
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Test cases. It exists only under wactl mod test, never in a running server.
---
---It exists only in tests that wactl mod test runs.
---@class wa.testing
local testing = {}

---Registers a case. Call it inside the function that the test file returns.
---@param name string Unique in the file.
---@param run fun(world: wa.TestWorld) Runs in a fresh world: the mod and its dependencies loaded, a fixed random seed, and no database.
function testing.case(name, run) end

---Checks a value. A failed check fails the case at the line that called it.
---@param value any
---@return wa.Expectation
function testing.expect(value) end

return testing
