-- The refiner of procedural towns (decision 0044), ported from rAthena's
-- refinemain (npc/merchants/refine.txt) without its safe and multiple
-- refine features. It stands at each town's blacksmith place. The
-- server keeps the rules: refine.yml's price, ore and chance, items up
-- to +9, and a failure destroys the item.

local events = require "wa.events"
local items = require "wa.items"
local internal = require "wa.internal"

local NAME = "[Refiner]"

-- The equipment slots, in refinemain's order, with
-- F_getpositionname's names.
local SLOTS = {
  { "head_top", "Head" }, { "armor", "Body" }, { "left_hand", "Left hand" }, { "right_hand", "Right hand" },
  { "garment", "Robe" }, { "shoes", "Shoes" }, { "left_accessory", "Accessory 1" },
  { "right_accessory", "Accessory 2" }, { "head_middle", "Head 2" }, { "head_low", "Head 3" },
}

return function()
  events.on("click", function(player, conversation)
    conversation:say(NAME, "I'm the Armsmith.", "I can refine all kinds of weapons, armor and equipment, so let me",
      "know what you want me to refine.")
    conversation:wait_for_next()
    local labels, slots = {}, {}
    for _, s in ipairs(SLOTS) do
      local info = internal.refine_info(player, s[1])
      if info ~= nil then
        labels[#labels + 1] = s[2] .. "-[" .. (items.name(info.item) or "?") .. "]"
        slots[#slots + 1] = s[1]
      end
    end
    if #slots == 0 then
      conversation:say(NAME, "I don't think I can refine any items you have...")
      return
    end
    local slot = slots[conversation:choose(table.unpack(labels))]
    local info = internal.refine_info(player, slot)
    if info == nil then
      conversation:say(NAME, "You're not wearing", "anything there that", "I can refine.")
      return
    end
    if not info.refineable then
      conversation:say(NAME, "I don't think I can", "refine this item at all...")
      return
    end
    if info.max then
      conversation:say(NAME, "I can't refine this", "any more. This is as", "refined as it gets!")
      return
    end
    local ore = items.name(info.material) or "ore"
    conversation:say(NAME, "To refine this I need", "one " .. ore .. " and", "a service fee of " .. info.price .. " Zeny.",
      "Do you really wish to continue?")
    conversation:wait_for_next()
    if conversation:choose("Yes", "No") == 2 then
      conversation:say(NAME, "Yeah...", "There's no need to", "rush. Take your time.")
      return
    end
    if info.chance < 100 then
      conversation:say(NAME, "Oh no! If I continue to", "refine this, there's a risk it could",
        "be destroyed, and you'd lose the item, any cards in it,", "or any added special properties.",
        "Do you still want to refine?")
      conversation:wait_for_next()
      if conversation:choose("Yes", "No") == 2 then
        conversation:say(NAME, "I completely agree...", "I might be a great refiner, but sometimes even I make mistakes.")
        return
      end
    end
    local result = internal.refine(player, slot)
    if result == "success" then
      conversation:say(NAME, "Perfect!", "Heh heh!", "Once again,", "flawless work", "from the master~")
    elseif result == "failure" then
      conversation:say(NAME, "Nooooooo!", "It broke!", "I-I'm sorry!")
    elseif result == "no_material" or result == "no_zeny" then
      conversation:say(NAME, "You don't seem to have", "enough Zeny or " .. ore .. "...", "Go get some more. I'll be",
        "here all day if you need me.")
    else
      conversation:say(NAME, "I can't refine that now.")
    end
  end)
end
