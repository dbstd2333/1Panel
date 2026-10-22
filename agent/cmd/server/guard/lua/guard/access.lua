local core = require "guard.core"
local ip = require "guard.ip"
local limit_count = require "resty.limit.count"
local t1k = require "resty.t1k"

local _M = {}

local limiters = {}

local REASONS = {
    ipDeny = "Your IP address is blocked / 您的 IP 已被禁止访问",
    ban = "Too many requests, temporarily banned / 请求过于频繁，已临时封禁",
    cc = "Too many requests / 请求过于频繁",
    geo = "Access from your region is not allowed / 您所在的地区禁止访问",
    attack = "Malicious request detected / 检测到恶意请求",
}

local function render(status, kind, event_id)
    local values = {
        status = tostring(status),
        reason = REASONS[kind] or "Forbidden",
        event_id = event_id or "-",
        ip = ngx.var.remote_addr or "-",
        time = ngx.localtime(),
    }
    return (core.block_tpl:gsub("{{(%w+_?%w*)}}", function(key)
        return values[key] or ""
    end))
end

local function block(kind, status, event_id)
    ngx.ctx.guard_event = { type = kind, action = "block", event_id = event_id }
    ngx.status = status
    ngx.header.content_type = "text/html; charset=utf-8"
    ngx.header.cache_control = "no-store"
    ngx.say(render(status, kind, event_id))
    return ngx.exit(status)
end

local function get_limiter(alias, cc)
    local key = alias .. ":" .. cc.threshold .. ":" .. cc.duration
    local lim = limiters[key]
    if not lim then
        local err
        lim, err = limit_count.new("guard_cc", cc.threshold, cc.duration)
        if not lim then
            ngx.log(ngx.ERR, "guard: create limiter failed: ", err)
            return nil
        end
        limiters[key] = lim
    end
    return lim
end

function _M.run()
    -- Internal redirects (index, try_files, error_page) re-enter the access
    -- phase; the original request was already checked and counted.
    if ngx.req.is_internal() then
        return
    end
    local alias, site = core.site()
    if not site then
        return
    end
    ngx.ctx.guard_site = alias

    local bin = ngx.var.binary_remote_addr
    if ip.any(site.ipAllowRules, bin) then
        return
    end
    if ip.any(site.ipDenyRules, bin) then
        return block("ipDeny", 403)
    end

    local client = ngx.var.remote_addr
    local ban_key = alias .. "|" .. client
    local bans = ngx.shared.guard_ban
    if bans and bans:get(ban_key) then
        return block("ban", 429)
    end

    if site.geoMode == "deny" or site.geoMode == "allow" then
        local country = ngx.var.guard_country or ""
        if country ~= "LAN" then
            local hit = site.countrySet[country] == true
            if (site.geoMode == "deny" and hit) or (site.geoMode == "allow" and not hit) then
                return block("geo", 403)
            end
        end
    end

    local cc = site.cc
    if cc and cc.enable and (cc.threshold or 0) > 0 and (cc.duration or 0) > 0 then
        local lim = get_limiter(alias, cc)
        if lim then
            local delay, err = lim:incoming(ban_key, true)
            if not delay and err == "rejected" then
                if bans and (cc.banTime or 0) > 0 then
                    bans:set(ban_key, true, cc.banTime)
                end
                return block("cc", 429)
            end
        end
    end

    local mode = site.mode
    if (mode == "block" or mode == "monitor") and core.config.detector then
        local ok, err, result = t1k.do_access({
            mode = mode,
            host = core.config.detector,
            connect_timeout = 300,
            send_timeout = 1000,
            read_timeout = 1000,
        }, false)
        if not ok then
            -- Fail open: an unavailable detector must not take the sites down.
            ngx.log(ngx.WARN, "guard: detector unavailable: ", err)
            return
        end
        if result and result.action == "?" then
            if mode == "block" then
                return block("attack", tonumber(result.status) or 403, result.event_id)
            end
            ngx.ctx.guard_event = { type = "attack", action = "monitor", event_id = result.event_id }
        end
    end
end

return _M
