-- The arena master pays tokens, mails trophies, and opens the prize
-- desk.

local events = require "wa.events"
local items = require "wa.items"

local TROPHY = items.id "example.arena:Arena_Trophy"

-- The tokens a match pays.
local PRIZE = 5

-- The trophy costs this many tokens.
local TROPHY_COST = 10

return function()
  events.on("click", function(player, conversation)
    conversation:say("[Arena Master]", "You have " .. player:get_currency("tokens") .. " Arena Tokens.")
    conversation:wait_for_next()
    local choice = conversation:choose("Claim a match prize", "Send me a trophy (" .. TROPHY_COST .. " tokens)", "The prize desk", "Goodbye")
    if choice == 1 then
      -- The mod changes its own currency freely.
      player:give_currency("tokens", PRIZE)
      conversation:say("[Arena Master]", "Here are " .. PRIZE .. " tokens.")
    elseif choice == 2 then
      -- take_currency returns false, and takes nothing, when the
      -- player has too few.
      if not player:take_currency("tokens", TROPHY_COST) then
        conversation:say("[Arena Master]", "A trophy costs " .. TROPHY_COST .. " tokens.")
        return
      end
      -- The mail goes out even when the player cannot carry the
      -- trophy now: it waits in the mailbox.
      player:send_mail({
        title = "Your trophy",
        body = "Well fought.",
        items = { { item = TROPHY, amount = 1 } },
      })
      conversation:say("[Arena Master]", "Your trophy is in the mail.")
    elseif choice == 3 then
      player:open_shop("Prize Desk")
    end
  end)
end
