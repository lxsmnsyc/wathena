-- Helpers of the quest boards and job guilds of procedural worlds
-- (decision 0044): the world's quests, which `wactl world new` chose for
-- each town and guild, and the rules of their kinds.
--   - A hunt is done when its kills reach the count.
--   - A collect quest is done when the player holds the items. Kills of
--     its monster drop the item while the quest is in the log.
--   - A delivery is done at the quest board of its town, with its
--     parcel when it has one.

local internal = require "wa.internal"
local maps = require "wa.maps"
local items = require "wa.items"

local Q = {}

-- list returns the world's quests that keep returns true for, by ID.
function Q.list(keep)
  local out = {}
  for _, q in ipairs(internal.world_quests()) do
    if keep(q) then
      out[#out + 1] = q
    end
  end
  return out
end

-- done reports whether the player met a hunt's or a collect quest's
-- objective.
function Q.done(player, q)
  if q.kind == "hunt" then
    local kills, goal = player:get_quest_progress(q.id, 1)
    return kills ~= nil and kills >= goal
  end
  if q.kind == "collect" then
    return player:count_item(q.item) >= q.count
  end
  return false
end

-- progress describes how far the player is.
function Q.progress(player, q)
  if q.kind == "hunt" then
    local kills, goal = player:get_quest_progress(q.id, 1)
    return (kills or 0) .. " of " .. (goal or q.count) .. " hunted."
  end
  if q.kind == "collect" then
    return math.min(player:count_item(q.item), q.count) .. " of " .. q.count .. " " .. (items.name(q.item) or "items") .. " found."
  end
  return "Take it to the quest board of " .. (maps.get_display_name(q.to) or q.to) .. "."
end

-- take takes what a done quest asks for: a collect quest's items, or a
-- delivery's parcel. It returns false when the player lacks them.
function Q.take(player, q)
  if q.kind == "collect" then
    return player:take_item(q.item, q.count)
  end
  if q.kind == "delivery" and q.item ~= nil then
    return player:take_item(q.item, 1)
  end
  return true
end

-- reward gives a quest's reward, and describes it.
function Q.reward(player, q)
  internal.give_exp(player, q.base_exp, q.job_exp)
  if q.zeny > 0 then
    player:give_zeny(q.zeny)
  end
  return q.base_exp .. " base exp, " .. q.job_exp .. " job exp and " .. q.zeny .. " zeny"
end

return Q
