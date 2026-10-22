-- Loads the agent-rendered config once per worker. The agent reloads nginx
-- after every change, so there is no runtime refresh here. The http-level
-- init_worker/access/log phases belong to the bundled 1pwaf include, so the
-- guard hooks in per server (see access_entry.lua) and initialises lazily.
local cjson = require "cjson.safe"
local ip = require "guard.ip"
local writer = require "guard.writer"

local _M = {
    config = { sites = {} },
    block_tpl = nil,
    ready = false,
}

local ROOT = "/www/guard"

local function read_file(file)
    local fd = io.open(file, "r")
    if not fd then
        return nil
    end
    local content = fd:read("*a")
    fd:close()
    return content
end

local function compile_site(site)
    site.ipAllowRules = ip.compile(site.ipAllow)
    site.ipDenyRules = ip.compile(site.ipDeny)
    site.countrySet = {}
    for _, c in ipairs(site.countries or {}) do
        site.countrySet[c] = true
    end
    return site
end

function _M.load()
    local content = read_file(ROOT .. "/conf/config.json")
    local config = content and cjson.decode(content)
    if type(config) ~= "table" then
        ngx.log(ngx.ERR, "guard: invalid or missing config, protection disabled")
        config = {}
    end
    config.sites = type(config.sites) == "table" and config.sites or {}
    config.logDir = config.logDir or "/var/log/nginx/guard"
    config.slowThreshold = tonumber(config.slowThreshold) or 1000
    for alias, site in pairs(config.sites) do
        config.sites[alias] = compile_site(site)
    end
    _M.config = config
    _M.block_tpl = read_file(ROOT .. "/lua/guard/block.html") or "<h1>{{status}} {{reason}}</h1>"
end

function _M.ensure()
    if _M.ready then
        return
    end
    _M.ready = true
    _M.load()
    local ok, err = ngx.timer.every(1, function(premature)
        if premature then
            writer.flush()
            return
        end
        require("guard.monitor").tick()
        writer.flush()
    end)
    if not ok then
        ngx.log(ngx.ERR, "guard: create flush timer failed: ", err)
    end
end

-- site returns the alias set by "set $guard_site" in the server block.
function _M.site()
    _M.ensure()
    local alias = ngx.var.guard_site
    if not alias or alias == "" then
        return nil, nil
    end
    return alias, _M.config.sites[alias]
end

return _M
