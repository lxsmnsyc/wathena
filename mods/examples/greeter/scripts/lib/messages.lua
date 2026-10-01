-- A helper module. It can return any value: here, a table of functions.

local messages = {}

function messages.welcome(name)
  return "Welcome, " .. name .. "! The greeter mod is running."
end

function messages.reminder(cookie)
  return "Five seconds have passed. Welcome cookies have item ID " .. cookie .. "."
end

return messages
