-- One run of the Pumpkin_Night event, from 18:00 to 06:00 server time.
-- Its drop_rate hook doubles what Pumpkin Porings drop. The hook belongs
-- to the run, so it ends at dawn.

local hooks = require "wa.hooks"
local maps = require "wa.maps"
local monsters = require "wa.monsters"
local text = require "wa.text"

local PUMPKIN_PORING = monsters.id "example.halloween:PUMPKIN_PORING"

return function()
  hooks.on("drop_rate", function(drop, next)
    local rate = next(drop)
    if drop.monster.id == PUMPKIN_PORING then
      return rate * 2
    end
    return rate
  end)
  maps.announce("prt_fild08", text "king.night")
end
