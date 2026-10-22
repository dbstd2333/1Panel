-- Per-worker, per-minute traffic aggregation. Every finished minute becomes
-- one JSON line per site in monitor.log; the agent merges the lines written
-- by all workers.
local cjson = require "cjson.safe"
local core = require "guard.core"
local writer = require "guard.writer"

local crc32 = ngx.crc32_long
local floor = math.floor

local _M = {}

-- Upper bounds (ms) of the response time histogram; the last bucket is open.
local BOUNDS = { 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000 }
local MAX_KEYS = 500
local MAX_HASHES = 10000
local MAX_SLOW = 20
local TOP_N = 100

local buckets = {}

local function new_bucket(alias, minute)
    local hist = {}
    for i = 1, #BOUNDS + 1 do
        hist[i] = 0
    end
    return {
        site = alias, minute = minute,
        req = 0, bin = 0, bout = 0, s2 = 0, s3 = 0, s4 = 0, s5 = 0, blocked = 0, rtSum = 0,
        hist = hist,
        uv = {}, uvN = 0, ips = {}, ipN = 0,
        top = { url = {}, ip = {}, ref = {}, ua = {}, country = {}, err = {} },
        topN = { url = 0, ip = 0, ref = 0, ua = 0, country = 0, err = 0 },
        slow = {},
    }
end

local function count(b, dim, key)
    if not key or key == "" then
        return
    end
    local t = b.top[dim]
    local v = t[key]
    if v then
        t[key] = v + 1
    elseif b.topN[dim] < MAX_KEYS then
        t[key] = 1
        b.topN[dim] = b.topN[dim] + 1
    else
        t["(other)"] = (t["(other)"] or 0) + 1
    end
end

local function add_hash(set, b, nkey, h)
    if set[h] then
        return
    end
    if b[nkey] < MAX_HASHES then
        set[h] = true
        b[nkey] = b[nkey] + 1
    end
end

local function cut(s, n)
    if s and #s > n then
        return s:sub(1, n)
    end
    return s
end

function _M.record(alias, blocked)
    local minute = floor(ngx.time() / 60) * 60
    local key = alias .. "|" .. minute
    local b = buckets[key]
    if not b then
        b = new_bucket(alias, minute)
        buckets[key] = b
    end

    local var = ngx.var
    local status = tonumber(var.status) or 0
    local rt = (tonumber(var.request_time) or 0) * 1000
    local client = var.remote_addr or ""
    local ua = cut(var.http_user_agent or "", 200)
    local uri = cut(var.uri or "", 256)

    b.req = b.req + 1
    b.bin = b.bin + (tonumber(var.request_length) or 0)
    b.bout = b.bout + (tonumber(var.bytes_sent) or 0)
    if status >= 500 then
        b.s5 = b.s5 + 1
    elseif status >= 400 then
        b.s4 = b.s4 + 1
    elseif status >= 300 then
        b.s3 = b.s3 + 1
    elseif status >= 200 then
        b.s2 = b.s2 + 1
    end
    if blocked then
        b.blocked = b.blocked + 1
    end
    b.rtSum = b.rtSum + rt
    local idx = #BOUNDS + 1
    for i = 1, #BOUNDS do
        if rt <= BOUNDS[i] then
            idx = i
            break
        end
    end
    b.hist[idx] = b.hist[idx] + 1

    add_hash(b.uv, b, "uvN", crc32(client .. "|" .. ua))
    add_hash(b.ips, b, "ipN", crc32(client))

    count(b, "url", uri)
    count(b, "ip", client)
    local ref = var.http_referer
    if ref then
        count(b, "ref", ref:match("^%a[%w+.-]*://([^/?#]+)") or cut(ref, 128))
    end
    count(b, "ua", ua)
    count(b, "country", var.guard_country)
    if status >= 400 then
        count(b, "err", status .. " " .. uri)
    end

    if rt >= core.config.slowThreshold then
        local slow = b.slow
        local item = {
            t = ngx.time(), m = ngx.req.get_method(), u = cut(var.request_uri or "", 512),
            ip = client, s = status, rt = rt,
        }
        if #slow < MAX_SLOW then
            slow[#slow + 1] = item
        else
            local min_i = 1
            for i = 2, #slow do
                if slow[i].rt < slow[min_i].rt then
                    min_i = i
                end
            end
            if slow[min_i].rt < rt then
                slow[min_i] = item
            end
        end
    end
end

local function top(t)
    local list = {}
    for k, v in pairs(t) do
        list[#list + 1] = { k, v }
    end
    table.sort(list, function(a, b) return a[2] > b[2] end)
    local out = {}
    for i = 1, math.min(#list, TOP_N) do
        out[list[i][1]] = list[i][2]
    end
    return out
end

local function keys(set)
    local out = {}
    for k in pairs(set) do
        out[#out + 1] = k
    end
    return out
end

local function serialize(b)
    local tops = {}
    for dim, t in pairs(b.top) do
        tops[dim] = top(t)
    end
    return cjson.encode({
        site = b.site, minute = b.minute, req = b.req, ["in"] = b.bin, out = b.bout,
        s2 = b.s2, s3 = b.s3, s4 = b.s4, s5 = b.s5, blocked = b.blocked,
        rtSum = b.rtSum, hist = b.hist, uv = keys(b.uv), ips = keys(b.ips),
        top = tops, slow = b.slow,
    })
end

-- tick flushes every bucket of a finished minute.
function _M.tick()
    local current = floor(ngx.time() / 60) * 60
    local file = core.config.logDir .. "/monitor.log"
    for key, b in pairs(buckets) do
        if b.minute < current then
            local line = serialize(b)
            if line then
                writer.push(file, line)
            end
            buckets[key] = nil
        end
    end
end

return _M
