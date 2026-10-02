-- The trainer: for a fee, a player forgets one skill of its job, with the
-- skills that need it when it agrees, or lowers one stat. The fee is paid
-- only when the lesson changed something. The refunded
-- points come back by rAthena's rules: one skill point per level, and the
-- status point cost of each stat step.

local events = require "wa.events"
local skills = require "wa.skills"
local jobs = require "wa.jobs"

-- The fee for one lesson, in zeny.
local FEE = 500

-- The skills the trainer offers to lower, by first class. A Novice has
-- only its Basic Skill, which a reset keeps for every other job.
local offered = {
  [jobs.swordman] = { "SM_BASH", "SM_MAGNUM", "SM_PROVOKE", "SM_ENDURE" },
  [jobs.mage] = { "MG_FIREBOLT", "MG_FIREBALL", "MG_COLDBOLT", "MG_FROSTDIVER" },
  [jobs.archer] = { "AC_OWL", "AC_VULTURE", "AC_DOUBLE", "AC_SHOWER" },
  [jobs.acolyte] = { "AL_HEAL", "AL_INCAGI", "AL_BLESSING", "AL_DECAGI" },
  [jobs.merchant] = { "MC_INCCARRY", "MC_DISCOUNT", "MC_OVERCHARGE", "MC_MAMMONITE" },
  [jobs.thief] = { "TF_DOUBLE", "TF_MISS", "TF_STEAL", "TF_HIDING" },
}

local stat_names = { "str", "agi", "vit", "int", "dex", "luk" }

-- forget_skill offers the skills the player has of its first class.
local function forget_skill(player, conversation)
  local list = offered[jobs.first_job(player.job)] or {}
  local ids, labels = {}, {}
  for _, name in ipairs(list) do
    local id = skills.id(name)
    local lv = player:get_skill_level(id)
    if lv > 0 then
      ids[#ids + 1] = id
      labels[#labels + 1] = name .. " (level " .. lv .. ")"
    end
  end
  if #ids == 0 then
    conversation:say("[Trainer]", "You have nothing I can make you forget.")
    return
  end
  labels[#labels + 1] = "Never mind"
  local choice = conversation:choose(table.unpack(labels))
  local id = ids[choice]
  if id == nil then
    return
  end
  if player.zeny < FEE then
    conversation:say("[Trainer]", "A lesson costs " .. FEE .. " zeny.")
    return
  end
  -- The fee is paid only when the lesson changed something.
  local done = player:reset_skill(id)
  if not done then
    -- Another learned skill needs it.
    conversation:say("[Trainer]", "Other skills need this one.", "Forget them too?")
    done = conversation:choose("Yes, forget them too", "No") == 1 and player:reset_skill(id, { with_dependents = true })
  end
  if done then
    player:pay_zeny(FEE)
    conversation:say("[Trainer]", "Done. Your skill points are back.")
  else
    conversation:say("[Trainer]", "Then we leave it as it is.")
  end
end

-- lower_stat lowers one stat to 1.
local function lower_stat(player, conversation)
  local labels = {}
  for i, name in ipairs(stat_names) do
    labels[i] = string.upper(name) .. " " .. player["base_" .. name]
  end
  labels[#labels + 1] = "Never mind"
  local stat = stat_names[conversation:choose(table.unpack(labels))]
  if stat == nil then
    return
  end
  if player["base_" .. stat] <= 1 then
    conversation:say("[Trainer]", "That stat is as low as it goes.")
    return
  end
  if not player:pay_zeny(FEE) then
    conversation:say("[Trainer]", "A lesson costs " .. FEE .. " zeny.")
    return
  end
  player:reset_stat(stat)
  conversation:say("[Trainer]", "Done. Your status points are back.")
end

return function()
  events.on("click", function(player, conversation)
    conversation:say("[Trainer]", "For " .. FEE .. " zeny, I help you forget what you learned.")
    conversation:wait_for_next()
    local choice = conversation:choose("Forget a skill", "Lower a stat", "Goodbye")
    if choice == 1 then
      forget_skill(player, conversation)
    elseif choice == 2 then
      lower_stat(player, conversation)
    end
  end)
end
