-- The quest board of procedural towns (decision 0044). It stands at each
-- town's quest_board place and offers the quests `wactl world new`
-- chose for the town. A quest done here gives its reward and may be
-- taken again. A delivery from another town ends here.

local events = require "wa.events"
local maps = require "wa.maps"
local items = require "wa.items"
local Q = require "core.procedural.quests"

local NAME = "[Quest Board]"

-- states shown beside each quest.
local MARKS = { active = " (taken)", inactive = " (taken)", complete = " (done)" }

-- deliver ends a delivery that reached its town.
local function deliver(player, conversation, q)
  if not Q.take(player, q) then
    conversation:say(NAME, "You do not have the parcel.")
    return
  end
  player:complete_quest(q.id)
  conversation:say(NAME, "Delivered: " .. q.title .. ".", "Reward: " .. Q.reward(player, q) .. ".")
end

-- taken handles a quest the player has.
local function taken(player, conversation, q)
  if q.kind ~= "delivery" and Q.done(player, q) then
    if not Q.take(player, q) then
      conversation:say(NAME, Q.progress(player, q))
      return
    end
    player:complete_quest(q.id)
    conversation:say(NAME, "Quest complete: " .. q.title .. ".", "Reward: " .. Q.reward(player, q) .. ".")
    return
  end
  conversation:say(NAME, q.title, Q.progress(player, q))
  conversation:wait_for_next()
  if conversation:choose("Keep it", "Give it up") == 2 then
    if q.kind == "delivery" and q.item ~= nil and player:count_item(q.item) > 0 then
      player:take_item(q.item, 1)
    end
    player:remove_quest(q.id)
    conversation:say(NAME, "The quest is back on the board.")
  end
end

-- offer describes a quest the player may take.
local function offer(player, conversation, q, state)
  local lines = { q.title }
  if q.kind == "delivery" then
    lines[#lines + 1] = "Go to " .. (maps.get_display_name(q.to) or q.to) .. "'s quest board."
  end
  lines[#lines + 1] = "Reward: " .. q.base_exp .. " base exp, " .. q.job_exp .. " job exp and " .. q.zeny .. " zeny."
  conversation:say(NAME, table.unpack(lines))
  conversation:wait_for_next()
  if conversation:choose("Take it", "Not now") ~= 1 then
    return
  end
  if q.kind == "delivery" and q.item ~= nil then
    if not player:can_carry(q.item, 1) then
      conversation:say(NAME, "You cannot carry the " .. (items.name(q.item) or "parcel") .. ".")
      return
    end
  end
  if state == "complete" then
    player:remove_quest(q.id)
  end
  player:add_quest(q.id)
  if q.kind == "delivery" and q.item ~= nil then
    player:give_item(q.item, 1)
  end
  conversation:say(NAME, "You took the quest: " .. q.title .. ".")
end

return function()
  events.on("click", function(player, conversation)
    local here = player.map
    local board = Q.list(function(q)
      return q.purpose == "board" and q.town == here
    end)
    local arriving = Q.list(function(q)
      local state = player:get_quest_state(q.id)
      return q.purpose == "board" and q.kind == "delivery" and q.to == here and (state == "active" or state == "inactive")
    end)
    if #board == 0 and #arriving == 0 then
      conversation:say(NAME, "No quests are posted here.")
      return
    end
    conversation:say(NAME, "Quests of " .. (maps.get_display_name(here) or "this town") .. ".")
    conversation:wait_for_next()
    local labels = {}
    for _, q in ipairs(arriving) do
      labels[#labels + 1] = "Deliver: " .. q.title
    end
    for _, q in ipairs(board) do
      labels[#labels + 1] = q.title .. (MARKS[player:get_quest_state(q.id)] or "")
    end
    labels[#labels + 1] = "Close"
    local choice = conversation:choose(table.unpack(labels))
    if choice <= #arriving then
      deliver(player, conversation, arriving[choice])
      return
    end
    local q = board[choice - #arriving]
    if q == nil then
      return
    end
    local state = player:get_quest_state(q.id)
    if state == "active" or state == "inactive" then
      taken(player, conversation, q)
    else
      offer(player, conversation, q, state)
    end
  end)
end
