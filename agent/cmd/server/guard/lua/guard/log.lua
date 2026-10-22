local cjson = require "cjson.safe"
local core = require "guard.core"
local monitor = require "guard.monitor"
local writer = require "guard.writer"

local _M = {}

local function cut(s, n)
    if not s then
        return ""
    end
    if #s > n then
        return s:sub(1, n)
    end
    return s
end

function _M.run()
    local alias = ngx.ctx.guard_site
    if not alias then
        -- Requests answered before the access phase (e.g. return, 301) still
        -- count towards the monitor.
        local site
        alias, site = core.site()
        if not site then
            return
        end
    end
    local event = ngx.ctx.guard_event
    if event then
        writer.push(core.config.logDir .. "/events.log", cjson.encode({
            site = alias,
            time = ngx.time(),
            ip = ngx.var.remote_addr,
            country = ngx.var.guard_country or "",
            method = ngx.req.get_method(),
            host = cut(ngx.var.host, 255),
            uri = cut(ngx.var.request_uri, 1024),
            type = event.type,
            action = event.action,
            eventId = event.event_id or "",
            ua = cut(ngx.var.http_user_agent, 512),
        }))
    end
    monitor.record(alias, event ~= nil and event.action == "block")
end

return _M
