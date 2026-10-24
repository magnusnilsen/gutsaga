-- Pure objective checks. No WoW API calls, so lua5.1 can test this file.
-- By default goals may complete in any order (ordered=false).
-- When ordered=true, step n is checked only after 1..n-1 are done.
-- state = { zone, subzone, map_id, x, y, npc, object, items = { [id] = count },
--           kill_name, kill_secret,
--           emote, emote_target, emote_zone, emote_subzone, emote_minimap,
--           emote_map_id, emote_x, emote_y,
--           time = { game = {hour, min}, ["local"] = {hour, min, unix}, realm = {hour, min, unix} } }
-- saved = { [objective_id] = { done = bool, count = number } }
--
-- all_of / any_of children are checked live (together), except kill counts which
-- accumulate. The composite itself stays done once it has been true.
-- Clocks: game = GetGameTime, local = time()/date("*t"), realm = GetServerTime as UTC.

local M = {}

local function norm(s)
    if type(s) ~= "string" then
        return ""
    end
    return (s:lower():gsub("^%s+", ""):gsub("%s+$", ""))
end

local function same(a, b)
    local left = norm(a)
    return left ~= "" and left == norm(b)
end

local function needCount(obj)
    local n = tonumber(obj.count) or 1
    if n < 1 then
        return 1
    end
    return n
end

local function playerXY(state)
    local x = tonumber(state.x)
    local y = tonumber(state.y)
    if not x or not y then
        return nil, nil
    end
    -- Some clients report map percents (0–100) instead of fractions.
    if x > 1 or y > 1 then
        x = x / 100
        y = y / 100
    end
    return x, y
end

local function onPoint(obj, state)
    if obj.x == nil or obj.y == nil then
        return false
    end
    local x, y = playerXY(state)
    local ox = tonumber(obj.x)
    local oy = tonumber(obj.y)
    if not x or not y or not ox or not oy then
        return false
    end
    local radius = tonumber(obj.radius) or 0.02
    local dx = x - ox
    local dy = y - oy
    return math.sqrt(dx * dx + dy * dy) <= radius
end

local function key(s)
    return (norm(s):gsub("[^%a%d]+", ""))
end

-- Split on "|" or ";" without a pattern. A "|" inside a Lua pattern is a
-- WoW escape and can make the matcher throw, which freezes later checks.
local function partsOf(want)
    local pipe = string.char(124)
    local out = {}
    local startAt = 1
    while startAt <= #want do
        local pipeAt = string.find(want, pipe, startAt, true)
        local semiAt = string.find(want, ";", startAt, true)
        local cut = pipeAt
        if semiAt and (not cut or semiAt < cut) then
            cut = semiAt
        end
        if not cut then
            table.insert(out, string.sub(want, startAt))
            break
        end
        table.insert(out, string.sub(want, startAt, cut - 1))
        startAt = cut + 1
    end
    return out
end

local function listed(want, got)
    if type(want) ~= "string" then
        return false
    end
    local gotKey = key(got)
    if want == "" or gotKey == "" then
        return false
    end
    if key(want) == gotKey then
        return true
    end
    local parts = partsOf(want)
    for i = 1, #parts do
        local partKey = key(parts[i])
        if partKey ~= "" and (partKey == gotKey or string.find(gotKey, partKey, 1, true) or string.find(partKey, gotKey, 1, true)) then
            return true
        end
    end
    return false
end

local function placeHit(obj, state)
    -- obj.text is the line the player sees. It stays usable even when the
    -- subzone field never arrives in the loaded quest table.
    local wants = { obj.subzone, obj.zone, obj.text }
    local names = { state.subzone, state.zone, state.minimap }
    for i = 1, #names do
        for j = 1, #wants do
            if listed(wants[j], names[i]) then
                return true
            end
        end
    end
    return false
end

local function locationOK(obj, state)
    -- A coordinate hit counts even when Classic and retail disagree on map id.
    -- The minimap label counts too: indoors, GetSubZoneText is often empty
    -- while the minimap still says "Lion's Pride Inn".
    local here = onPoint(obj, state)
    if here or placeHit(obj, state) then
        return true
    end
    local zoneOnly = norm(obj.zone) ~= "" and obj.x == nil and obj.map_id == nil and norm(obj.subzone) == ""
    if zoneOnly and norm(obj.zone) == norm(state.zone) then
        return true
    end
    if obj.map_id ~= nil and obj.x == nil and norm(obj.subzone) == "" and norm(obj.zone) == "" then
        return tonumber(obj.map_id) == tonumber(state.map_id)
    end
    return false
end

local function interactOK(obj, state)
    local wantNPC = norm(obj.npc) ~= ""
    local wantObject = norm(obj.object) ~= ""
    if not wantNPC and not wantObject then
        return false
    end
    -- state.npc is the gossip/merchant unit. state.object is the current target.
    local npc = wantNPC and (same(obj.npc, state.npc) or same(obj.npc, state.object))
    local object = wantObject and (same(obj.object, state.object) or same(obj.object, state.npc))
    return npc or object
