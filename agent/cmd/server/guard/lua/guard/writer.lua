-- Per-worker buffered appender. Files are reopened on every flush so the
-- agent can rotate them by renaming.
local _M = {}

local buffers = {}
local MAX_LINES = 20000

function _M.push(file, line)
    local buf = buffers[file]
    if not buf then
        buf = {}
        buffers[file] = buf
    end
    if #buf < MAX_LINES then
        buf[#buf + 1] = line
    end
end

function _M.flush()
    for file, buf in pairs(buffers) do
        if #buf > 0 then
            local fd, err = io.open(file, "a")
            if fd then
                fd:write(table.concat(buf, "\n"), "\n")
                fd:close()
            else
                ngx.log(ngx.WARN, "guard: open ", file, " failed: ", err)
            end
            buffers[file] = {}
        end
    end
end

return _M
