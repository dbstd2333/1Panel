-- Entry for "log_by_lua_file" in a guarded server block.
local root = "/www/guard/lua/"
if not package.path:find(root, 1, true) then
    package.path = root .. "?.lua;" .. package.path
end
require("guard.log").run()
