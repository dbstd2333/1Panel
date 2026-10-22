-- Entry for "access_by_lua_file" in a guarded server block. lua_package_path
-- is owned by the 1pwaf include, so the guard prepends its own path here.
local root = "/www/guard/lua/"
if not package.path:find(root, 1, true) then
    package.path = root .. "?.lua;" .. package.path
end
require("guard.access").run()
