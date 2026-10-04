-- The condition of the fixture's world events: it never holds, so they
-- start only from world_events.start.
return {
  never = function()
    return false
  end,
}