end

local function itemHave(state, id)
    if type(state.items) ~= "table" or id == nil then
        return 0
    end
    local n = state.items[id]
    if n == nil then
        n = state.items[tostring(id)]
    end
    return tonumber(n) or 0
end

local function emoteToken(s)
    s = norm(s)
    if s:sub(1, 1) == "/" then
        s = s:sub(2)
    end
    return (s:gsub("%s+", "_"):upper())
end

local function hasPlace(obj)
    return norm(obj.zone) ~= "" or norm(obj.subzone) ~= "" or obj.map_id ~= nil or obj.x ~= nil or obj.y ~= nil
end

local function emoteOK(obj, state)
    local want = emoteToken(obj.emote)
    local got = emoteToken(state.emote)
    if want == "" or got == "" or want ~= got then
        return false
    end
    if norm(obj.npc) ~= "" and not (same(obj.npc, state.emote_target) or listed(obj.npc, state.emote_target)) then
        return false
    end
    if not hasPlace(obj) then
        return true
    end
    return locationOK(obj, {
        zone = state.emote_zone,
        subzone = state.emote_subzone,
        minimap = state.emote_minimap,
        map_id = state.emote_map_id,
        x = state.emote_x,
        y = state.emote_y,
    })
end

local function isLeap(y)
    return (y % 4 == 0 and y % 100 ~= 0) or (y % 400 == 0)
end

local function monthDays(y, m)
    local mdays = { 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31 }
    if m == 2 and isLeap(y) then
        return 29
    end
    return mdays[m] or 0
end

local function unixUTC(y, m, d, hour, minute, second)
    if m < 1 or m > 12 or d < 1 or d > monthDays(y, m) then
        return nil
    end
    local days = 0
    for year = 1970, y - 1 do
        days = days + (isLeap(year) and 366 or 365)
    end
    for month = 1, m - 1 do
        days = days + monthDays(y, month)
    end
    days = days + d - 1
    return days * 86400 + hour * 3600 + minute * 60 + second
end

local function parseRFC3339(s)
    if type(s) ~= "string" then
        return nil
    end
    local y, mo, d, H, Mi, S, rest = s:match("^(%d%d%d%d)%-(%d%d)%-(%d%d)T(%d%d):(%d%d):(%d%d)(.*)$")
    if not y then
        return nil
    end
    rest = rest:gsub("^%.%d+", "")
    local offset = 0
    if rest ~= "Z" and rest ~= "" then
        local sign, oh, om = rest:match("^([%+%-])(%d%d):(%d%d)$")
        if not sign then
            return nil
        end
        offset = (tonumber(oh) * 60 + tonumber(om)) * 60
        if sign == "-" then
            offset = -offset
        end
    end
    local unix = unixUTC(tonumber(y), tonumber(mo), tonumber(d), tonumber(H), tonumber(Mi), tonumber(S))
    if not unix then
        return nil
    end
    return unix - offset
end

local function minutes(s)
    if type(s) ~= "string" then
        return nil
    end
    local h, m = s:match("^(%d%d):(%d%d)$")
    h, m = tonumber(h), tonumber(m)
    if not h or not m then
        return nil
    end
    return h * 60 + m
end

local function inWindow(hour, min, startAt, endAt)
    hour, min = tonumber(hour), tonumber(min)
    local cur = hour and min and (hour * 60 + min) or nil
    local a, b = minutes(startAt), minutes(endAt)
    if not cur or not a or not b then
        return false
    end
    if a <= b then
        return cur >= a and cur <= b
    end
    return cur >= a or cur <= b
end

-- timeOpen is true when there is no window, or the state's clock matches.
-- A missing clock fails closed so a saga cannot complete on the wrong clock.
function M.timeOpen(when, state)
    if type(when) ~= "table" then
        return true
    end
    local clock = when.clock
    if type(clock) ~= "string" or clock == "" then
        clock = "local"
    end
    local clocks = type(state) == "table" and state.time or nil
    local now = type(clocks) == "table" and clocks[clock] or nil
    if type(now) ~= "table" then
        return false
    end
    if type(when.start) == "string" and when.start ~= "" then
        if not inWindow(now.hour, now.min, when.start, when["end"]) then
            return false
        end
    end
    local unix = tonumber(now.unix)
    if type(when.starts_at) == "string" and when.starts_at ~= "" then
        local t = parseRFC3339(when.starts_at)
        if not t or not unix or unix < t then
            return false
        end
    end
    if type(when.ends_at) == "string" and when.ends_at ~= "" then
        local t = parseRFC3339(when.ends_at)
        if not t or not unix or unix > t then
            return false
        end
    end
    return true
end

