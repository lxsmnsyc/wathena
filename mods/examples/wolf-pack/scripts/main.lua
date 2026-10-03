-- The mod script runs once in every zone. Its hook keeps the wolves
-- away from Novices: when the AI picks a Novice, the hook picks another
-- candidate, or none. Hooks need the permission rules.override.

local hooks = require "wa.hooks"
local jobs = require "wa.jobs"
local monsters = require "wa.monsters"

local ALPHA = monsters.id "example.wolf-pack:ALPHA_WOLF"
local PACK = monsters.id "example.wolf-pack:PACK_WOLF"

local function novice(unit)
  return unit.kind == "player" and unit.job == jobs.novice
end

return function()
  hooks.on("monster_target", function(choice, next)
    local id = choice.monster.id
    if id ~= ALPHA and id ~= PACK then
      return next(choice)
    end
    -- rAthena's choice, after the hooks of other mods.
    local pick = next(choice)
    if pick == nil or not novice(pick) then
      return pick
    end
    for _, unit in ipairs(choice.candidates) do
      if not novice(unit) then
        return unit
      end
    end
    return nil
  end)
end
