-- Pure objective checks. No WoW API calls, so lua5.1 can test this file.
-- By default goals may complete in any order (ordered=false).
-- When ordered=true, step n is checked only after 1..n-1 are done.
-- state = { zone, subzone, map_id, x, y, npc, object, items = { [id] = count },
--           kill_name, kill_secret }
-- saved = { [objective_id] = { done = bool, count = number } }

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

local function checkOne(obj, state, prev)
    local row = {
        done = prev.done and true or false,
        count = tonumber(prev.count) or 0,
        need = needCount(obj),
        active = false,
    }
    if row.done then
        return row
    end
    row.active = true
    local kind = obj.type
    if kind == "location" then
        row.done = locationOK(obj, state)
    elseif kind == "interact" then
        row.done = interactOK(obj, state)
    elseif kind == "collect" then
        row.count = itemHave(state, obj.item_id)
        row.done = row.count >= row.need
    elseif kind == "kill" then
        if state.kill_secret then
            row.count = tonumber(prev.count) or 0
        elseif listed(obj.npc, state.kill_name) then
            row.count = (tonumber(prev.count) or 0) + 1
        else
            row.count = tonumber(prev.count) or 0
        end
        row.done = row.count >= row.need
    else
        row.done = false
    end
    return row
end

-- ordered: when true (or omitted for backwards-compatible tests), gate later
-- goals on earlier ones. Pass false for any-order sagas.
function M.evaluate(objectives, state, saved, ordered)
    state = state or {}
    saved = saved or {}
    if ordered == nil then
        ordered = true
    end
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
            local row
            if ordered and not prevDone and not (prev.done and true or false) then
                row = {
                    done = false,
                    count = tonumber(prev.count) or 0,
                    need = needCount(obj),
                    active = false,
                }
                allDone = false
            else
                row = checkOne(obj, state, prev)
                if row.done then
                    prevDone = true
                else
                    prevDone = false
                    allDone = false
                end
            end
            steps[id] = row
        end
    end
    return steps, allDone
end

GutSagaObjectives = M
return M
