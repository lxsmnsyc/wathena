-- The Hunt Master gives the Poring Patrol quest, and pays for it once its
-- three Porings are down.
--   - Players within 14 cells see a quest icon over her: "!" while they
--     can take the quest, and "?" once its kills are done.
--   - A player who takes too long sees that the time is up, and may take
--     the quest again.

local context = require "wa.context"
local events = require "wa.events"
local timers = require "wa.timers"

local QUEST = "porings"
local APPLE = 512

-- icon_for is the icon a player should see over the NPC.
local function icon_for(player)
  local state = player:get_quest_state(QUEST)
  if state == "none" then
    return "quest"
  end
  if state == "complete" then
    return nil
  end
  local count, goal = player:get_quest_progress(QUEST, 1)
  if count >= goal and not player:quest_timed_out(QUEST) then
    return "quest_question"
  end
  return nil
end

return function()
  local npc = context:get_instance()
  -- shown holds the icon each nearby player sees, by name, or false for
  -- none. A player who walks away is forgotten, so the icon goes again
  -- when it comes back, also after a map change.
  local shown = {}

  local function update(player)
    local want = icon_for(player) or false
    if shown[player.name] ~= want then
      shown[player.name] = want
      npc:show_quest_icon(player, want or nil)
    end
  end

  timers.every(1000, function()
    local near = {}
    for _, player in ipairs(npc:find_nearby { radius = 14, kinds = { "player" } }) do
      near[player.name] = true
      update(player)
    end
    for name in pairs(shown) do
      if not near[name] then
        shown[name] = nil
      end
    end
  end)

  events.on("quest_progress", function(event)
    if event.quest == "example.hunting-board:" .. QUEST and shown[event.player.name] ~= nil then
      update(event.player)
    end
  end)

  events.on("click", function(player, conversation)
    local state = player:get_quest_state(QUEST)
    if state == "complete" then
      conversation:say("[Hunt Master]", "The fields are quiet thanks to you.")
      return
    end
    if state == "none" then
      conversation:say("[Hunt Master]", "Porings are eating the crops again.", "Can you take down three of them within the hour?")
      if conversation:choose("I'll do it.", "Not now.") == 1 then
        player:add_quest(QUEST)
        conversation:say("[Hunt Master]", "Good. Your quest window (Alt+U) keeps the count.")
      end
      update(player)
      return
    end
    if player:quest_timed_out(QUEST) then
      conversation:say("[Hunt Master]", "The hour is up. Come back when you are ready to try again.")
      player:remove_quest(QUEST)
      update(player)
      return
    end
    local count, goal = player:get_quest_progress(QUEST, 1)
    if count < goal then
      conversation:say("[Hunt Master]", "You have taken down " .. count .. " of " .. goal .. " Porings.")
      return
    end
    player:complete_quest(QUEST)
    player:give_zeny(500)
    player:give_item(APPLE, 1)
    conversation:say("[Hunt Master]", "Well done. Here is your pay: 500 zeny and an Apple.")
    update(player)
  end)
end
