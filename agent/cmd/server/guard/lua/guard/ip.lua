-- CIDR matching against ngx.var.binary_remote_addr (4 or 16 bytes).
local bit = require "bit"

local band, lshift = bit.band, bit.lshift
local byte, char, sub = string.byte, string.char, string.sub
local tonumber = tonumber

local _M = {}

local function parse_ipv6(addr)
    local head, tail = addr, nil
    local pos = addr:find("::", 1, true)
    if pos then
        head, tail = addr:sub(1, pos - 1), addr:sub(pos + 2)
    end
    local function groups(s)
        local list = {}
        if s == "" then
            return list
        end
        for g in s:gmatch("[^:]+") do
            local v = tonumber(g, 16)
            if not v or #g > 4 then
                return nil
            end
            list[#list + 1] = v
        end
        return list
    end
    local h, t = groups(head), groups(tail or "")
    if not h or not t then
        return nil
    end
    local fill = 8 - #h - #t
    if (pos and fill < 1) or (not pos and fill ~= 0) then
        return nil
    end
    local all = {}
    for _, v in ipairs(h) do all[#all + 1] = v end
    for _ = 1, fill do all[#all + 1] = 0 end
    for _, v in ipairs(t) do all[#all + 1] = v end
    local out = {}
    for _, v in ipairs(all) do
        out[#out + 1] = char(band(v, 0xff00) / 256, band(v, 0xff))
    end
    return table.concat(out)
end

-- parse returns { bin = <bytes>, bits = <prefix> } or nil for invalid input.
function _M.parse(cidr)
    local addr, bits = cidr:match("^%s*([^/%s]+)/?(%d*)%s*$")
    if not addr then
        return nil
    end
    local bin
    local a, b, c, d = addr:match("^(%d+)%.(%d+)%.(%d+)%.(%d+)$")
    if a then
        a, b, c, d = tonumber(a), tonumber(b), tonumber(c), tonumber(d)
        if a > 255 or b > 255 or c > 255 or d > 255 then
            return nil
        end
        bin = char(a, b, c, d)
    else
        bin = parse_ipv6(addr)
    end
    if not bin then
        return nil
    end
    local max = #bin * 8
    bits = tonumber(bits) or max
    if bits > max then
        return nil
    end
    return { bin = bin, bits = bits }
end

function _M.compile(list)
    local rules = {}
    for _, item in ipairs(list or {}) do
        local rule = _M.parse(item)
        if rule then
            rules[#rules + 1] = rule
        end
    end
    return rules
end

local function match(bin, rule)
    if #bin ~= #rule.bin then
        return false
    end
    local full = math.floor(rule.bits / 8)
    if full > 0 and sub(bin, 1, full) ~= sub(rule.bin, 1, full) then
        return false
    end
    local rest = rule.bits % 8
    if rest == 0 then
        return true
    end
    local mask = band(lshift(0xff, 8 - rest), 0xff)
    return band(byte(bin, full + 1), mask) == band(byte(rule.bin, full + 1), mask)
end

function _M.any(rules, bin)
    if not rules or not bin then
        return false
    end
    for i = 1, #rules do
        if match(bin, rules[i]) then
            return true
        end
    end
    return false
end

return _M