-- groupOK is the saga-level party check. A missing rule is always ok.
-- members below min_level do not count. instance matches the dungeon name or the zone.
function M.groupOK(rule, group)
    if type(rule) ~= "table" or tonumber(rule.size) == nil then
        return true
    end
    local need = tonumber(rule.size) or 1
    if need < 1 then
        return true
    end
    local minLevel = tonumber(rule.min_level) or 1
    group = type(group) == "table" and group or {}
    local n = 0
    local members = type(group.members) == "table" and group.members or {}
    for i = 1, #members do
        local m = members[i]
        if type(m) == "table" and norm(m.name) ~= "" and (tonumber(m.level) or 0) >= minLevel then
            n = n + 1
        end
    end
    if n < need then
        return false
    end
    if norm(rule.instance) ~= "" then
        if not (listed(rule.instance, group.instance) or listed(rule.instance, group.zone)) then
            return false
        end
    end
    return true
end

local function preserveTree(obj, saved, into)
    if type(obj) ~= "table" or type(obj.objectives) ~= "table" then
        return
    end
    for i, child in ipairs(obj.objectives) do
        if type(child) == "table" then
            local id = child.id or tostring(i)
            if type(saved[id]) == "table" then
                into[id] = saved[id]
            end
            preserveTree(child, saved, into)
        end
    end
end

local checkOne

local function checkComposite(obj, state, saved)
    local rows = {}
    local all, any = true, false
    local have, total = 0, 0
    local kids = type(obj.objectives) == "table" and obj.objectives or {}
    for i, child in ipairs(kids) do
        if type(child) == "table" then
            total = total + 1
            local id = child.id or tostring(i)
            local prev = saved[id] or {}
            local row, nested = checkOne(child, state, saved, prev, true)
            rows[id] = row
            if type(nested) == "table" then
                for cid, crow in pairs(nested) do
                    rows[cid] = crow
                end
            end
            if row.done then
                have = have + 1
                any = true
            else
                all = false
            end
        end
    end
    local done = false
    if obj.type == "any_of" then
        done = any
    elseif total > 0 then
        done = all
    end
    return done, rows, have, total
end

checkOne = function(obj, state, saved, prev, live)
    local row = {
        done = prev.done and true or false,
        count = tonumber(prev.count) or 0,
        need = needCount(obj),
        active = false,
    }
    if not live and row.done then
        local kept = {}
        preserveTree(obj, saved, kept)
        return row, kept
    end
    if live then
        row.done = false
    end
    row.active = true
    local open = M.timeOpen(obj.when, state)
    local nested
    local kind = obj.type
    if kind == "location" then
        row.done = open and locationOK(obj, state)
    elseif kind == "interact" then
        row.done = open and interactOK(obj, state)
    elseif kind == "collect" then
        row.count = itemHave(state, obj.item_id)
        row.done = open and row.count >= row.need
    elseif kind == "kill" then
        if state.kill_secret then
            row.count = tonumber(prev.count) or 0
        elseif listed(obj.npc, state.kill_name) then
            row.count = (tonumber(prev.count) or 0) + 1
        else
            row.count = tonumber(prev.count) or 0
        end
        row.done = open and row.count >= row.need
    elseif kind == "emote" then
        -- Sticky once matched: pendingEmote is only live for a few seconds, and
        -- finishQuest / UI refresh may land on a later tick.
        if open and emoteOK(obj, state) then
            row.done = true
        elseif not live and prev.done then
            row.done = true
        else
            row.done = false
        end
    elseif kind == "all_of" or kind == "any_of" then
        local done, rows, have, total = checkComposite(obj, state, saved)
        nested = rows
        row.count = have
        row.need = total
        row.done = open and done
    else
        row.done = false
    end
    if not open and not (prev.done and not live) then
        row.done = false
    end
    return row, nested
end

-- ordered: when true (or omitted for backwards-compatible tests), gate later
-- goals on earlier ones. Pass false for any-order sagas.
-- sagaWhen: optional saga-level window. While it is closed, nothing new completes.
function M.evaluate(objectives, state, saved, ordered, sagaWhen)
    state = state or {}
    saved = saved or {}
    if ordered == nil then
        ordered = true
    end
    local sagaOpen = M.timeOpen(sagaWhen, state)
    local steps = {}
    local prevDone = true
    local allDone = type(objectives) == "table" and #objectives > 0
    if type(objectives) ~= "table" then
        return steps, false
    end
    for i, obj in ipairs(objectives) do
        if type(obj) ~= "table" then
            allDone = false
            prevDone = false
        else
            local id = obj.id or tostring(i)
            local prev = saved[id] or {}
            local row, nested
            if ordered and not prevDone and not (prev.done and true or false) then
                row = {
                    done = false,
                    count = tonumber(prev.count) or 0,
                    need = needCount(obj),
                    active = false,
                }
                nested = {}
                preserveTree(obj, saved, nested)
                allDone = false
            else
                row, nested = checkOne(obj, state, saved, prev, false)
                if not sagaOpen and not (prev.done and true or false) then
                    row.done = false
                end
                if row.done then
                    prevDone = true
                else
                    prevDone = false
                    allDone = false
                end
            end
            steps[id] = row
            if type(nested) == "table" then
                for cid, crow in pairs(nested) do
                    steps[cid] = crow
                end
            end
        end
    end
    if not sagaOpen then
        allDone = false
    end
    return steps, allDone
end

GutSagaObjectives = M
return M
