-- Pure objective checks. No WoW API calls, so lua5.1 can test this file.
-- Objectives are ordered: step n is checked only after 1..n-1 are done.
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

local function locationOK(obj, state)
    local any = false
    if norm(obj.zone) ~= "" then
        any = true
        if norm(obj.zone) ~= norm(state.zone) then
            return false
        end
    end
    if norm(obj.subzone) ~= "" then
        any = true
        if norm(obj.subzone) ~= norm(state.subzone) then
            return false
        end
    end
    if obj.map_id ~= nil then
        any = true
        if tonumber(obj.map_id) ~= tonumber(state.map_id) then
            return false
        end
    end
    if obj.x ~= nil and obj.y ~= nil then
        any = true
        local x = tonumber(state.x)
        local y = tonumber(state.y)
        local ox = tonumber(obj.x)
        local oy = tonumber(obj.y)
        if not x or not y or not ox or not oy then
            return false
        end
        local radius = tonumber(obj.radius) or 0.02
        local dx = x - ox
        local dy = y - oy
        if math.sqrt(dx * dx + dy * dy) > radius then
            return false
        end
    end
    return any
end

local function interactOK(obj, state)
    local wantNPC = norm(obj.npc) ~= ""
    local wantObject = norm(obj.object) ~= ""
    if not wantNPC and not wantObject then
        return false
    end
    local npc = wantNPC and same(obj.npc, state.npc)
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

function M.evaluate(objectives, state, saved)
    state = state or {}
    saved = saved or {}
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
            local row = {
                done = prev.done and true or false,
                count = tonumber(prev.count) or 0,
                need = needCount(obj),
                active = false,
            }
            if row.done then
                prevDone = true
            elseif not prevDone then
                allDone = false
            else
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
                    elseif same(state.kill_name, obj.npc) then
                        row.count = (tonumber(prev.count) or 0) + 1
                    else
                        row.count = tonumber(prev.count) or 0
                    end
                    row.done = row.count >= row.need
                else
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
        end
    end
    return steps, allDone
end

GutSagaObjectives = M
return M
